package platform

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"event-network-manager/backend/internal/domain"
)

// BuildReferenceResetPlan recreates the operational parts of the supplied
// nm-default-SG350-28-56 reference. Device identity is deliberately excluded:
// hostname, management addressing, users, SNMP secrets, SSH keys and
// certificates must remain those of the target switch.
func BuildReferenceResetPlan(switchID, currentConfiguration string) (domain.ConfigPlan, error) {
	if strings.TrimSpace(switchID) == "" {
		return domain.ConfigPlan{}, errors.New("Switch-ID fehlt")
	}
	managementVLAN, managementAddress, ok := staticManagementAddress(currentConfiguration)
	if !ok {
		return domain.ConfigPlan{}, errors.New("Wiederherstellung abgebrochen: die aktuelle statische Management-IP konnte nicht sicher gelesen werden")
	}

	commands := []string{
		"configure terminal",
		"vlan database",
		"vlan 2 name Control",
		"vlan 3 name Lighting",
		"vlan 4 name Internet",
		"vlan 4000 name Management",
		"exit",
		"voice vlan state disabled",
		"no eee enable",
		"qos trust dscp",
	}
	// Match the reference DSCP table exactly. All non-event DSCP values use the
	// default queue, while Dante clock/audio/control keep queues 4/3/2.
	for dscp := 0; dscp <= 63; dscp++ {
		queue := 1
		switch dscp {
		case 8:
			queue = 2
		case 46:
			queue = 3
		case 56:
			queue = 4
		}
		commands = append(commands, fmt.Sprintf("qos map dscp-queue %d to %d", dscp, queue))
	}
	// Remove per-VLAN queriers before disabling snooping globally. Multicast
	// filtering is disabled as well so grandMA2-style multicast is flooded and
	// cannot be black-holed by stale group state.
	for _, vlan := range []int{1, 2, 3, 4, 4000} {
		commands = append(commands,
			fmt.Sprintf("no ip igmp snooping vlan %d querier", vlan),
			fmt.Sprintf("no ip igmp snooping vlan %d", vlan),
		)
	}
	commands = append(commands, "no ip igmp snooping", "no bridge multicast filtering")

	accessVLAN := map[int]int{}
	for _, port := range []int{1, 2, 3, 4, 13, 14, 15, 16} {
		accessVLAN[port] = 1
	}
	for _, port := range []int{5, 6, 7, 8, 17, 18, 19, 20} {
		accessVLAN[port] = 2
	}
	for _, port := range []int{9, 10, 21, 22} {
		accessVLAN[port] = 3
	}
	for _, port := range []int{11, 12, 23, 24} {
		accessVLAN[port] = 4
	}
	ports := make([]int, 0, len(accessVLAN))
	for port := range accessVLAN {
		ports = append(ports, port)
	}
	sort.Ints(ports)
	poeCapable := strings.Contains(strings.ToLower(currentConfiguration), "sg350-28p") || strings.Contains(strings.ToLower(currentConfiguration), "power inline ")
	for _, port := range ports {
		commands = append(commands,
			fmt.Sprintf("interface gi%d", port),
			"no description",
			"switchport mode access",
			fmt.Sprintf("switchport access vlan %d", accessVLAN[port]),
			"spanning-tree disable",
			"no spanning-tree portfast",
		)
		if poeCapable {
			commands = append(commands, "power inline auto")
		}
		commands = append(commands, "exit")
	}
	for port := 25; port <= 28; port++ {
		commands = append(commands,
			fmt.Sprintf("interface gi%d", port),
			"no description",
			"switchport mode trunk",
			"switchport access vlan 4000",
			"switchport general pvid 4000",
			"switchport trunk native vlan 4000",
			"switchport trunk allowed vlan 1-4,4000",
			"no spanning-tree disable",
			"no spanning-tree portfast",
			"exit",
		)
	}
	commands = append(commands, "end")

	warnings := []string{
		"Vor der Wiederherstellung wird automatisch ein vollständiger Running-Config-Snapshot erstellt.",
		fmt.Sprintf("Die aktuelle Management-IP %s auf VLAN %d wird nicht verändert.", managementAddress, managementVLAN),
		"Switch-Name, Benutzer, Passwörter, SNMP-Zugänge, SSH-Schlüssel und Zertifikate bleiben erhalten.",
		"Ports 1–24 werden auf das ME-1–4-Schema gesetzt; Ports 25–28 werden Management-Trunks.",
		"IGMP Snooping, IGMP Querier und Multicast-Filterung werden ausgeschaltet.",
		"Zusätzliche, danach unbenutzte VLAN-Definitionen bleiben erhalten, damit keine weitere Management-Schnittstelle versehentlich gelöscht wird.",
		"Nach dem Anwenden wird die Startup Config gespeichert und vollständig gegengeprüft.",
	}
	return domain.ConfigPlan{
		ID:          "me-reference-reset-" + switchID,
		SwitchID:    switchID,
		Description: "ME-Standard aus Referenz wiederherstellen (IGMP aus)",
		Commands:    commands,
		Warnings:    warnings,
	}, nil
}

func (a *SG350SSHConfigurator) ReferenceResetPlan(ctx context.Context, switchID string) (domain.ConfigPlan, error) {
	if switchID != a.config.SwitchID {
		return domain.ConfigPlan{}, fmt.Errorf("unknown switch %q", switchID)
	}
	configuration, err := a.runningConfiguration(ctx)
	if err != nil {
		return domain.ConfigPlan{}, err
	}
	return BuildReferenceResetPlan(switchID, configuration)
}

func staticManagementAddress(configuration string) (vlan int, address string, ok bool) {
	interfacePattern := regexp.MustCompile(`(?i)^interface\s+vlan\s+([0-9]{1,4})$`)
	addressPattern := regexp.MustCompile(`^ip address\s+([0-9]{1,3}(?:\.[0-9]{1,3}){3})\s+([0-9]{1,3}(?:\.[0-9]{1,3}){3})$`)
	currentVLAN := 0
	for _, line := range strings.Split(strings.ReplaceAll(configuration, "\r", ""), "\n") {
		trimmed := strings.TrimSpace(line)
		if match := interfacePattern.FindStringSubmatch(trimmed); len(match) == 2 {
			currentVLAN, _ = strconv.Atoi(match[1])
			continue
		}
		if strings.HasPrefix(strings.ToLower(trimmed), "interface ") || trimmed == "exit" || trimmed == "!" {
			currentVLAN = 0
			continue
		}
		if currentVLAN == 0 {
			continue
		}
		if match := addressPattern.FindStringSubmatch(trimmed); len(match) == 3 && validIPv4(match[1]) && validIPv4(match[2]) {
			return currentVLAN, match[1], true
		}
	}
	return 0, "", false
}

func validIPv4(value string) bool {
	parts := strings.Split(value, ".")
	if len(parts) != 4 {
		return false
	}
	for _, part := range parts {
		n, err := strconv.Atoi(part)
		if err != nil || n < 0 || n > 255 {
			return false
		}
	}
	return true
}

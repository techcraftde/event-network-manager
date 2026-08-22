package platform

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"event-network-manager/backend/internal/domain"
)

// buildDantePlan renders only commands documented for the SG350 2.5 CLI.
// Immediate Leave is intentionally omitted: enabling it is unsafe when more
// than one Dante receiver can sit behind a port.
func buildDantePlan(request domain.DantePlanRequest) (domain.ConfigPlan, error) {
	if request.SwitchID == "" {
		return domain.ConfigPlan{}, errors.New("Switch-ID fehlt")
	}
	if request.VLANID < 1 || request.VLANID > 4094 {
		return domain.ConfigPlan{}, errors.New("VLAN-ID muss zwischen 1 und 4094 liegen")
	}
	seen := map[int]bool{}
	ports := make([]int, 0, len(request.Ports))
	for _, port := range request.Ports {
		if port < 1 || port > 24 {
			return domain.ConfigPlan{}, fmt.Errorf("Dante-Port %d liegt außerhalb des Kupferport-Bereichs 1–24", port)
		}
		if !seen[port] {
			seen[port] = true
			ports = append(ports, port)
		}
	}
	if len(ports) == 0 {
		return domain.ConfigPlan{}, errors.New("mindestens ein Dante-Port muss ausgewählt sein")
	}
	sort.Ints(ports)
	commands := []string{
		"configure terminal",
		"ip igmp snooping",
		fmt.Sprintf("ip igmp snooping vlan %d", request.VLANID),
		"qos trust dscp",
	}
	for _, port := range ports {
		commands = append(commands,
			fmt.Sprintf("interface gi1/0/%d", port),
			"qos trust",
			"no eee enable",
			"exit",
		)
	}
	commands = append(commands, "end")
	portStrings := make([]string, len(ports))
	for i, port := range ports {
		portStrings[i] = fmt.Sprintf("gi%d", port)
	}
	return domain.ConfigPlan{
		ID:          fmt.Sprintf("dante-%s-vlan-%d", request.SwitchID, request.VLANID),
		SwitchID:    request.SwitchID,
		Description: fmt.Sprintf("Dante-Vorgabe mit QoS, IGMP und EEE für VLAN %d auf %s", request.VLANID, strings.Join(portStrings, ", ")),
		Commands:    commands,
		Warnings: []string{
			"Der IGMP-Querier wird nicht automatisch aktiviert; im VLAN sollte genau ein Querier festgelegt sein.",
			"Immediate Leave bleibt deaktiviert, weil nachgelagerte Verteilungen nicht sicher erkannt werden können.",
		},
	}, nil
}

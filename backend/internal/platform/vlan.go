package platform

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"event-network-manager/backend/internal/domain"
)

func normalizePorts(ports []int, max int) ([]int, error) {
	seen := map[int]bool{}
	result := make([]int, 0, len(ports))
	for _, port := range ports {
		if port < 1 || port > max {
			return nil, fmt.Errorf("Port %d liegt außerhalb des gültigen Bereichs 1–%d", port, max)
		}
		if !seen[port] {
			seen[port] = true
			result = append(result, port)
		}
	}
	if len(result) == 0 {
		return nil, errors.New("mindestens ein Port muss ausgewählt sein")
	}
	sort.Ints(result)
	return result, nil
}

func buildVLANPlan(request domain.VLANPlanRequest) (domain.ConfigPlan, error) {
	if request.SwitchID == "" {
		return domain.ConfigPlan{}, errors.New("Switch-ID fehlt")
	}
	if request.VLANID < 1 || request.VLANID > 4094 {
		return domain.ConfigPlan{}, errors.New("VLAN-ID muss zwischen 1 und 4094 liegen")
	}
	ports, err := normalizePorts(request.Ports, 28)
	if err != nil {
		return domain.ConfigPlan{}, err
	}
	if request.Mode != "access" && request.Mode != "trunk" {
		return domain.ConfigPlan{}, errors.New("Modus muss access oder trunk sein")
	}
	commands := []string{"configure terminal"}
	for _, port := range ports {
		commands = append(commands, fmt.Sprintf("interface gi%d", port))
		if request.Mode == "access" {
			commands = append(commands, "switchport mode access", fmt.Sprintf("switchport access vlan %d", request.VLANID))
		} else {
			commands = append(commands, "switchport mode trunk", fmt.Sprintf("switchport trunk allowed vlan add %d", request.VLANID))
		}
		commands = append(commands, "exit")
	}
	commands = append(commands, "end")
	portNames := make([]string, len(ports))
	for i, port := range ports {
		portNames[i] = fmt.Sprintf("gi%d", port)
	}
	modeText := "als ungetaggte Access-Ports"
	warnings := []string{"Access-Zuweisung ersetzt den bisherigen Portmodus und die ungetaggte VLAN-Zuweisung."}
	if request.Mode == "trunk" {
		modeText = "als getaggte Trunk-Ports"
		warnings = []string{"Das VLAN wird zur vorhandenen Trunk-Liste hinzugefügt; andere getaggte VLANs bleiben erhalten."}
	}
	return domain.ConfigPlan{
		ID: fmt.Sprintf("vlan-%s-%d-%s", request.SwitchID, request.VLANID, request.Mode), SwitchID: request.SwitchID,
		Description: fmt.Sprintf("VLAN %d auf %s %s zuweisen", request.VLANID, strings.Join(portNames, ", "), modeText),
		Commands:    commands, Warnings: warnings,
	}, nil
}

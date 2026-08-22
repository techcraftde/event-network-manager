package platform

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"event-network-manager/backend/internal/domain"
)

var safePortName = regexp.MustCompile(`^[A-Za-z0-9ÄÖÜäöüß _.,:+()/#-]{1,64}$`)
var safeVLANName = regexp.MustCompile(`[^A-Za-z0-9_-]+`)

func BuildRolePlan(request domain.RolePlanRequest, profiles []domain.RoleProfile, existingVLANs []domain.VLAN) (domain.ConfigPlan, error) {
	if request.SwitchID == "" {
		return domain.ConfigPlan{}, errors.New("Switch-ID fehlt")
	}
	if len(request.Ports) == 0 {
		return domain.ConfigPlan{}, errors.New("mindestens ein Port muss ausgewählt sein")
	}
	profileByID := map[string]domain.RoleProfile{}
	for _, profile := range profiles {
		if profile.ID == "" || profile.Name == "" {
			return domain.ConfigPlan{}, errors.New("Rollenprofil ist unvollständig")
		}
		if profile.PortMode == "access" && (profile.VLANID < 1 || profile.VLANID > 4094) {
			return domain.ConfigPlan{}, fmt.Errorf("Rolle %s hat keine gültige Netzwerkzuordnung", profile.Name)
		}
		profileByID[profile.ID] = profile
	}
	seenPorts := map[int]bool{}
	ports := append([]domain.RolePortRequest(nil), request.Ports...)
	sort.Slice(ports, func(i, j int) bool { return ports[i].PortIndex < ports[j].PortIndex })
	for _, port := range ports {
		if port.PortIndex < 1 || port.PortIndex > 28 {
			return domain.ConfigPlan{}, fmt.Errorf("Port %d liegt außerhalb des gültigen Bereichs 1–28", port.PortIndex)
		}
		if seenPorts[port.PortIndex] {
			return domain.ConfigPlan{}, fmt.Errorf("Port %d wurde mehrfach angegeben", port.PortIndex)
		}
		seenPorts[port.PortIndex] = true
		if !safePortName.MatchString(strings.TrimSpace(port.DisplayName)) {
			return domain.ConfigPlan{}, fmt.Errorf("Name für Port %d muss 1–64 Zeichen lang sein und darf keine Steuerzeichen enthalten", port.PortIndex)
		}
		if _, ok := profileByID[port.RoleID]; !ok {
			return domain.ConfigPlan{}, fmt.Errorf("unbekannte Rolle für Port %d", port.PortIndex)
		}
	}
	exists := map[int]bool{}
	for _, vlan := range existingVLANs {
		exists[vlan.ID] = true
	}
	required := map[int]domain.RoleProfile{}
	for _, port := range ports {
		profile := profileByID[port.RoleID]
		if profile.PortMode == "access" {
			required[profile.VLANID] = profile
		}
		if profile.PortMode == "trunk" {
			for _, id := range profile.AllowedRoleIDs {
				if allowed, ok := profileByID[id]; ok && allowed.VLANID > 0 {
					required[allowed.VLANID] = allowed
				}
			}
		}
	}
	commands := []string{"configure terminal"}
	ids := make([]int, 0, len(required))
	for id := range required {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	missing := []string{}
	for _, id := range ids {
		if exists[id] {
			continue
		}
		name := strings.Trim(safeVLANName.ReplaceAllString(required[id].Name, "-"), "-")
		if name == "" {
			name = fmt.Sprintf("Role-%d", id)
		}
		commands = append(commands, "vlan database", fmt.Sprintf("vlan %d name %s", id, name), "exit")
		missing = append(missing, required[id].Name)
	}
	needsMulticast, needsQoS := map[int]bool{}, false
	for _, port := range ports {
		profile := profileByID[port.RoleID]
		if profile.Multicast && profile.VLANID > 0 {
			needsMulticast[profile.VLANID] = true
		}
		needsQoS = needsQoS || profile.DanteQoS
	}
	if len(needsMulticast) > 0 {
		commands = append(commands, "ip igmp snooping")
		multicastIDs := make([]int, 0, len(needsMulticast))
		for id := range needsMulticast {
			multicastIDs = append(multicastIDs, id)
		}
		sort.Ints(multicastIDs)
		for _, id := range multicastIDs {
			commands = append(commands, fmt.Sprintf("ip igmp snooping vlan %d", id))
		}
	}
	if needsQoS {
		commands = append(commands, "qos trust dscp")
	}
	labels := make([]string, 0, len(ports))
	for _, port := range ports {
		profile := profileByID[port.RoleID]
		commands = append(commands, fmt.Sprintf("interface gi%d", port.PortIndex), `description "`+strings.TrimSpace(port.DisplayName)+`"`)
		if profile.PortMode == "trunk" {
			commands = append(commands, "switchport mode trunk")
			allowed := []int{}
			for _, id := range profile.AllowedRoleIDs {
				if p, ok := profileByID[id]; ok && p.VLANID > 0 {
					allowed = append(allowed, p.VLANID)
				}
			}
			sort.Ints(allowed)
			for _, id := range allowed {
				commands = append(commands, fmt.Sprintf("switchport trunk allowed vlan add %d", id))
			}
		} else {
			commands = append(commands, "switchport mode access", fmt.Sprintf("switchport access vlan %d", profile.VLANID))
		}
		if profile.DanteQoS {
			commands = append(commands, "qos trust")
		}
		if port.PortIndex <= 24 {
			if profile.DisableEEE {
				commands = append(commands, "no eee enable")
			}
			if profile.PoEMode == "off" {
				commands = append(commands, "power inline never")
			} else {
				commands = append(commands, "power inline auto")
			}
		}
		commands = append(commands, "exit")
		labels = append(labels, fmt.Sprintf("%s → %s", strings.TrimSpace(port.DisplayName), profile.Name))
	}
	commands = append(commands, "end")
	warnings := []string{"Vor dem Anwenden wird automatisch ein vollständiger Konfigurations-Snapshot erstellt."}
	if len(missing) > 0 {
		warnings = append(warnings, "Benötigte Netzwerkbereiche werden automatisch angelegt: "+strings.Join(missing, ", ")+".")
	}
	return domain.ConfigPlan{ID: fmt.Sprintf("roles-%s", request.SwitchID), SwitchID: request.SwitchID, Description: strings.Join(labels, ", "), Commands: commands, Warnings: warnings}, nil
}

func ValidateRoleProfiles(profiles []domain.RoleProfile) error {
	defaults := domain.DefaultRoleProfiles()
	if len(profiles) != len(defaults) {
		return errors.New("alle sieben Rollen müssen vorhanden sein")
	}
	required := map[string]bool{}
	for _, profile := range defaults {
		required[profile.ID] = true
	}
	for _, profile := range profiles {
		if !required[profile.ID] {
			return fmt.Errorf("unbekannte Rolle %q", profile.ID)
		}
		delete(required, profile.ID)
		if profile.Name == "" || profile.Color == "" || (profile.PortMode != "access" && profile.PortMode != "trunk") {
			return fmt.Errorf("Rolle %s ist unvollständig", profile.ID)
		}
		if profile.PortMode == "access" && (profile.VLANID < 1 || profile.VLANID > 4094) {
			return fmt.Errorf("Rolle %s hat eine ungültige Netzwerk-ID", profile.Name)
		}
		if profile.PoEMode != "auto" && profile.PoEMode != "off" {
			return fmt.Errorf("Rolle %s hat eine ungültige PoE-Einstellung", profile.Name)
		}
	}
	return nil
}

package platform

import (
	"strings"
	"testing"

	"event-network-manager/backend/internal/domain"
)

func TestBuildRolePlanHidesCiscoDetailsBehindDanteRole(t *testing.T) {
	plan, err := BuildRolePlan(domain.RolePlanRequest{SwitchID: "switch-1", Ports: []domain.RolePortRequest{{PortIndex: 7, DisplayName: "FOH Mischpult", RoleID: "dante"}}}, domain.DefaultRoleProfiles(), []domain.VLAN{{ID: 1}})
	if err != nil {
		t.Fatal(err)
	}
	commands := strings.Join(plan.Commands, "\n")
	for _, wanted := range []string{"vlan 20 name Dante-Audio", `description "FOH Mischpult"`, "switchport access vlan 20", "ip igmp snooping vlan 20", "qos trust dscp", "no eee enable", "power inline auto"} {
		if !strings.Contains(commands, wanted) {
			t.Errorf("missing %q in\n%s", wanted, commands)
		}
	}
	if plan.Description != "FOH Mischpult → Dante/Audio" {
		t.Fatalf("description = %q", plan.Description)
	}
}

func TestBuildRolePlanTrunkUsesConfiguredRolesAndSkipsCopperCommandsOnSFP(t *testing.T) {
	plan, err := BuildRolePlan(domain.RolePlanRequest{SwitchID: "switch-1", Ports: []domain.RolePortRequest{{PortIndex: 25, DisplayName: "Uplink Bühne", RoleID: "trunk"}}}, domain.DefaultRoleProfiles(), nil)
	if err != nil {
		t.Fatal(err)
	}
	commands := strings.Join(plan.Commands, "\n")
	for _, vlan := range []string{"10", "20", "30", "40", "50", "99"} {
		if !strings.Contains(commands, "switchport trunk allowed vlan add "+vlan) {
			t.Errorf("missing vlan %s", vlan)
		}
	}
	if strings.Contains(commands, "eee enable") || strings.Contains(commands, "power inline") {
		t.Fatalf("SFP plan contains copper-only command:\n%s", commands)
	}
}

func TestBuildRolePlanRejectsUnsafePortName(t *testing.T) {
	_, err := BuildRolePlan(domain.RolePlanRequest{SwitchID: "switch-1", Ports: []domain.RolePortRequest{{PortIndex: 1, DisplayName: "bad\nend", RoleID: "control"}}}, domain.DefaultRoleProfiles(), nil)
	if err == nil {
		t.Fatal("expected validation error")
	}
}

func TestRoleRollbackRestoresNameAndRemovesCreatedVLAN(t *testing.T) {
	configuration := "vlan database\n vlan 1,10\n exit\ninterface gi7\n description Alt\n switchport mode access\n switchport access vlan 10\n power inline auto\n exit\n"
	applied := []string{"configure terminal", "vlan database", "vlan 20 name Dante-Audio", "exit", "interface gi7", `description "FOH Mischpult"`, "switchport mode access", "switchport access vlan 20", "no eee enable", "power inline auto", "exit", "end"}
	rollback := strings.Join(buildRollbackCommands(configuration, applied), "\n")
	for _, wanted := range []string{"description Alt", "switchport access vlan 10", "eee enable", "no vlan 20"} {
		if !strings.Contains(rollback, wanted) {
			t.Errorf("missing %q in\n%s", wanted, rollback)
		}
	}
}

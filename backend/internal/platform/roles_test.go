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
	for _, wanted := range []string{`description "FOH Mischpult"`, "switchport access vlan 1", "bridge multicast filtering", "ip igmp snooping vlan 1 querier", "qos trust dscp", "qos map dscp-queue 56 to 4", "flowcontrol off", "no eee enable", "power inline auto"} {
		if !strings.Contains(commands, wanted) {
			t.Errorf("missing %q in\n%s", wanted, commands)
		}
	}
	if plan.Description != "FOH Mischpult → Dante/Audio" {
		t.Fatalf("description = %q", plan.Description)
	}
}

func TestBuildRolePlanTrunkUsesConfiguredRolesAndSkipsCopperCommandsOnSFP(t *testing.T) {
	plan, err := BuildRolePlan(domain.RolePlanRequest{SwitchID: "switch-1", Ports: []domain.RolePortRequest{{PortIndex: 27, DisplayName: "Uplink Bühne", RoleID: "trunk"}}}, domain.DefaultRoleProfiles(), nil)
	if err != nil {
		t.Fatal(err)
	}
	commands := strings.Join(plan.Commands, "\n")
	for _, vlan := range []string{"1", "2", "3", "4", "5", "4000"} {
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

func TestVerifyAccessVLANOneAcceptsCiscoDefaultOmission(t *testing.T) {
	commands := []string{"configure terminal", "interface gi1", "switchport access vlan 1", "exit", "end"}
	configuration := "interface GigabitEthernet1\n description Audio\n!\n"
	if err := verifyAppliedConfiguration(commands, configuration); err != nil {
		t.Fatalf("default VLAN 1 should verify without an explicit line: %v", err)
	}
	if err := verifyAppliedConfiguration([]string{"interface gi1", "switchport access vlan 2"}, configuration); err == nil {
		t.Fatal("non-default VLAN must still require an explicit configuration line")
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

func TestRoleRollbackRestoresFlowControlWhenItWasEnabled(t *testing.T) {
	configuration := "interface gi7\n flowcontrol on\n exit\n"
	applied := []string{"configure terminal", "interface gi7", "flowcontrol off", "exit", "end"}
	rollback := strings.Join(buildRollbackCommands(configuration, applied), "\n")
	if !strings.Contains(rollback, "flowcontrol on") {
		t.Fatalf("flow control was not restored:\n%s", rollback)
	}
}

func TestBuildRolePlanConfiguresMultiplePortsAndLoopProtection(t *testing.T) {
	plan, err := BuildRolePlan(domain.RolePlanRequest{SwitchID: "switch-1", Ports: []domain.RolePortRequest{
		{PortIndex: 3, DisplayName: "Bühne 3", RoleID: "lighting"},
		{PortIndex: 4, DisplayName: "Bühne 4", RoleID: "lighting"},
	}}, domain.DefaultRoleProfiles(), nil)
	if err != nil {
		t.Fatal(err)
	}
	commands := strings.Join(plan.Commands, "\n")
	for _, wanted := range []string{"interface gi3", "interface gi4", "switchport access vlan 3", "no spanning-tree disable", "spanning-tree portfast"} {
		if !strings.Contains(commands, wanted) {
			t.Errorf("missing %q in\n%s", wanted, commands)
		}
	}
}

func TestBuildAndInspectEventBaseline(t *testing.T) {
	profiles := domain.DefaultRoleProfiles()
	plan, err := BuildEventBaselinePlan("switch-1", profiles, nil)
	if err != nil {
		t.Fatal(err)
	}
	configuration := strings.Join(plan.Commands, "\n")
	status := inspectEventBaselineConfiguration("switch-1", profiles, configuration)
	if !status.Healthy {
		t.Fatalf("status=%#v\n%s", status, configuration)
	}
	for _, wanted := range []string{"bridge multicast filtering", "ip igmp snooping vlan 1 querier", "vlan 4000 name Trunk-Management", "qos map dscp-queue 56 to 4", "no eee enable"} {
		if !strings.Contains(configuration, wanted) {
			t.Errorf("missing %q", wanted)
		}
	}
}

func TestParseSwitchConfigStateUsesSwitchAsSourceOfTruth(t *testing.T) {
	configuration := "hostname Stage-Rack\ninterface GigabitEthernet7\n description \"FOH Pult\"\n switchport mode access\n switchport access vlan 1\n exit\ninterface GigabitEthernet25\n description Uplink\n switchport mode trunk\n exit\n"
	state := parseSwitchConfigState("switch-1", configuration, domain.DefaultRoleProfiles())
	if state.Name != "Stage-Rack" || len(state.PortSettings) != 28 {
		t.Fatalf("state=%#v", state)
	}
	byPort := map[int]domain.PortSetting{}
	for _, setting := range state.PortSettings {
		byPort[setting.PortIndex] = setting
	}
	if byPort[7].DisplayName != "FOH Pult" || byPort[7].RoleID != "dante" || byPort[25].RoleID != "trunk" {
		t.Fatalf("ports 7/25=%#v %#v", byPort[7], byPort[25])
	}
}

func TestBuildSwitchNamePlanNormalizesFriendlyName(t *testing.T) {
	plan, name, err := BuildSwitchNamePlan("switch-1", "Stage links")
	if err != nil || name != "Stage-links" || !strings.Contains(strings.Join(plan.Commands, "\n"), "hostname Stage-links") {
		t.Fatalf("name=%q plan=%#v err=%v", name, plan, err)
	}
}

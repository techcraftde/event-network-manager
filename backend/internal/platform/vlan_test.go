package platform

import (
	"strings"
	"testing"

	"event-network-manager/backend/internal/domain"
)

func TestBuildVLANAccessPlan(t *testing.T) {
	plan, err := buildVLANPlan(domain.VLANPlanRequest{SwitchID: "sw", VLANID: 20, Ports: []int{5, 2, 5}, Mode: "access"})
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(plan.Commands, "\n")
	for _, wanted := range []string{"interface gi1/0/2\nswitchport mode access\nswitchport access vlan 20", "interface gi1/0/5"} {
		if !strings.Contains(got, wanted) {
			t.Errorf("Plan enthält %q nicht:\n%s", wanted, got)
		}
	}
}

func TestBuildVLANTrunkPlan(t *testing.T) {
	plan, err := buildVLANPlan(domain.VLANPlanRequest{SwitchID: "sw", VLANID: 30, Ports: []int{28}, Mode: "trunk"})
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(plan.Commands, "\n")
	if !strings.Contains(got, "switchport trunk allowed vlan add 30") {
		t.Fatal(got)
	}
}

func TestBuildVLANPlanRejectsInvalidInput(t *testing.T) {
	for _, request := range []domain.VLANPlanRequest{
		{SwitchID: "sw", VLANID: 0, Ports: []int{1}, Mode: "access"},
		{SwitchID: "sw", VLANID: 2, Ports: []int{29}, Mode: "access"},
		{SwitchID: "sw", VLANID: 2, Ports: []int{1}, Mode: "general"},
	} {
		if _, err := buildVLANPlan(request); err == nil {
			t.Errorf("ungültige Anfrage akzeptiert: %#v", request)
		}
	}
}

func TestInspectDanteConfiguration(t *testing.T) {
	config := "IGMP Snooping is globally enabled\nVLAN 10\n  IGMP Snooping is enabled\nAdvanced mode trust type: dscp\nAdvanced mode ports state: Trusted\nEEE Administrate status is enabled on ports: gi2\n"
	health := inspectDanteConfiguration("sw", 10, []int{1, 2}, config)
	if !health.IGMPGlobal || !health.IGMPVLAN || !health.QoSDSCP {
		t.Fatalf("Globale Prüfung falsch: %#v", health)
	}
	if health.QoSTrustedPorts != 2 || health.EEEDisabledPorts != 1 || health.Healthy {
		t.Fatalf("Portprüfung falsch: %#v", health)
	}
}

func TestVLANRollbackRestoresPortConfiguration(t *testing.T) {
	configuration := "interface gi1/0/5\n switchport mode trunk\n switchport trunk allowed vlan add 1,10\n exit\n"
	applied := []string{"configure terminal", "interface gi1/0/5", "switchport mode access", "switchport access vlan 20", "exit", "end"}
	rollback := strings.Join(buildRollbackCommands(configuration, applied), "\n")
	for _, wanted := range []string{"interface gi1/0/5", "no switchport access vlan", "switchport mode trunk"} {
		if !strings.Contains(rollback, wanted) {
			t.Errorf("Rollback enthält %q nicht:\n%s", wanted, rollback)
		}
	}
}

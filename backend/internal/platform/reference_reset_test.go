package platform

import (
	"strings"
	"testing"
)

func TestBuildReferenceResetPlanPreservesIdentityAndMatchesPortLayout(t *testing.T) {
	configuration := `hostname Stage-Test
username admin password encrypted secret-value
vlan database
 vlan 2-5,50,4000
 exit
interface vlan 4000
 ip address 192.168.250.55 255.255.255.0
 exit
interface GigabitEthernet25
 switchport mode access
 switchport access vlan 50
 exit
ip igmp snooping
bridge multicast filtering
`
	plan, err := BuildReferenceResetPlan("stage", configuration)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(plan.Commands, "\n")
	for _, wanted := range []string{
		"no ip igmp snooping",
		"no bridge multicast filtering",
		"qos map dscp-queue 8 to 2",
		"qos map dscp-queue 46 to 3",
		"qos map dscp-queue 56 to 4",
		"interface gi5\nno description\nswitchport mode access\nswitchport access vlan 2",
		"interface gi25\nno description\nswitchport mode trunk\nswitchport access vlan 4000\nswitchport general pvid 4000\nswitchport trunk native vlan 4000\nswitchport trunk allowed vlan 1-4,4000",
	} {
		if !strings.Contains(joined, wanted) {
			t.Fatalf("plan misses %q", wanted)
		}
	}
	for _, forbidden := range []string{"ip address ", "hostname ", "username ", "secret-value", "no vlan 5", "no vlan 50"} {
		if strings.Contains(joined, forbidden) {
			t.Fatalf("plan must preserve device identity/additional VLANs, found %q", forbidden)
		}
	}
	if err := validateConfigCommands(plan.Commands); err != nil {
		t.Fatalf("generated plan violates allowlist: %v", err)
	}
	if len(plan.Commands) > 512 {
		t.Fatalf("plan exceeds SG350 command limit: %d", len(plan.Commands))
	}
}

func TestBuildReferenceResetPlanRequiresStaticManagementAddress(t *testing.T) {
	_, err := BuildReferenceResetPlan("stage", "interface vlan 4000\n ip address dhcp\n exit\n")
	if err == nil || !strings.Contains(err.Error(), "Management-IP") {
		t.Fatalf("expected safe management address failure, got %v", err)
	}
}

func TestReferenceResetVerificationAndRollback(t *testing.T) {
	before := `voice vlan state auto-enabled
eee enable
ip igmp snooping
bridge multicast filtering
interface GigabitEthernet5
 description "Old control"
 switchport mode trunk
 switchport access vlan 50
 spanning-tree portfast
 power inline never
 exit
interface GigabitEthernet25
 switchport mode access
 switchport access vlan 50
 exit
`
	commands := []string{
		"configure terminal", "voice vlan state disabled", "no eee enable",
		"no ip igmp snooping", "no bridge multicast filtering",
		"interface gi5", "no description", "switchport mode access", "switchport access vlan 2", "spanning-tree disable", "no spanning-tree portfast", "power inline auto", "exit",
		"interface gi25", "no description", "switchport mode trunk", "switchport access vlan 4000", "switchport general pvid 4000", "switchport trunk native vlan 4000", "switchport trunk allowed vlan 1-4,4000", "no spanning-tree disable", "no spanning-tree portfast", "exit", "end",
	}
	after := `voice vlan state disabled
no eee enable
no ip igmp snooping
no bridge multicast filtering
interface GigabitEthernet5
 switchport access vlan 2
 spanning-tree disable
 no spanning-tree portfast
 exit
interface GigabitEthernet25
 switchport mode trunk
 switchport access vlan 4000
 switchport general pvid 4000
 switchport trunk native vlan 4000
 switchport trunk allowed vlan 1-4,4000
 no spanning-tree portfast
 exit
`
	if err := verifyAppliedConfiguration(commands, after); err != nil {
		t.Fatal(err)
	}
	rollback := strings.Join(buildRollbackCommands(before, commands), "\n")
	for _, wanted := range []string{
		"voice vlan state auto-enabled", "eee enable", "ip igmp snooping", "bridge multicast filtering",
		`description "Old control"`, "switchport access vlan 50", "switchport mode trunk", "spanning-tree portfast", "power inline never",
		"switchport trunk allowed vlan 1-4094", "switchport general pvid 1", "switchport trunk native vlan 1", "switchport mode access",
	} {
		if !strings.Contains(rollback, wanted) {
			t.Fatalf("rollback misses %q:\n%s", wanted, rollback)
		}
	}
}

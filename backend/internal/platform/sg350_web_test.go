package platform

import (
	"regexp"
	"strings"
	"testing"
)

func TestExtractInterfaceCounters(t *testing.T) {
	html := `
writeFormattedNumber("702157", pgTkn, "TotalBytesNum", "", "StringCell");
<INPUT TYPE=HIDDEN NAME=ifInErrors$query disabled value="3">
writeFormattedNumber("1203313", pgTkn, "TotalBytesBottomNum", "", "StringCell");`
	tests := []struct {
		name        string
		want        uint64
		patternName string
	}{
		{"rx", 702157, "rx"},
		{"tx", 1203313, "tx"},
		{"errors", 3, "errors"},
	}
	patterns := map[string]*regexp.Regexp{"rx": inOctetsPattern, "tx": outOctetsPattern, "errors": inErrorsPattern}
	for _, tc := range tests {
		got, ok := extractUint(patterns[tc.patternName], html)
		if !ok || got != tc.want {
			t.Fatalf("%s = %d, %v; want %d, true", tc.name, got, ok, tc.want)
		}
	}
}

func TestCiscoLoginPath(t *testing.T) {
	got := mtsPathPattern.FindStringSubmatch("/csc3dc344c/mts/config/log_off_page.htm")
	if len(got) != 2 || got[1] != "/csc3dc344c/mts" {
		t.Fatalf("unexpected match: %#v", got)
	}
}

func TestCiscoAuthenticatedPath(t *testing.T) {
	got := mtsAuthenticatedPathPattern.FindStringSubmatch("/csc3dc344c/mts/home.htm")
	if len(got) != 2 || got[1] != "/csc3dc344c/mts" {
		t.Fatalf("unexpected match: %#v", got)
	}
}

func TestParseVLANRange(t *testing.T) {
	got := parseVLANRange("1-4, 4000,3")
	want := []int{1, 2, 3, 4, 4000}
	if len(got) != len(want) {
		t.Fatalf("got %#v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %#v; want %#v", got, want)
		}
	}
}

func TestModelFromDescription(t *testing.T) {
	if got := modelFromDescription("SG350-28P 28-Port Gigabit PoE Managed Switch"); got != "SG350-28P" {
		t.Fatalf("model = %q", got)
	}
}

func TestBuildRollbackCommandsRestoresOnlyChangedSettings(t *testing.T) {
	configuration := "qos trust cos\ninterface gi1/0/5\n eee enable\n exit\n"
	applied := []string{"configure terminal", "ip igmp snooping", "qos trust dscp", "interface gi1/0/5", "qos trust", "no eee enable", "exit", "end"}
	rollback := strings.Join(buildRollbackCommands(configuration, applied), "\n")
	for _, expected := range []string{"qos trust cos", "interface gi1/0/5", "no qos trust", "eee enable"} {
		if !strings.Contains(rollback, expected) {
			t.Errorf("rollback missing %q:\n%s", expected, rollback)
		}
	}
}

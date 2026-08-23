package api

import (
	"strings"
	"testing"
	"time"

	"event-network-manager/backend/internal/domain"
)

func TestAlarmMonitorDetectsEventRisks(t *testing.T) {
	topology := domain.Topology{UpdatedAt: time.Now(), Switches: []domain.Switch{{ID: "stage", Name: "Stage", Status: "online", TemperatureC: 62, PoEBudgetWatts: 100, PoEUsageWatts: 90, Ports: []domain.Port{{Index: 1, DisplayName: "Dante Pult", RoleID: "dante", Link: true, SpeedMbps: 100, RxMbps: 75, Errors: 2}}}}}
	report := newAlarmMonitor().evaluate(topology)
	if report.WarningCount < 4 {
		t.Fatalf("warnings=%d alarms=%#v", report.WarningCount, report.Alarms)
	}
	wanted := map[string]bool{"Temperatur": false, "PoE": false, "Paketfehler": false, "Auslastung": false, "Dante/Audio": false}
	for _, alarm := range report.Alarms {
		if _, ok := wanted[alarm.Category]; ok {
			wanted[alarm.Category] = true
		}
	}
	for category, found := range wanted {
		if !found {
			t.Errorf("missing category %s", category)
		}
	}
}

func TestEventModeReportsLinkLossWithRecognizedDevice(t *testing.T) {
	monitor := newAlarmMonitor()
	before := domain.Topology{Switches: []domain.Switch{{ID: "stage", Name: "Stage", Status: "online", Ports: []domain.Port{{Index: 7, DisplayName: "Lichtpult", Link: true, SpeedMbps: 1000}}}}, Devices: []domain.ConnectedDevice{{ID: "console", SwitchID: "stage", PortIndex: 7, Name: "grandMA3", IPAddress: "192.168.250.70"}}}
	monitor.setEventMode(true, before)
	after := before
	after.Switches[0].Ports[0].Link = false
	report := monitor.evaluate(after)
	if !report.EventMode || report.CriticalCount != 1 || len(report.Alarms) != 1 {
		t.Fatalf("report=%#v", report)
	}
	if !strings.Contains(report.Alarms[0].Message, "grandMA3") || report.Alarms[0].Category != "Link-Änderung" {
		t.Fatalf("alarm=%#v", report.Alarms[0])
	}
}

func TestAlarmMonitorAllClear(t *testing.T) {
	report := newAlarmMonitor().evaluate(domain.Topology{Switches: []domain.Switch{{ID: "foh", Name: "FOH", Status: "online", Ports: []domain.Port{{Index: 1, Link: true, SpeedMbps: 1000, RxMbps: 10}}}}})
	if len(report.Alarms) != 0 || report.HealthPercent != 100 || len(report.ChecksOK) < 3 {
		t.Fatalf("report=%#v", report)
	}
}

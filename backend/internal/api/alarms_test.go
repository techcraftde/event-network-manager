package api

import (
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

func TestAlarmMonitorAllClear(t *testing.T) {
	report := newAlarmMonitor().evaluate(domain.Topology{Switches: []domain.Switch{{ID: "foh", Name: "FOH", Status: "online", Ports: []domain.Port{{Index: 1, Link: true, SpeedMbps: 1000, RxMbps: 10}}}}})
	if len(report.Alarms) != 0 || report.HealthPercent != 100 || len(report.ChecksOK) < 3 {
		t.Fatalf("report=%#v", report)
	}
}

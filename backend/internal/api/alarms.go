package api

import (
	"fmt"
	"math"
	"sort"
	"sync"
	"time"

	"event-network-manager/backend/internal/domain"
)

type alarmMonitor struct {
	mu             sync.Mutex
	firstSeen      map[string]time.Time
	eventMode      bool
	eventModeSince time.Time
	previousLinks  map[string]observedLink
	eventAlarms    map[string]domain.Alarm
}

type observedLink struct {
	up         bool
	switchID   string
	switchName string
	portIndex  int
	portName   string
	deviceName string
}

func newAlarmMonitor() *alarmMonitor {
	return &alarmMonitor{firstSeen: map[string]time.Time{}, previousLinks: map[string]observedLink{}, eventAlarms: map[string]domain.Alarm{}}
}

func (m *alarmMonitor) setEventMode(enabled bool, topology domain.Topology) domain.EventModeStatus {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.eventMode = enabled
	m.previousLinks = observeLinks(topology)
	m.eventAlarms = map[string]domain.Alarm{}
	if enabled {
		m.eventModeSince = time.Now().UTC()
	} else {
		m.eventModeSince = time.Time{}
	}
	return domain.EventModeStatus{Enabled: m.eventMode, EnabledAt: m.eventModeSince}
}

func (m *alarmMonitor) eventModeStatus() domain.EventModeStatus {
	m.mu.Lock()
	defer m.mu.Unlock()
	return domain.EventModeStatus{Enabled: m.eventMode, EnabledAt: m.eventModeSince}
}

func (m *alarmMonitor) evaluate(topology domain.Topology) domain.AlarmReport {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now().UTC()
	report := domain.AlarmReport{GeneratedAt: now, EventMode: m.eventMode, EventModeSince: m.eventModeSince, HealthPercent: 100, Alarms: []domain.Alarm{}}
	active := map[string]bool{}
	add := func(alarm domain.Alarm) {
		active[alarm.ID] = true
		if first, ok := m.firstSeen[alarm.ID]; ok {
			alarm.DetectedAt = first
		} else {
			alarm.DetectedAt = now
			m.firstSeen[alarm.ID] = now
		}
		report.Alarms = append(report.Alarms, alarm)
		switch alarm.Severity {
		case "critical":
			report.CriticalCount++
		case "warning":
			report.WarningCount++
		default:
			report.InfoCount++
		}
	}
	if m.eventMode {
		current := observeLinks(topology)
		for key, value := range current {
			previous, found := m.previousLinks[key]
			if !found || previous.up == value.up {
				continue
			}
			device := value.deviceName
			if device == "" {
				device = previous.deviceName
			}
			deviceText := ""
			if device != "" {
				deviceText = " (" + device + ")"
			}
			alarm := domain.Alarm{ID: "event-link-" + key, Category: "Link-Änderung", SwitchID: value.switchID, SwitchName: value.switchName, PortIndex: value.portIndex, PortName: value.portName, DetectedAt: now}
			if value.up {
				alarm.Severity = "info"
				alarm.Title = "Link verbunden"
				alarm.Message = fmt.Sprintf("%s%s ist jetzt verbunden.", value.portName, deviceText)
				alarm.Recommendation = "Prüfen, ob die Verbindung während der Veranstaltung erwartet wurde."
			} else {
				alarm.Severity = "critical"
				alarm.Title = "Link ausgefallen"
				alarm.Message = fmt.Sprintf("%s%s hat die Verbindung verloren.", value.portName, deviceText)
				alarm.Recommendation = "Gerät, Stromversorgung und Kabel sofort prüfen."
			}
			m.eventAlarms[alarm.ID] = alarm
		}
		m.previousLinks = current
		for _, alarm := range m.eventAlarms {
			add(alarm)
		}
	}
	for _, sw := range topology.Switches {
		if sw.Status != "online" {
			add(domain.Alarm{ID: sw.ID + "-offline", Severity: "critical", Category: "Erreichbarkeit", SwitchID: sw.ID, SwitchName: sw.Name, Title: "Switch nicht erreichbar", Message: "Live-Daten dieses Switches fehlen.", Recommendation: "Stromversorgung, Uplink und Management-Verbindung prüfen."})
		}
		if sw.TemperatureC >= 70 {
			add(domain.Alarm{ID: sw.ID + "-temperature", Severity: "critical", Category: "Temperatur", SwitchID: sw.ID, SwitchName: sw.Name, Title: "Switch zu heiß", Message: fmt.Sprintf("Temperatur %.0f °C.", sw.TemperatureC), Recommendation: "Rackbelüftung und Lüfter sofort prüfen.", CurrentValue: sw.TemperatureC, Threshold: 70})
		} else if sw.TemperatureC >= 60 {
			add(domain.Alarm{ID: sw.ID + "-temperature", Severity: "warning", Category: "Temperatur", SwitchID: sw.ID, SwitchName: sw.Name, Title: "Erhöhte Switch-Temperatur", Message: fmt.Sprintf("Temperatur %.0f °C.", sw.TemperatureC), Recommendation: "Belüftung und Umgebungstemperatur prüfen.", CurrentValue: sw.TemperatureC, Threshold: 60})
		}
		if sw.PoEBudgetWatts > 0 && sw.PoEUsageWatts/sw.PoEBudgetWatts >= .85 {
			percent := sw.PoEUsageWatts / sw.PoEBudgetWatts * 100
			add(domain.Alarm{ID: sw.ID + "-poe", Severity: "warning", Category: "PoE", SwitchID: sw.ID, SwitchName: sw.Name, Title: "PoE-Budget fast ausgeschöpft", Message: fmt.Sprintf("%.0f %% des verfügbaren PoE-Budgets werden genutzt.", percent), Recommendation: "PoE-Last verteilen oder Netzteile der Endgeräte verwenden.", CurrentValue: percent, Threshold: 85})
		}
		for _, port := range sw.Ports {
			portName := port.DisplayName
			if portName == "" {
				portName = fmt.Sprintf("Port %d", port.Index)
			}
			base := domain.Alarm{SwitchID: sw.ID, SwitchName: sw.Name, PortIndex: port.Index, PortName: portName}
			if port.Errors > 0 {
				alarm := base
				alarm.ID = fmt.Sprintf("%s-%d-errors", sw.ID, port.Index)
				alarm.Severity = "warning"
				alarm.Category = "Paketfehler"
				alarm.Title = "Paketfehler erkannt"
				alarm.Message = fmt.Sprintf("%s meldet %d Fehler.", portName, port.Errors)
				alarm.Recommendation = "Kabel, Steckverbindungen und Linkpartner prüfen."
				alarm.CurrentValue = float64(port.Errors)
				add(alarm)
			}
			if !port.Link {
				continue
			}
			capacity := float64(port.SpeedMbps)
			if capacity <= 0 {
				capacity = 1000
			}
			utilization := math.Max(port.RxMbps, port.TxMbps) / capacity * 100
			threshold := 85.0
			if port.RoleID == "dante" || port.RoleID == "trunk" {
				threshold = 70
			}
			if utilization >= threshold {
				alarm := base
				alarm.ID = fmt.Sprintf("%s-%d-utilization", sw.ID, port.Index)
				alarm.Category = "Auslastung"
				alarm.Title = "Port stark ausgelastet"
				alarm.Message = fmt.Sprintf("%s erreicht %.1f %% der Linkkapazität.", portName, utilization)
				alarm.Recommendation = "Traffic verteilen oder schnelleren/zusätzlichen Uplink verwenden."
				alarm.CurrentValue = utilization
				alarm.Threshold = threshold
				if utilization >= 90 {
					alarm.Severity = "critical"
				} else {
					alarm.Severity = "warning"
				}
				add(alarm)
			}
			if port.RoleID == "dante" && port.SpeedMbps > 0 && port.SpeedMbps < 1000 {
				alarm := base
				alarm.ID = fmt.Sprintf("%s-%d-dante-speed", sw.ID, port.Index)
				alarm.Severity = "warning"
				alarm.Category = "Dante/Audio"
				alarm.Title = "Audio-Port nur mit 100 Mbit verbunden"
				alarm.Message = fmt.Sprintf("%s handelt nur %d Mbit/s aus.", portName, port.SpeedMbps)
				alarm.Recommendation = "Kabel und Endgerät prüfen; für Dante wird 1 Gbit/s empfohlen."
				alarm.CurrentValue = float64(port.SpeedMbps)
				alarm.Threshold = 1000
				add(alarm)
			}
			if port.RoleID == "lighting" && port.SpeedMbps > 0 && port.SpeedMbps < 1000 {
				alarm := base
				alarm.ID = fmt.Sprintf("%s-%d-lighting-speed", sw.ID, port.Index)
				alarm.Severity = "warning"
				alarm.Category = "Lighting"
				alarm.Title = "Lighting-Port unter 1 Gbit/s"
				alarm.Message = fmt.Sprintf("%s handelt nur %d Mbit/s aus.", portName, port.SpeedMbps)
				alarm.Recommendation = "Kabel und Endgerät prüfen; MA-Net3 setzt 1 Gbit/s voraus."
				alarm.CurrentValue = float64(port.SpeedMbps)
				alarm.Threshold = 1000
				add(alarm)
			}
		}
	}
	for id := range m.firstSeen {
		if !active[id] {
			delete(m.firstSeen, id)
		}
	}
	sort.Slice(report.Alarms, func(i, j int) bool {
		rank := map[string]int{"critical": 0, "warning": 1, "info": 2}
		if rank[report.Alarms[i].Severity] != rank[report.Alarms[j].Severity] {
			return rank[report.Alarms[i].Severity] < rank[report.Alarms[j].Severity]
		}
		return report.Alarms[i].SwitchName < report.Alarms[j].SwitchName
	})
	report.HealthPercent = 100 - report.CriticalCount*20 - report.WarningCount*5 - report.InfoCount
	if report.HealthPercent < 0 {
		report.HealthPercent = 0
	}
	if len(topology.Switches) > 0 && report.CriticalCount == 0 {
		report.ChecksOK = append(report.ChecksOK, fmt.Sprintf("%d/%d Switches erreichbar", len(topology.Switches), len(topology.Switches)))
	}
	if !hasCategory(report.Alarms, "Paketfehler") {
		report.ChecksOK = append(report.ChecksOK, "Keine Portfehler erkannt")
	}
	if !hasCategory(report.Alarms, "Auslastung") {
		report.ChecksOK = append(report.ChecksOK, "Keine Ports über dem sicheren Auslastungswert")
	}
	if !hasCategory(report.Alarms, "PoE") {
		report.ChecksOK = append(report.ChecksOK, "PoE-Budget im sicheren Bereich")
	}
	return report
}

func observeLinks(topology domain.Topology) map[string]observedLink {
	devices := map[string]string{}
	for _, device := range topology.Devices {
		name := device.Name
		if device.IPAddress != "" && device.IPAddress != name {
			name += " · " + device.IPAddress
		}
		devices[fmt.Sprintf("%s-%d", device.SwitchID, device.PortIndex)] = name
	}
	result := map[string]observedLink{}
	for _, sw := range topology.Switches {
		if sw.Status != "online" {
			continue
		}
		for _, port := range sw.Ports {
			key := fmt.Sprintf("%s-%d", sw.ID, port.Index)
			name := port.DisplayName
			if name == "" {
				name = fmt.Sprintf("Port %d", port.Index)
			}
			result[key] = observedLink{up: port.Link, switchID: sw.ID, switchName: sw.Name, portIndex: port.Index, portName: name, deviceName: devices[key]}
		}
	}
	return result
}

func hasCategory(alarms []domain.Alarm, category string) bool {
	for _, alarm := range alarms {
		if alarm.Category == category {
			return true
		}
	}
	return false
}

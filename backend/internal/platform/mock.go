package platform

import (
	"context"
	"fmt"
	"sync"
	"time"

	"event-network-manager/backend/internal/domain"
)

type MockAdapter struct {
	mu            sync.Mutex
	snapshots     []domain.Snapshot
	profiles      []domain.RoleProfile
	settings      []domain.PortSetting
	switchNames   map[string]string
	roleRollbacks map[string][]domain.PortSetting
}

func (m *MockAdapter) Status(context.Context, string) (domain.ConfigStatus, error) {
	return domain.ConfigStatus{SwitchID: "demo", Available: true, HostKeyTrusted: true, Message: "Demo-Konfiguration verfügbar"}, nil
}
func (m *MockAdapter) TrustHostKey(context.Context, domain.HostKeyTrust) (domain.ConfigStatus, error) {
	return domain.ConfigStatus{SwitchID: "demo", Available: true, HostKeyTrusted: true}, nil
}
func (m *MockAdapter) DantePlan(_ context.Context, request domain.DantePlanRequest) (domain.ConfigPlan, error) {
	return buildDantePlan(request)
}
func (m *MockAdapter) VLANPlan(_ context.Context, request domain.VLANPlanRequest) (domain.ConfigPlan, error) {
	return buildVLANPlan(request)
}
func (m *MockAdapter) DanteHealth(_ context.Context, request domain.DanteHealthRequest) (domain.DanteHealth, error) {
	ports, err := normalizePorts(request.Ports, 24)
	if err != nil {
		return domain.DanteHealth{}, err
	}
	configuration := fmt.Sprintf("ip igmp snooping\nip igmp snooping vlan %d\nqos trust dscp\n", request.VLANID)
	for _, port := range ports {
		configuration += fmt.Sprintf("interface gi%d\n qos trust\n no eee enable\n exit\n", port)
	}
	return inspectDanteConfiguration(request.SwitchID, request.VLANID, ports, configuration), nil
}
func (m *MockAdapter) EventBaselineStatus(_ context.Context, switchID string, _ []domain.RoleProfile) (domain.EventBaselineStatus, error) {
	checks := []domain.EventBaselineCheck{
		{ID: "networks", Title: "Rollen-Netzwerke", Description: "Alle benötigten Netzwerke sind vorbereitet.", OK: true},
		{ID: "multicast", Title: "Multicast", Description: "IGMP Snooping und Querier sind aktiv.", OK: true},
		{ID: "qos", Title: "Audio-Priorisierung", Description: "Dante-Pakete werden priorisiert.", OK: true},
		{ID: "eee", Title: "Stabile Links", Description: "EEE ist deaktiviert.", OK: true},
	}
	return domain.EventBaselineStatus{SwitchID: switchID, CheckedAt: time.Now(), Healthy: true, Checks: checks}, nil
}
func (m *MockAdapter) CaptureSnapshot(ctx context.Context, switchID string) (domain.Snapshot, error) {
	s := domain.Snapshot{ID: fmt.Sprintf("snap-%d", time.Now().UnixNano()), SwitchID: switchID, CreatedAt: time.Now(), Configuration: "! mock running-config", SizeBytes: len("! mock running-config")}
	return s, m.Save(ctx, s)
}

func NewMockServices() Services {
	m := &MockAdapter{profiles: domain.DefaultRoleProfiles(), switchNames: map[string]string{}, roleRollbacks: map[string][]domain.PortSetting{}}
	return Services{Mode: "mock", Discovery: m, Telemetry: m, Inventory: m, Configurator: m, Snapshots: m, Preferences: m}
}
func (m *MockAdapter) ConfigurationState(_ context.Context, switchID string, _ []domain.RoleProfile) (domain.SwitchConfigState, error) {
	settings, _ := m.PortSettings(context.Background(), switchID)
	name, _, _ := m.SwitchDisplayName(context.Background(), switchID)
	return domain.SwitchConfigState{SwitchID: switchID, Name: name, PortSettings: settings}, nil
}
func (m *MockAdapter) ConnectedDevices(context.Context, string) ([]domain.ConnectedDevice, error) {
	return []domain.ConnectedDevice{{ID: "device-console", SwitchID: "foh", PortIndex: 2, Name: "Yamaha CL5", IPAddress: "192.168.50.20", MACAddress: "00:11:22:33:44:55", Model: "Dante Console", SuggestedRole: "Dante/Audio", Protocol: "LLDP/MAC"}}, nil
}

func (m *MockAdapter) Discover(ctx context.Context) ([]domain.Switch, error) {
	t, err := m.Topology(ctx)
	return t.Switches, err
}

func (m *MockAdapter) Topology(context.Context) (domain.Topology, error) {
	now := time.Now()
	return domain.Topology{UpdatedAt: now, Source: "Demo", Switches: []domain.Switch{
		{ID: "foh", Name: "FOH Core", Model: "SG350-28P", Address: "192.168.50.2", Status: "online", CPUPercent: 18, TemperatureC: 42, Ports: mockPorts(28, 1)},
		{ID: "stage", Name: "Stage Left", Model: "SG350-28", Address: "192.168.50.3", Status: "online", CPUPercent: 11, TemperatureC: 39, Ports: mockPorts(28, 2)},
		{ID: "video", Name: "Video Rack", Model: "SG350-28P", Address: "192.168.50.4", Status: "warning", CPUPercent: 27, TemperatureC: 47, Ports: mockPorts(28, 3)},
	}, Links: []domain.Link{
		{ID: "foh-stage", SourceSwitchID: "foh", SourcePort: 25, TargetSwitchID: "stage", TargetPort: 25, Protocol: "LLDP"},
		{ID: "foh-video", SourceSwitchID: "foh", SourcePort: 26, TargetSwitchID: "video", TargetPort: 25, Protocol: "CDP"},
	}}, nil
}

func mockPorts(count, seed int) []domain.Port {
	ports := make([]domain.Port, count)
	roles := []string{"Dante", "Control", "Lighting", "Video", "Unused"}
	for i := range ports {
		idx := i + 1
		link := idx <= 10 || idx >= 25
		role := roles[(idx+seed)%len(roles)]
		ports[i] = domain.Port{Index: idx, Name: fmt.Sprintf("gi%d", idx), DisplayName: fmt.Sprintf("Port %d", idx), Link: link, SpeedMbps: 1000, Role: role, VLANs: []int{10 + (idx%4)*10}, RxMbps: float64((idx*seed*7)%90) / 10, TxMbps: float64((idx*seed*11)%70) / 10}
		if role == "Dante" && link {
			ports[i].PoEWatts = 6.4
		}
	}
	return ports
}

func (m *MockAdapter) Apply(ctx context.Context, change domain.ConfigChange) (domain.Snapshot, error) {
	// The production adapter must retrieve and persist running-config before SSH.
	s := domain.Snapshot{ID: fmt.Sprintf("snap-%d", time.Now().UnixNano()), SwitchID: change.SwitchID, CreatedAt: time.Now(), Configuration: "! mock running-config before: " + change.Description}
	s.SizeBytes = len(s.Configuration)
	if err := m.Save(ctx, s); err != nil {
		return domain.Snapshot{}, err
	}
	return s, nil
}

func (m *MockAdapter) SaveStartup(context.Context, string) error { return nil }
func (m *MockAdapter) Rollback(context.Context, string) error    { return nil }
func (m *MockAdapter) Save(_ context.Context, s domain.Snapshot) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.snapshots = append(m.snapshots, s)
	return nil
}
func (m *MockAdapter) List(_ context.Context, switchID string) ([]domain.Snapshot, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []domain.Snapshot{}
	for _, s := range m.snapshots {
		if switchID == "" || s.SwitchID == switchID {
			out = append(out, s)
		}
	}
	return out, nil
}

func (m *MockAdapter) RoleProfiles(context.Context) ([]domain.RoleProfile, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]domain.RoleProfile(nil), m.profiles...), nil
}
func (m *MockAdapter) SaveRoleProfiles(_ context.Context, profiles []domain.RoleProfile) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.profiles = append([]domain.RoleProfile(nil), profiles...)
	return nil
}
func (m *MockAdapter) PortSettings(_ context.Context, switchID string) ([]domain.PortSetting, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []domain.PortSetting{}
	for _, setting := range m.settings {
		if setting.SwitchID == switchID {
			out = append(out, setting)
		}
	}
	return out, nil
}
func (m *MockAdapter) SavePortSettings(_ context.Context, settings []domain.PortSetting) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, setting := range settings {
		updated := false
		for i := range m.settings {
			if m.settings[i].SwitchID == setting.SwitchID && m.settings[i].PortIndex == setting.PortIndex {
				m.settings[i] = setting
				updated = true
			}
		}
		if !updated {
			m.settings = append(m.settings, setting)
		}
	}
	return nil
}
func (m *MockAdapter) SwitchDisplayName(_ context.Context, switchID string) (string, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	name, ok := m.switchNames[switchID]
	return name, ok, nil
}
func (m *MockAdapter) SaveSwitchDisplayName(_ context.Context, switchID, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.switchNames[switchID] = name
	return nil
}
func (m *MockAdapter) SaveRoleSettingRollback(_ context.Context, snapshotID string, settings []domain.PortSetting) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.roleRollbacks[snapshotID] = append([]domain.PortSetting(nil), settings...)
	return nil
}
func (m *MockAdapter) RestoreRoleSettings(_ context.Context, snapshotID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, before := range m.roleRollbacks[snapshotID] {
		kept := m.settings[:0]
		for _, current := range m.settings {
			if current.SwitchID != before.SwitchID || current.PortIndex != before.PortIndex {
				kept = append(kept, current)
			}
		}
		m.settings = kept
		if before.DisplayName != "" || before.RoleID != "" {
			m.settings = append(m.settings, before)
		}
	}
	return nil
}

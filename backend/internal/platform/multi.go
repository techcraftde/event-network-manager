package platform

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"event-network-manager/backend/internal/domain"
)

type MultiAdapter struct {
	mu               sync.RWMutex
	topologyMu       sync.Mutex
	scanMu           sync.Mutex
	devices          []Services
	knownSwitches    map[string]domain.Switch
	snapshots        SnapshotStore
	preferences      RoleStore
	allowInsecureTLS bool
}

func NewMultiServices(devices []Services, snapshots SnapshotStore) Services {
	m := &MultiAdapter{devices: devices, snapshots: snapshots, allowInsecureTLS: true}
	return Services{Mode: "sg350-multi", Discovery: m, Telemetry: m, Inventory: m, StateReader: m, Configurator: m, Snapshots: snapshots}
}

func NewManagedServices(devices []Services, snapshots SnapshotStore, preferences RoleStore, allowInsecureTLS bool) Services {
	m := &MultiAdapter{devices: devices, snapshots: snapshots, preferences: preferences, allowInsecureTLS: allowInsecureTLS}
	return Services{Mode: "sg350-managed", Discovery: m, Telemetry: m, Inventory: m, StateReader: m, Configurator: m, Snapshots: snapshots, Preferences: preferences}
}

func (m *MultiAdapter) devicesSnapshot() []Services {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return append([]Services(nil), m.devices...)
}

func (m *MultiAdapter) ConfigurationState(ctx context.Context, switchID string, profiles []domain.RoleProfile) (domain.SwitchConfigState, error) {
	for _, device := range m.devicesSnapshot() {
		if device.StateReader == nil {
			continue
		}
		state, err := device.StateReader.ConfigurationState(ctx, switchID, profiles)
		if err == nil && state.SwitchID == switchID {
			return state, nil
		}
	}
	return domain.SwitchConfigState{}, fmt.Errorf("Konfiguration für Switch %q ist nicht lesbar", switchID)
}

func (m *MultiAdapter) ConnectedDevices(ctx context.Context, switchID string) ([]domain.ConnectedDevice, error) {
	for _, device := range m.devicesSnapshot() {
		if device.Inventory == nil {
			continue
		}
		items, err := device.Inventory.ConnectedDevices(ctx, switchID)
		if err == nil {
			return items, nil
		}
	}
	return []domain.ConnectedDevice{}, nil
}

func (m *MultiAdapter) Discover(ctx context.Context) ([]domain.Switch, error) {
	topology, err := m.Topology(ctx)
	return topology.Switches, err
}

func (m *MultiAdapter) Topology(ctx context.Context) (domain.Topology, error) {
	m.topologyMu.Lock()
	defer m.topologyMu.Unlock()
	type result struct {
		topology domain.Topology
		err      error
	}
	devices := m.devicesSnapshot()
	results := make(chan result, len(devices))
	var wg sync.WaitGroup
	for _, device := range devices {
		wg.Add(1)
		go func(service Services) {
			defer wg.Done()
			topology, err := service.Telemetry.Topology(ctx)
			results <- result{topology, err}
		}(device)
	}
	wg.Wait()
	close(results)
	combined := domain.Topology{Switches: []domain.Switch{}, Links: []domain.Link{}, VLANs: []domain.VLAN{}, Devices: []domain.ConnectedDevice{}, UpdatedAt: time.Now(), Source: "Cisco HTTPS · Multi-Switch live"}
	vlans := map[int]domain.VLAN{}
	var errs []error
	for result := range results {
		if result.err != nil {
			errs = append(errs, result.err)
			continue
		}
		combined.Switches = append(combined.Switches, result.topology.Switches...)
		combined.Links = append(combined.Links, result.topology.Links...)
		combined.Devices = append(combined.Devices, result.topology.Devices...)
		for _, vlan := range result.topology.VLANs {
			vlans[vlan.ID] = vlan
		}
	}
	for _, vlan := range vlans {
		combined.VLANs = append(combined.VLANs, vlan)
	}
	if m.knownSwitches == nil {
		m.knownSwitches = map[string]domain.Switch{}
	}
	current := map[string]bool{}
	for _, sw := range combined.Switches {
		current[sw.ID] = true
		m.knownSwitches[sw.ID] = sw
	}
	for id, known := range m.knownSwitches {
		if current[id] {
			continue
		}
		known.Status = "offline"
		for i := range known.Ports {
			known.Ports[i].Link = false
			known.Ports[i].RxMbps = 0
			known.Ports[i].TxMbps = 0
		}
		combined.Switches = append(combined.Switches, known)
	}
	sort.Slice(combined.VLANs, func(i, j int) bool { return combined.VLANs[i].ID < combined.VLANs[j].ID })
	// Device polling runs concurrently. Always normalize the returned order so
	// cards and graphs never jump around merely because one switch answered first.
	sort.Slice(combined.Switches, func(i, j int) bool {
		if combined.Switches[i].Address != combined.Switches[j].Address {
			return ipLess(combined.Switches[i].Address, combined.Switches[j].Address)
		}
		return combined.Switches[i].ID < combined.Switches[j].ID
	})
	for i := range combined.Switches {
		sort.Slice(combined.Switches[i].Ports, func(a, b int) bool { return combined.Switches[i].Ports[a].Index < combined.Switches[i].Ports[b].Index })
	}
	sort.Slice(combined.Devices, func(i, j int) bool {
		if combined.Devices[i].SwitchID != combined.Devices[j].SwitchID {
			return combined.Devices[i].SwitchID < combined.Devices[j].SwitchID
		}
		if combined.Devices[i].PortIndex != combined.Devices[j].PortIndex {
			return combined.Devices[i].PortIndex < combined.Devices[j].PortIndex
		}
		return combined.Devices[i].ID < combined.Devices[j].ID
	})
	// LLDP/CDP initially reports another managed switch as a neighbor device.
	// Reconcile it with a switch already known by this multi-switch adapter so
	// frontend links terminate on the real switch card rather than in free space.
	switchByName := map[string]string{}
	for _, sw := range combined.Switches {
		switchByName[safeID(sw.Name)] = sw.ID
	}
	resolvedDevices := map[string]bool{}
	for i := range combined.Links {
		if combined.Links[i].TargetSwitchID != "" || combined.Links[i].TargetDeviceID == "" {
			continue
		}
		for _, device := range combined.Devices {
			if device.ID != combined.Links[i].TargetDeviceID {
				continue
			}
			if targetID := switchByName[safeID(device.Name)]; targetID != "" && targetID != combined.Links[i].SourceSwitchID {
				combined.Links[i].TargetSwitchID = targetID
				resolvedDevices[device.ID] = true
			}
			break
		}
	}
	if len(resolvedDevices) > 0 {
		kept := combined.Devices[:0]
		for _, device := range combined.Devices {
			if !resolvedDevices[device.ID] {
				kept = append(kept, device)
			}
		}
		combined.Devices = kept
	}
	sort.Slice(combined.Links, func(i, j int) bool { return combined.Links[i].ID < combined.Links[j].ID })
	if len(combined.Switches) == 0 && len(errs) > 0 {
		return combined, errors.Join(errs...)
	}
	return combined, nil
}

func (m *MultiAdapter) Identify(ctx context.Context, switchID string, durationSeconds int) error {
	target, err := m.target(ctx, switchID)
	if err != nil {
		return err
	}
	identifier, ok := target.(SwitchIdentifier)
	if !ok {
		return errors.New("Identify wird von diesem Switch-Adapter nicht unterstützt")
	}
	return identifier.Identify(ctx, switchID, durationSeconds)
}

func (m *MultiAdapter) target(ctx context.Context, switchID string) (Configurator, error) {
	for _, device := range m.devicesSnapshot() {
		status, err := device.Configurator.Status(ctx, switchID)
		if err == nil && status.SwitchID == switchID {
			return device.Configurator, nil
		}
	}
	return nil, fmt.Errorf("unknown switch %q", switchID)
}

func (m *MultiAdapter) Status(ctx context.Context, switchID string) (domain.ConfigStatus, error) {
	target, err := m.target(ctx, switchID)
	if err != nil {
		return domain.ConfigStatus{}, err
	}
	return target.Status(ctx, switchID)
}
func (m *MultiAdapter) TrustHostKey(ctx context.Context, trust domain.HostKeyTrust) (domain.ConfigStatus, error) {
	target, err := m.target(ctx, trust.SwitchID)
	if err != nil {
		return domain.ConfigStatus{}, err
	}
	return target.TrustHostKey(ctx, trust)
}
func (m *MultiAdapter) DantePlan(ctx context.Context, request domain.DantePlanRequest) (domain.ConfigPlan, error) {
	target, err := m.target(ctx, request.SwitchID)
	if err != nil {
		return domain.ConfigPlan{}, err
	}
	return target.DantePlan(ctx, request)
}
func (m *MultiAdapter) VLANPlan(ctx context.Context, request domain.VLANPlanRequest) (domain.ConfigPlan, error) {
	target, err := m.target(ctx, request.SwitchID)
	if err != nil {
		return domain.ConfigPlan{}, err
	}
	return target.VLANPlan(ctx, request)
}
func (m *MultiAdapter) DanteHealth(ctx context.Context, request domain.DanteHealthRequest) (domain.DanteHealth, error) {
	target, err := m.target(ctx, request.SwitchID)
	if err != nil {
		return domain.DanteHealth{}, err
	}
	return target.DanteHealth(ctx, request)
}
func (m *MultiAdapter) EventBaselineStatus(ctx context.Context, switchID string, profiles []domain.RoleProfile) (domain.EventBaselineStatus, error) {
	target, err := m.target(ctx, switchID)
	if err != nil {
		return domain.EventBaselineStatus{}, err
	}
	return target.EventBaselineStatus(ctx, switchID, profiles)
}
func (m *MultiAdapter) ReferenceResetPlan(ctx context.Context, switchID string) (domain.ConfigPlan, error) {
	target, err := m.target(ctx, switchID)
	if err != nil {
		return domain.ConfigPlan{}, err
	}
	planner, ok := target.(ReferenceResetPlanner)
	if !ok {
		return domain.ConfigPlan{}, errors.New("ME-Standard-Wiederherstellung ist für diesen Switch nicht verfügbar")
	}
	return planner.ReferenceResetPlan(ctx, switchID)
}
func (m *MultiAdapter) CaptureSnapshot(ctx context.Context, switchID string) (domain.Snapshot, error) {
	target, err := m.target(ctx, switchID)
	if err != nil {
		return domain.Snapshot{}, err
	}
	return target.CaptureSnapshot(ctx, switchID)
}
func (m *MultiAdapter) SaveStartup(ctx context.Context, switchID string) error {
	target, err := m.target(ctx, switchID)
	if err != nil {
		return err
	}
	return target.SaveStartup(ctx, switchID)
}
func (m *MultiAdapter) Apply(ctx context.Context, change domain.ConfigChange) (domain.Snapshot, error) {
	target, err := m.target(ctx, change.SwitchID)
	if err != nil {
		return domain.Snapshot{}, err
	}
	return target.Apply(ctx, change)
}
func (m *MultiAdapter) Rollback(ctx context.Context, snapshotID string) error {
	snapshots, err := m.snapshots.List(ctx, "")
	if err != nil {
		return err
	}
	for _, snapshot := range snapshots {
		if snapshot.ID == snapshotID {
			target, targetErr := m.target(ctx, snapshot.SwitchID)
			if targetErr != nil {
				return targetErr
			}
			return target.Rollback(ctx, snapshotID)
		}
	}
	return fmt.Errorf("unknown snapshot %q", snapshotID)
}

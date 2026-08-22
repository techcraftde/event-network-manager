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
	devices   []Services
	snapshots SnapshotStore
}

func NewMultiServices(devices []Services, snapshots SnapshotStore) Services {
	m := &MultiAdapter{devices: devices, snapshots: snapshots}
	return Services{Mode: "sg350-multi", Discovery: m, Telemetry: m, Inventory: m, StateReader: m, Configurator: m, Snapshots: snapshots}
}

func (m *MultiAdapter) ConfigurationState(ctx context.Context, switchID string, profiles []domain.RoleProfile) (domain.SwitchConfigState, error) {
	for _, device := range m.devices {
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
	for _, device := range m.devices {
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
	type result struct {
		topology domain.Topology
		err      error
	}
	results := make(chan result, len(m.devices))
	var wg sync.WaitGroup
	for _, device := range m.devices {
		wg.Add(1)
		go func(service Services) {
			defer wg.Done()
			topology, err := service.Telemetry.Topology(ctx)
			results <- result{topology, err}
		}(device)
	}
	wg.Wait()
	close(results)
	combined := domain.Topology{UpdatedAt: time.Now(), Source: "Cisco HTTPS · Multi-Switch live"}
	vlans := map[int]domain.VLAN{}
	var errs []error
	for result := range results {
		if result.err != nil {
			errs = append(errs, result.err)
			continue
		}
		combined.Switches = append(combined.Switches, result.topology.Switches...)
		combined.Links = append(combined.Links, result.topology.Links...)
		for _, vlan := range result.topology.VLANs {
			vlans[vlan.ID] = vlan
		}
	}
	for _, vlan := range vlans {
		combined.VLANs = append(combined.VLANs, vlan)
	}
	sort.Slice(combined.VLANs, func(i, j int) bool { return combined.VLANs[i].ID < combined.VLANs[j].ID })
	if len(combined.Switches) == 0 && len(errs) > 0 {
		return combined, errors.Join(errs...)
	}
	return combined, nil
}

func (m *MultiAdapter) target(ctx context.Context, switchID string) (Configurator, error) {
	for _, device := range m.devices {
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

package platform

import (
	"context"
	"event-network-manager/backend/internal/domain"
)

// These narrow interfaces are the boundary between product logic and devices.
// Mock, local SG350 and future remote-agent implementations are interchangeable.
type Discovery interface {
	Discover(context.Context) ([]domain.Switch, error)
}
type Telemetry interface {
	Topology(context.Context) (domain.Topology, error)
}
type DeviceInventory interface {
	ConnectedDevices(context.Context, string) ([]domain.ConnectedDevice, error)
}
type ConfigStateReader interface {
	ConfigurationState(context.Context, string, []domain.RoleProfile) (domain.SwitchConfigState, error)
}
type Configurator interface {
	Status(context.Context, string) (domain.ConfigStatus, error)
	TrustHostKey(context.Context, domain.HostKeyTrust) (domain.ConfigStatus, error)
	DantePlan(context.Context, domain.DantePlanRequest) (domain.ConfigPlan, error)
	VLANPlan(context.Context, domain.VLANPlanRequest) (domain.ConfigPlan, error)
	DanteHealth(context.Context, domain.DanteHealthRequest) (domain.DanteHealth, error)
	EventBaselineStatus(context.Context, string, []domain.RoleProfile) (domain.EventBaselineStatus, error)
	CaptureSnapshot(context.Context, string) (domain.Snapshot, error)
	SaveStartup(context.Context, string) error
	Apply(context.Context, domain.ConfigChange) (domain.Snapshot, error)
	Rollback(context.Context, string) error
}

// ReferenceResetPlanner is intentionally separate from Configurator so future
// adapters can opt in only when they can read the live configuration first.
// The plan must be built server-side; accepting reset commands from a client
// would undermine the SG350 command allowlist.
type ReferenceResetPlanner interface {
	ReferenceResetPlan(context.Context, string) (domain.ConfigPlan, error)
}
type SnapshotStore interface {
	Save(context.Context, domain.Snapshot) error
	List(context.Context, string) ([]domain.Snapshot, error)
}

type HostKeyStore interface {
	TrustedHostKey(context.Context, string) (algorithm, fingerprint string, publicKey []byte, found bool, err error)
	TrustHostKey(context.Context, string, string, string, []byte) error
}

type RollbackStore interface {
	SaveRollbackCommands(context.Context, string, []string) error
	RollbackCommands(context.Context, string) ([]string, bool, error)
}

type RoleStore interface {
	RoleProfiles(context.Context) ([]domain.RoleProfile, error)
	SaveRoleProfiles(context.Context, []domain.RoleProfile) error
	PortSettings(context.Context, string) ([]domain.PortSetting, error)
	SavePortSettings(context.Context, []domain.PortSetting) error
	SwitchDisplayName(context.Context, string) (string, bool, error)
	SaveSwitchDisplayName(context.Context, string, string) error
	SaveRoleSettingRollback(context.Context, string, []domain.PortSetting) error
	RestoreRoleSettings(context.Context, string) error
}

type Services struct {
	Mode         string
	Discovery    Discovery
	Telemetry    Telemetry
	Inventory    DeviceInventory
	StateReader  ConfigStateReader
	Configurator Configurator
	Snapshots    SnapshotStore
	Preferences  RoleStore
}

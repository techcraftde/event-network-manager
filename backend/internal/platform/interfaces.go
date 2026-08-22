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
type Configurator interface {
	Status(context.Context, string) (domain.ConfigStatus, error)
	TrustHostKey(context.Context, domain.HostKeyTrust) (domain.ConfigStatus, error)
	DantePlan(context.Context, domain.DantePlanRequest) (domain.ConfigPlan, error)
	VLANPlan(context.Context, domain.VLANPlanRequest) (domain.ConfigPlan, error)
	DanteHealth(context.Context, domain.DanteHealthRequest) (domain.DanteHealth, error)
	CaptureSnapshot(context.Context, string) (domain.Snapshot, error)
	Apply(context.Context, domain.ConfigChange) (domain.Snapshot, error)
	Rollback(context.Context, string) error
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

type Services struct {
	Mode         string
	Discovery    Discovery
	Telemetry    Telemetry
	Configurator Configurator
	Snapshots    SnapshotStore
}

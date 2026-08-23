package platform

import (
	"context"
	"errors"
	"testing"

	"event-network-manager/backend/internal/domain"
)

type changingTelemetry struct {
	topology domain.Topology
	fail     bool
}

func (f *changingTelemetry) Topology(context.Context) (domain.Topology, error) {
	if f.fail {
		return domain.Topology{}, errors.New("device unreachable")
	}
	return f.topology, nil
}

func TestMultiTopologyKeepsMissingSwitchOfflineAndStable(t *testing.T) {
	first := &changingTelemetry{topology: domain.Topology{Switches: []domain.Switch{{ID: "later", Name: "Later", Address: "192.168.250.56", Status: "online", Ports: []domain.Port{{Index: 2, Link: true}, {Index: 1, Link: true}}}}}}
	second := &changingTelemetry{topology: domain.Topology{Switches: []domain.Switch{{ID: "earlier", Name: "Earlier", Address: "192.168.250.51", Status: "online"}}}}
	multi := &MultiAdapter{devices: []Services{{Telemetry: first}, {Telemetry: second}}}
	topology, err := multi.Topology(context.Background())
	if err != nil || len(topology.Switches) != 2 || topology.Switches[0].ID != "earlier" || topology.Switches[1].Ports[0].Index != 1 {
		t.Fatalf("first topology=%#v err=%v", topology, err)
	}
	first.fail = true
	topology, err = multi.Topology(context.Background())
	if err != nil || len(topology.Switches) != 2 {
		t.Fatalf("offline topology=%#v err=%v", topology, err)
	}
	if topology.Switches[1].ID != "later" || topology.Switches[1].Status != "offline" {
		t.Fatalf("missing switch was not retained offline: %#v", topology.Switches)
	}
}

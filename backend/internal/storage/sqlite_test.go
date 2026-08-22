package storage

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"event-network-manager/backend/internal/domain"
)

func TestSnapshotRoundTrip(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "enm.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	want := domain.Snapshot{ID: "snap-1", SwitchID: "switch-1", CreatedAt: time.Now().UTC(), Configuration: "hostname test"}
	if err := store.Save(context.Background(), want); err != nil {
		t.Fatal(err)
	}
	got, err := store.List(context.Background(), "switch-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Configuration != want.Configuration {
		t.Fatalf("got %#v", got)
	}
}

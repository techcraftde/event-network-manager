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

func TestRoleAndFriendlyNamesRoundTrip(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "enm.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	profiles, err := store.RoleProfiles(ctx)
	if err != nil || len(profiles) != 7 {
		t.Fatalf("profiles=%d err=%v", len(profiles), err)
	}
	setting := domain.PortSetting{SwitchID: "switch-1", PortIndex: 3, DisplayName: "Lichtpult", RoleID: "lighting"}
	if err := store.SavePortSettings(ctx, []domain.PortSetting{setting}); err != nil {
		t.Fatal(err)
	}
	settings, err := store.PortSettings(ctx, "switch-1")
	if err != nil || len(settings) != 1 || settings[0] != setting {
		t.Fatalf("settings=%#v err=%v", settings, err)
	}
	if err := store.SaveSwitchDisplayName(ctx, "switch-1", "Bühne links"); err != nil {
		t.Fatal(err)
	}
	name, found, err := store.SwitchDisplayName(ctx, "switch-1")
	if err != nil || !found || name != "Bühne links" {
		t.Fatalf("name=%q found=%v err=%v", name, found, err)
	}
	if err := store.SaveRoleSettingRollback(ctx, "snap-role", []domain.PortSetting{setting}); err != nil {
		t.Fatal(err)
	}
	if err := store.SavePortSettings(ctx, []domain.PortSetting{{SwitchID: "switch-1", PortIndex: 3, DisplayName: "Neu", RoleID: "video"}}); err != nil {
		t.Fatal(err)
	}
	if err := store.RestoreRoleSettings(ctx, "snap-role"); err != nil {
		t.Fatal(err)
	}
	restored, err := store.PortSettings(ctx, "switch-1")
	if err != nil || len(restored) != 1 || restored[0] != setting {
		t.Fatalf("restored=%#v err=%v", restored, err)
	}
}

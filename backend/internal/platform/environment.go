package platform

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"os/exec"
	"strings"
	"time"

	"event-network-manager/backend/internal/storage"
)

// NewServicesFromEnvironment keeps credentials outside files and SQLite.
func NewServicesFromEnvironment() Services {
	type target struct {
		Address  string `json:"address"`
		Username string `json:"username"`
		Password string `json:"password"`
	}
	targets := []target{}
	if raw := os.Getenv("ENM_SWITCHES_JSON"); raw != "" {
		if err := json.Unmarshal([]byte(raw), &targets); err != nil {
			log.Printf("invalid ENM_SWITCHES_JSON: %v", err)
		}
	}
	if len(targets) == 0 {
		address, username, password := os.Getenv("ENM_SWITCH_ADDRESS"), os.Getenv("ENM_SWITCH_USERNAME"), os.Getenv("ENM_SWITCH_PASSWORD")
		if password == "" && address != "" {
			password = keychainPassword(address)
		}
		targets = append(targets, target{Address: address, Username: username, Password: password})
	}
	if targets[0].Address == "" || targets[0].Username == "" || targets[0].Password == "" {
		log.Print("device credentials not configured; using demo adapter")
		return NewMockServices()
	}
	store, err := storage.Open(os.Getenv("ENM_DATABASE_PATH"))
	if err != nil {
		log.Printf("SQLite unavailable (%v); using in-memory snapshots", err)
	}
	var snapshots SnapshotStore
	if store != nil {
		snapshots = store
	}
	devices := []Services{}
	for _, target := range targets {
		if target.Address == "" || target.Username == "" || target.Password == "" {
			continue
		}
		services, createErr := NewSG350WebServices(SG350WebConfig{
			Address: target.Address, Username: target.Username, Password: target.Password,
			AllowInsecureTLS: os.Getenv("ENM_SWITCH_VERIFY_TLS") != "1", Snapshots: snapshots,
		})
		if createErr != nil {
			log.Printf("SG350 adapter %s unavailable: %v", target.Address, createErr)
			continue
		}
		devices = append(devices, services)
	}
	if len(devices) == 0 {
		log.Print("no SG350 adapters available; using demo adapter")
		return NewMockServices()
	}
	if len(devices) == 1 {
		return devices[0]
	}
	return NewMultiServices(devices, snapshots)
}

func keychainPassword(address string) string {
	if len(address) > 255 || strings.ContainsAny(address, "\r\n\x00") {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, "/usr/bin/security", "find-generic-password", "-s", "app.eventnetwork.manager", "-a", address, "-w").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

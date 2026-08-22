package platform

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"event-network-manager/backend/internal/domain"
	"golang.org/x/crypto/ssh"
)

type SG350SSHConfig struct {
	SwitchID string
	Address  string
	Username string
	Password string
	Store    interface {
		SnapshotStore
		HostKeyStore
	}
}

type SG350SSHConfigurator struct {
	config         SG350SSHConfig
	mu             sync.Mutex
	lastSnapshotID string
	inventoryMu    sync.Mutex
	inventoryAt    time.Time
	inventory      []domain.ConnectedDevice
}

type observedHostKey struct {
	algorithm, fingerprint string
	publicKey              []byte
}

func NewSG350SSHConfigurator(config SG350SSHConfig) (*SG350SSHConfigurator, error) {
	if config.SwitchID == "" || config.Address == "" || config.Username == "" || config.Password == "" || config.Store == nil {
		return nil, errors.New("complete SSH configuration and persistent store are required")
	}
	return &SG350SSHConfigurator{config: config}, nil
}

func (a *SG350SSHConfigurator) ConnectedDevices(ctx context.Context, switchID string) ([]domain.ConnectedDevice, error) {
	if switchID != "" && switchID != a.config.SwitchID {
		return nil, fmt.Errorf("unknown switch %q", switchID)
	}
	a.inventoryMu.Lock()
	defer a.inventoryMu.Unlock()
	if time.Since(a.inventoryAt) < 30*time.Second {
		return append([]domain.ConnectedDevice(nil), a.inventory...), nil
	}
	client, err := a.dial(ctx)
	if err != nil {
		return nil, err
	}
	defer client.Close()
	output, err := a.run(client, []string{"terminal datadump", "show mac address-table", "show arp"})
	if err != nil {
		return nil, err
	}
	a.inventory = parseConnectedDevices(a.config.SwitchID, output)
	a.inventoryAt = time.Now()
	return append([]domain.ConnectedDevice(nil), a.inventory...), nil
}

var macTableLine = regexp.MustCompile(`(?im)^\s*(?:[1-9][0-9]{0,3}\s+)?([0-9a-f]{2}(?:(?::|-)[0-9a-f]{2}){5})\s+\S+\s+(?:GigabitEthernet|gi)([1-9]|1[0-9]|2[0-8])\s*$`)
var arpLine = regexp.MustCompile(`(?im)^\s*([0-9]{1,3}(?:\.[0-9]{1,3}){3})\s+([0-9a-f]{2}(?:(?::|-)[0-9a-f]{2}){5})(?:\s|$)`)

func parseConnectedDevices(switchID, output string) []domain.ConnectedDevice {
	ips := map[string]string{}
	for _, match := range arpLine.FindAllStringSubmatch(output, -1) {
		ips[normalizeMAC(match[2])] = match[1]
	}
	seen := map[string]bool{}
	result := []domain.ConnectedDevice{}
	for _, match := range macTableLine.FindAllStringSubmatch(output, -1) {
		mac := normalizeMAC(match[1])
		if seen[mac] {
			continue
		}
		seen[mac] = true
		port := 0
		fmt.Sscan(match[2], &port)
		name := ips[mac]
		if name == "" {
			name = mac
		}
		result = append(result, domain.ConnectedDevice{ID: "endpoint-" + safeID(mac), SwitchID: switchID, PortIndex: port, Name: name, IPAddress: ips[mac], MACAddress: mac, Protocol: "MAC/ARP"})
	}
	return result
}
func normalizeMAC(value string) string {
	value = strings.ToLower(strings.ReplaceAll(value, "-", ":"))
	return value
}

func (a *SG350SSHConfigurator) Status(ctx context.Context, switchID string) (domain.ConfigStatus, error) {
	if switchID != "" && switchID != a.config.SwitchID {
		return domain.ConfigStatus{}, fmt.Errorf("unknown switch %q", switchID)
	}
	observed, probeErr := a.probe(ctx)
	status := domain.ConfigStatus{SwitchID: a.config.SwitchID}
	if observed != nil {
		status.HostKeyAlgorithm = observed.algorithm
		status.HostKeyFingerprint = observed.fingerprint
	}
	if probeErr != nil && observed == nil {
		status.Message = probeErr.Error()
		return status, nil
	}
	_, trustedFingerprint, _, found, err := a.config.Store.TrustedHostKey(ctx, a.sshAddress())
	if err != nil {
		return status, err
	}
	status.HostKeyTrusted = found && observed != nil && trustedFingerprint == observed.fingerprint
	if !status.HostKeyTrusted {
		status.Message = "SSH-Hostschlüssel muss geprüft und bestätigt werden"
		return status, nil
	}
	client, err := a.dial(ctx)
	if err != nil {
		status.Message = "Hostschlüssel ist bestätigt, aber die SSH-Anmeldung ist fehlgeschlagen: " + err.Error()
		return status, nil
	}
	_ = client.Close()
	status.Available = true
	status.Message = "SSH-Verbindung erfolgreich geprüft"
	return status, nil
}

func (a *SG350SSHConfigurator) TrustHostKey(ctx context.Context, trust domain.HostKeyTrust) (domain.ConfigStatus, error) {
	if trust.SwitchID != a.config.SwitchID {
		return domain.ConfigStatus{}, fmt.Errorf("unknown switch %q", trust.SwitchID)
	}
	observed, err := a.probe(ctx)
	if observed == nil {
		return domain.ConfigStatus{}, fmt.Errorf("SSH key probe failed: %w", err)
	}
	if trust.Fingerprint == "" || trust.Fingerprint != observed.fingerprint {
		return domain.ConfigStatus{}, errors.New("fingerprint confirmation does not match the key currently presented by the switch")
	}
	if err := a.config.Store.TrustHostKey(ctx, a.sshAddress(), observed.algorithm, observed.fingerprint, observed.publicKey); err != nil {
		return domain.ConfigStatus{}, err
	}
	return a.Status(ctx, trust.SwitchID)
}

func (a *SG350SSHConfigurator) DantePlan(_ context.Context, request domain.DantePlanRequest) (domain.ConfigPlan, error) {
	if request.SwitchID != a.config.SwitchID {
		return domain.ConfigPlan{}, fmt.Errorf("unknown switch %q", request.SwitchID)
	}
	return buildDantePlan(request)
}

func (a *SG350SSHConfigurator) VLANPlan(_ context.Context, request domain.VLANPlanRequest) (domain.ConfigPlan, error) {
	if request.SwitchID != a.config.SwitchID {
		return domain.ConfigPlan{}, fmt.Errorf("unbekannter Switch %q", request.SwitchID)
	}
	return buildVLANPlan(request)
}

func (a *SG350SSHConfigurator) DanteHealth(ctx context.Context, request domain.DanteHealthRequest) (domain.DanteHealth, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if request.SwitchID != a.config.SwitchID {
		return domain.DanteHealth{}, fmt.Errorf("unbekannter Switch %q", request.SwitchID)
	}
	if request.VLANID < 1 || request.VLANID > 4094 {
		return domain.DanteHealth{}, errors.New("VLAN-ID muss zwischen 1 und 4094 liegen")
	}
	ports, err := normalizePorts(request.Ports, 24)
	if err != nil {
		return domain.DanteHealth{}, err
	}
	client, err := a.dial(ctx)
	if err != nil {
		return domain.DanteHealth{}, err
	}
	defer client.Close()
	statusOutput, err := a.run(client, []string{"terminal datadump", fmt.Sprintf("show ip igmp snooping interface %d", request.VLANID), "show qos", "show eee"})
	if err != nil || strings.TrimSpace(statusOutput) == "" {
		return domain.DanteHealth{}, fmt.Errorf("Dante-Zustand konnte nicht geprüft werden: %w", err)
	}
	health := inspectDanteConfiguration(a.config.SwitchID, request.VLANID, ports, statusOutput)
	return health, nil
}

func inspectDanteConfiguration(switchID string, vlanID int, ports []int, configuration string) domain.DanteHealth {
	h := domain.DanteHealth{SwitchID: switchID, VLANID: vlanID, CheckedAt: time.Now().UTC(), SelectedPorts: len(ports)}
	lower := strings.ToLower(configuration)
	h.IGMPGlobal = strings.Contains(lower, "igmp snooping is globally enabled") || configHasGlobal(configuration, "ip igmp snooping")
	h.IGMPVLAN = regexp.MustCompile(fmt.Sprintf(`(?is)vlan\s+%d.*?igmp snooping is enabled`, vlanID)).MatchString(configuration) || configHasGlobal(configuration, fmt.Sprintf("ip igmp snooping vlan %d", vlanID))
	h.QoSDSCP = strings.Contains(lower, "advanced mode trust type: dscp") || configHasGlobal(configuration, "qos trust dscp") || configHasGlobal(configuration, "qos advanced-mode trust dscp")
	allTrusted := strings.Contains(lower, "advanced mode ports state: trusted") || configHasGlobal(configuration, "qos advanced ports-trusted")
	allEEEDisabled := strings.Contains(lower, "eee globally disabled") || configHasGlobal(configuration, "no eee enable")
	eeeEnabled := eeeEnabledPorts(configuration)
	for _, port := range ports {
		iface := fmt.Sprintf("interface gi%d", port)
		if allTrusted || interfaceHas(configuration, iface, "qos trust") {
			h.QoSTrustedPorts++
		}
		if allEEEDisabled || interfaceHas(configuration, iface, "no eee enable") || (eeeEnabled != nil && !eeeEnabled[port]) {
			h.EEEDisabledPorts++
		}
	}
	h.Healthy = h.IGMPGlobal && h.IGMPVLAN && h.QoSDSCP && h.QoSTrustedPorts == h.SelectedPorts && h.EEEDisabledPorts == h.SelectedPorts
	if !h.IGMPGlobal || !h.IGMPVLAN {
		h.Messages = append(h.Messages, "IGMP Snooping ist noch nicht vollständig für das gewählte VLAN eingerichtet.")
	}
	if !h.QoSDSCP || h.QoSTrustedPorts != h.SelectedPorts {
		h.Messages = append(h.Messages, "Die DSCP-Priorisierung fehlt global oder auf mindestens einem gewählten Port.")
	}
	if h.EEEDisabledPorts != h.SelectedPorts {
		h.Messages = append(h.Messages, "EEE ist auf mindestens einem gewählten Dante-Port noch aktiv.")
	}
	if h.Healthy {
		h.Messages = []string{"Alle geprüften Dante-Regeln sind in der laufenden Konfiguration aktiv."}
	}
	return h
}

func eeeEnabledPorts(output string) map[int]bool {
	match := regexp.MustCompile(`(?im)^EEE Administrate status is enabled on ports:\s*(.*)$`).FindStringSubmatch(output)
	if len(match) != 2 {
		return nil
	}
	result := map[int]bool{}
	for _, token := range strings.FieldsFunc(strings.ToLower(match[1]), func(r rune) bool { return r == ',' || r == ' ' }) {
		token = strings.TrimPrefix(token, "gi")
		var start, end int
		if _, err := fmt.Sscanf(token, "%d-%d", &start, &end); err == nil {
			for port := start; port <= end; port++ {
				result[port] = true
			}
			continue
		}
		if _, err := fmt.Sscanf(token, "%d", &start); err == nil {
			result[start] = true
		}
	}
	return result
}

func configHasGlobal(configuration, wanted string) bool {
	for _, line := range strings.Split(strings.ReplaceAll(configuration, "\r", ""), "\n") {
		if line == strings.TrimSpace(line) && strings.TrimSpace(line) == wanted {
			return true
		}
	}
	return false
}

func (a *SG350SSHConfigurator) CaptureSnapshot(ctx context.Context, switchID string) (domain.Snapshot, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if switchID != a.config.SwitchID {
		return domain.Snapshot{}, fmt.Errorf("unknown switch %q", switchID)
	}
	client, err := a.dial(ctx)
	if err != nil {
		return domain.Snapshot{}, err
	}
	defer client.Close()
	return a.captureWithClient(ctx, client)
}

func (a *SG350SSHConfigurator) captureWithClient(ctx context.Context, client *ssh.Client) (domain.Snapshot, error) {
	running, err := a.run(client, []string{"terminal datadump", "show running-config"})
	if err != nil {
		return domain.Snapshot{}, fmt.Errorf("read running configuration: %w", err)
	}
	if strings.TrimSpace(running) == "" {
		return domain.Snapshot{}, errors.New("switch returned an empty running configuration")
	}
	snapshot := domain.Snapshot{ID: fmt.Sprintf("snap-%d", time.Now().UTC().UnixNano()), SwitchID: a.config.SwitchID, CreatedAt: time.Now().UTC(), Configuration: running, SizeBytes: len(running)}
	if err := a.config.Store.Save(ctx, snapshot); err != nil {
		return domain.Snapshot{}, fmt.Errorf("persist snapshot: %w", err)
	}
	return snapshot, nil
}

func (a *SG350SSHConfigurator) Apply(ctx context.Context, change domain.ConfigChange) (domain.Snapshot, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if change.SwitchID != a.config.SwitchID {
		return domain.Snapshot{}, fmt.Errorf("unknown switch %q", change.SwitchID)
	}
	if err := validateConfigCommands(change.Commands); err != nil {
		return domain.Snapshot{}, err
	}
	client, err := a.dial(ctx)
	if err != nil {
		return domain.Snapshot{}, err
	}
	snapshot, err := a.captureWithClient(ctx, client)
	_ = client.Close()
	if err != nil {
		return domain.Snapshot{}, err
	}
	rollbackCommands := buildRollbackCommands(snapshot.Configuration, change.Commands)
	rollbackStore, ok := a.config.Store.(RollbackStore)
	if !ok {
		return domain.Snapshot{}, errors.New("persistent rollback storage is unavailable")
	}
	if err := rollbackStore.SaveRollbackCommands(ctx, snapshot.ID, rollbackCommands); err != nil {
		return domain.Snapshot{}, fmt.Errorf("persist rollback plan: %w", err)
	}
	// Older SG350 SSH servers are unreliable when several shell channels are
	// opened on one transport. Use a fresh connection for each phase.
	client, err = a.dial(ctx)
	if err != nil {
		return snapshot, err
	}
	output, err := a.run(client, append([]string{"terminal datadump"}, change.Commands...))
	_ = client.Close()
	if os.Getenv("ENM_DEBUG_SSH") == "1" {
		log.Printf("SG350 apply output: %q", output)
	}
	if err != nil || cliErrorPattern.MatchString(output) {
		rollbackErr := a.executeRollback(ctx, rollbackCommands)
		if rollbackErr == nil {
			return snapshot, fmt.Errorf("configuration rejected and was automatically rolled back: %s", cleanCLIError(output, err))
		}
		return snapshot, fmt.Errorf("configuration rejected and automatic rollback failed (%v): %s", rollbackErr, cleanCLIError(output, err))
	}
	client, err = a.dial(ctx)
	if err != nil {
		return snapshot, err
	}
	verify, err := a.run(client, []string{"terminal datadump", "show running-config"})
	_ = client.Close()
	if err != nil || strings.TrimSpace(verify) == "" {
		return snapshot, errors.New("configuration verification failed; running configuration was not saved")
	}
	if err := verifyAppliedConfiguration(change.Commands, verify); err != nil {
		rollbackErr := a.executeRollback(ctx, rollbackCommands)
		if rollbackErr == nil {
			return snapshot, fmt.Errorf("configuration verification failed and the change was automatically rolled back: %w", err)
		}
		return snapshot, fmt.Errorf("configuration verification failed (%v) and automatic rollback failed: %w", err, rollbackErr)
	}
	client, err = a.dial(ctx)
	if err != nil {
		return snapshot, err
	}
	if _, err := a.run(client, []string{"terminal datadump", "copy running-config startup-config", "y"}); err != nil {
		_ = client.Close()
		return snapshot, fmt.Errorf("change is active but could not be saved to startup configuration: %w", err)
	}
	_ = client.Close()
	a.lastSnapshotID = snapshot.ID
	return snapshot, nil
}

func (a *SG350SSHConfigurator) executeRollback(ctx context.Context, commands []string) error {
	client, err := a.dial(ctx)
	if err != nil {
		return err
	}
	output, runErr := a.run(client, append([]string{"terminal datadump"}, commands...))
	_ = client.Close()
	if runErr != nil || cliErrorPattern.MatchString(output) {
		return fmt.Errorf("rollback commands rejected: %s", cleanCLIError(output, runErr))
	}
	client, err = a.dial(ctx)
	if err != nil {
		return err
	}
	_, err = a.run(client, []string{"terminal datadump", "copy running-config startup-config", "y"})
	_ = client.Close()
	return err
}

func verifyAppliedConfiguration(commands []string, configuration string) error {
	currentPort := ""
	for _, command := range commands {
		switch {
		case strings.HasPrefix(command, "interface gi"):
			currentPort = command
		case strings.HasPrefix(command, "vlan ") && strings.Contains(command, " name "):
			fields := strings.Fields(command)
			if len(fields) < 2 || !configurationHasVLAN(configuration, fields[1]) {
				return fmt.Errorf("angelegter Netzwerkbereich ist nicht aktiv")
			}
		case strings.HasPrefix(command, "description ") && currentPort != "":
			if !interfaceHas(configuration, currentPort, command) {
				return fmt.Errorf("Portname wurde nicht übernommen")
			}
		case strings.HasPrefix(command, "switchport access vlan ") && currentPort != "":
			if !interfaceHas(configuration, currentPort, command) {
				return fmt.Errorf("Portrolle wurde nicht vollständig übernommen")
			}
		case strings.HasPrefix(command, "switchport trunk allowed vlan add ") && currentPort != "":
			if !interfaceVLANMembership(configuration, currentPort, strings.TrimPrefix(command, "switchport trunk allowed vlan add ")) {
				return fmt.Errorf("Trunk-Rolle wurde nicht vollständig übernommen")
			}
		}
	}
	return nil
}

func (a *SG350SSHConfigurator) Rollback(ctx context.Context, snapshotID string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	rollbackStore, ok := a.config.Store.(RollbackStore)
	if !ok {
		return errors.New("persistent rollback storage is unavailable")
	}
	commands, found, err := rollbackStore.RollbackCommands(ctx, snapshotID)
	if err != nil {
		return err
	}
	if !found || len(commands) == 0 {
		return errors.New("snapshot has no automatic rollback plan")
	}
	client, err := a.dial(ctx)
	if err != nil {
		return err
	}
	output, err := a.run(client, append([]string{"terminal datadump"}, commands...))
	_ = client.Close()
	if os.Getenv("ENM_DEBUG_SSH") == "1" {
		log.Printf("SG350 rollback output: %q", output)
	}
	if err != nil || cliErrorPattern.MatchString(output) {
		return fmt.Errorf("rollback rejected: %s", cleanCLIError(output, err))
	}
	client, err = a.dial(ctx)
	if err != nil {
		return err
	}
	if _, err := a.run(client, []string{"terminal datadump", "copy running-config startup-config", "y"}); err != nil {
		_ = client.Close()
		return fmt.Errorf("rollback is active but could not be saved: %w", err)
	}
	_ = client.Close()
	return nil
}

func buildRollbackCommands(configuration string, applied []string) []string {
	global := func(command string) bool {
		return regexp.MustCompile(`(?m)^\s*` + regexp.QuoteMeta(command) + `\s*$`).MatchString(configuration)
	}
	originalTrust := ""
	if match := regexp.MustCompile(`(?m)^\s*qos trust (dscp|cos|cos-dscp)\s*$`).FindStringSubmatch(configuration); len(match) == 2 {
		originalTrust = "qos trust " + match[1]
	}
	commands := []string{"configure terminal"}
	type actions struct{ membership, settings, mode []string }
	portActions := map[string]*actions{}
	createdVLANs := []string{}
	currentPort := ""
	for _, command := range applied {
		switch {
		case strings.HasPrefix(command, "vlan ") && strings.Contains(command, " name "):
			fields := strings.Fields(command)
			if len(fields) >= 2 && !configurationHasVLAN(configuration, fields[1]) {
				createdVLANs = append(createdVLANs, fields[1])
			}
		case command == "ip igmp snooping" && global("no ip igmp snooping"):
			commands = append(commands, "no ip igmp snooping")
		case strings.HasPrefix(command, "ip igmp snooping vlan ") && global("no "+command):
			commands = append(commands, "no "+command)
		case command == "qos trust dscp" && originalTrust != command:
			if global("qos advanced-mode trust dscp") {
				continue
			} else if originalTrust == "" {
				commands = append(commands, "no qos")
			} else {
				commands = append(commands, originalTrust)
			}
		case strings.HasPrefix(command, "interface gi"):
			currentPort = command
			if portActions[currentPort] == nil {
				portActions[currentPort] = &actions{}
			}
		case command == "qos trust" && currentPort != "" && !interfaceHas(configuration, currentPort, "qos trust") && !global("qos advanced ports-trusted"):
			portActions[currentPort].settings = append(portActions[currentPort].settings, "no qos trust")
		case command == "no eee enable" && currentPort != "" && !interfaceHas(configuration, currentPort, "no eee enable") && !global("no eee enable"):
			portActions[currentPort].settings = append(portActions[currentPort].settings, "eee enable")
		case command == "eee enable" && currentPort != "" && !interfaceHas(configuration, currentPort, "eee enable"):
			portActions[currentPort].settings = append(portActions[currentPort].settings, "no eee enable")
		case strings.HasPrefix(command, "description ") && currentPort != "":
			original := interfaceCommand(configuration, currentPort, "description ")
			if original != command {
				if original == "" {
					original = "no description"
				}
				portActions[currentPort].settings = append(portActions[currentPort].settings, original)
			}
		case (command == "power inline auto" || command == "power inline never") && currentPort != "":
			original := interfaceCommand(configuration, currentPort, "power inline ")
			if original != command {
				if original == "" {
					original = "power inline auto"
				}
				portActions[currentPort].settings = append(portActions[currentPort].settings, original)
			}
		case strings.HasPrefix(command, "switchport access vlan ") && currentPort != "":
			original := interfaceCommand(configuration, currentPort, "switchport access vlan ")
			if original != command {
				if original == "" {
					original = "switchport access vlan 1"
				}
				portActions[currentPort].membership = append(portActions[currentPort].membership, original)
			}
		case strings.HasPrefix(command, "switchport trunk allowed vlan add ") && currentPort != "":
			vlan := strings.TrimPrefix(command, "switchport trunk allowed vlan add ")
			if !interfaceVLANMembership(configuration, currentPort, vlan) {
				portActions[currentPort].membership = append(portActions[currentPort].membership, "switchport trunk allowed vlan remove "+vlan)
			}
		case strings.HasPrefix(command, "switchport mode ") && currentPort != "":
			original := interfaceCommand(configuration, currentPort, "switchport mode ")
			if original != command {
				if original == "" {
					original = "switchport mode access"
				}
				portActions[currentPort].mode = append(portActions[currentPort].mode, original)
			}
		}
	}
	ports := make([]string, 0, len(portActions))
	for port := range portActions {
		ports = append(ports, port)
	}
	sort.Strings(ports)
	for _, port := range ports {
		commands = append(commands, port)
		commands = append(commands, portActions[port].membership...)
		commands = append(commands, portActions[port].settings...)
		commands = append(commands, portActions[port].mode...)
		commands = append(commands, "exit")
	}
	if len(createdVLANs) > 0 {
		commands = append(commands, "vlan database")
		for _, vlan := range createdVLANs {
			commands = append(commands, "no vlan "+vlan)
		}
		commands = append(commands, "exit")
	}
	return append(commands, "end")
}

func configurationHasVLAN(configuration, wanted string) bool {
	inside := false
	for _, line := range strings.Split(strings.ReplaceAll(configuration, "\r", ""), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "vlan database" {
			inside = true
			continue
		}
		if inside && trimmed == "exit" {
			inside = false
			continue
		}
		if !inside || !strings.HasPrefix(trimmed, "vlan ") {
			continue
		}
		value := strings.TrimSpace(strings.TrimPrefix(trimmed, "vlan "))
		value = strings.Fields(value)[0]
		for _, id := range parseVLANRange(value) {
			if fmt.Sprint(id) == wanted {
				return true
			}
		}
	}
	return wanted == "1"
}

func interfaceCommand(configuration, interfaceCommand, prefix string) string {
	for _, line := range interfaceLines(configuration, interfaceCommand) {
		if strings.HasPrefix(line, prefix) {
			return line
		}
	}
	return ""
}

func interfaceLines(configuration, interfaceCommand string) []string {
	var result []string
	inside := false
	for _, line := range strings.Split(strings.ReplaceAll(configuration, "\r", ""), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "interface ") {
			inside = interfaceNamesEqual(trimmed, interfaceCommand)
			continue
		}
		if inside && trimmed == "exit" {
			break
		}
		if inside && trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func interfaceNamesEqual(left, right string) bool {
	normalize := func(value string) string {
		value = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(value, "interface ")))
		value = strings.ReplaceAll(value, "gigabitethernet", "gi")
		return value
	}
	return normalize(left) == normalize(right)
}

func interfaceVLANMembership(configuration, interfaceCommand, vlan string) bool {
	for _, line := range interfaceLines(configuration, interfaceCommand) {
		if strings.HasPrefix(line, "switchport trunk allowed vlan") {
			fields := regexp.MustCompile(`[^0-9-]+`).Split(line, -1)
			for _, field := range fields {
				if field == vlan {
					return true
				}
			}
		}
	}
	return false
}

func interfaceHas(configuration, interfaceCommand, wanted string) bool {
	lines := strings.Split(strings.ReplaceAll(configuration, "\r", ""), "\n")
	inside := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "interface ") {
			inside = interfaceNamesEqual(trimmed, interfaceCommand)
			continue
		}
		if inside && trimmed == "exit" {
			return false
		}
		if inside && trimmed == wanted {
			return true
		}
	}
	return false
}

var cliErrorPattern = regexp.MustCompile(`(?im)^\s*(% ?(?:bad|wrong|unrecognized|invalid|incomplete|ambiguous|error)|bad command|unknown command)`)
var allowedConfigCommand = regexp.MustCompile(`^(configure terminal|end|exit|vlan database|vlan [1-9][0-9]{0,3} name [A-Za-z0-9_-]{1,32}|no vlan [1-9][0-9]{0,3}|ip igmp snooping(?: vlan [1-9][0-9]{0,3})?|qos trust(?: dscp)?|(?:no )?eee enable|power inline (?:auto|never)|description "[A-Za-z0-9ÄÖÜäöüß _.,:+()/#-]{1,64}"|description [A-Za-z0-9ÄÖÜäöüß_.,:+()/#-]{1,64}|no description|interface gi(?:[1-9]|1[0-9]|2[0-8])|switchport mode (?:access|trunk)|switchport access vlan [1-9][0-9]{0,3}|switchport trunk allowed vlan (?:add|remove) [1-9][0-9]{0,3})$`)

func validateConfigCommands(commands []string) error {
	if len(commands) == 0 || len(commands) > 512 {
		return errors.New("configuration plan must contain 1-512 commands")
	}
	for _, command := range commands {
		if strings.ContainsAny(command, "\r\n;|&") || !allowedConfigCommand.MatchString(strings.TrimSpace(command)) {
			return fmt.Errorf("command is not permitted by the SG350 safety policy: %q", command)
		}
	}
	return nil
}

func (a *SG350SSHConfigurator) probe(ctx context.Context) (*observedHostKey, error) {
	var observed *observedHostKey
	config := &ssh.ClientConfig{
		User: a.config.Username, Timeout: 6 * time.Second,
		HostKeyCallback: func(_ string, _ net.Addr, key ssh.PublicKey) error {
			observed = &observedHostKey{algorithm: key.Type(), fingerprint: ssh.FingerprintSHA256(key), publicKey: ssh.MarshalAuthorizedKey(key)}
			return nil
		},
	}
	dialer := net.Dialer{Timeout: 6 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", a.sshAddress())
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	deadline, _ := ctx.Deadline()
	if deadline.IsZero() {
		deadline = time.Now().Add(8 * time.Second)
	}
	_ = conn.SetDeadline(deadline)
	_, _, _, err = ssh.NewClientConn(conn, a.sshAddress(), config)
	return observed, err
}

func (a *SG350SSHConfigurator) dial(ctx context.Context) (*ssh.Client, error) {
	algorithm, fingerprint, publicKey, found, err := a.config.Store.TrustedHostKey(ctx, a.sshAddress())
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, errors.New("SSH host key is not trusted")
	}
	callback := func(_ string, _ net.Addr, key ssh.PublicKey) error {
		if key.Type() != algorithm || ssh.FingerprintSHA256(key) != fingerprint || !bytes.Equal(ssh.MarshalAuthorizedKey(key), publicKey) {
			return errors.New("SSH host key changed; connection refused")
		}
		return nil
	}
	keyboard := ssh.KeyboardInteractive(func(_, _ string, questions []string, _ []bool) ([]string, error) {
		answers := make([]string, len(questions))
		for i := range answers {
			answers[i] = a.config.Password
		}
		return answers, nil
	})
	config := &ssh.ClientConfig{
		User: a.config.Username, HostKeyCallback: callback, Timeout: 8 * time.Second,
		Auth: []ssh.AuthMethod{ssh.Password(a.config.Password), keyboard},
	}
	type result struct {
		client *ssh.Client
		err    error
	}
	ch := make(chan result, 1)
	go func() { client, err := ssh.Dial("tcp", a.sshAddress(), config); ch <- result{client, err} }()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case result := <-ch:
		if result.err != nil {
			return nil, fmt.Errorf("SSH login: %w", result.err)
		}
		return result.client, nil
	}
}

func (a *SG350SSHConfigurator) sshAddress() string {
	address := strings.TrimPrefix(strings.TrimPrefix(a.config.Address, "https://"), "http://")
	if _, _, err := net.SplitHostPort(address); err == nil {
		return address
	}
	return net.JoinHostPort(address, "22")
}

func (a *SG350SSHConfigurator) run(client *ssh.Client, commands []string) (string, error) {
	session, err := client.NewSession()
	if err != nil {
		return "", err
	}
	defer session.Close()
	output := &synchronizedBuffer{}
	stdout, err := session.StdoutPipe()
	if err != nil {
		return "", err
	}
	stderr, err := session.StderrPipe()
	if err != nil {
		return "", err
	}
	stdin, err := session.StdinPipe()
	if err != nil {
		return "", err
	}
	if err := session.RequestPty("xterm", 80, 200, ssh.TerminalModes{ssh.ECHO: 0}); err != nil {
		return "", err
	}
	if err := session.Shell(); err != nil {
		return "", err
	}
	done := make(chan struct{})
	go func() { _, _ = io.Copy(output, io.MultiReader(stdout, stderr)); close(done) }()
	_ = waitForCLIPrompt(output, 0, 4*time.Second)
	for _, command := range commands {
		start := output.Len()
		if _, err := io.WriteString(stdin, command+"\n"); err != nil {
			return output.String(), err
		}
		if strings.HasPrefix(command, "copy running-config") {
			time.Sleep(1500 * time.Millisecond)
			continue
		}
		timeout := 12 * time.Second
		if strings.HasPrefix(command, "show running-config") || command == "y" {
			timeout = 25 * time.Second
		}
		if err := waitForCLIPrompt(output, start, timeout); err != nil {
			return output.String(), fmt.Errorf("wait for CLI after %q: %w", command, err)
		}
	}
	_, _ = io.WriteString(stdin, "exit\n")
	_ = stdin.Close()
	wait := make(chan error, 1)
	go func() { wait <- session.Wait() }()
	select {
	case err := <-wait:
		select {
		case <-done:
		case <-time.After(time.Second):
		}
		var missingStatus *ssh.ExitMissingError
		if errors.As(err, &missingStatus) {
			err = nil
		}
		return output.String(), err
	case <-time.After(15 * time.Second):
		_ = session.Close()
		return output.String(), errors.New("SSH command timed out")
	}
}

type synchronizedBuffer struct {
	mu   sync.Mutex
	data bytes.Buffer
}

func (b *synchronizedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.data.Write(p)
}
func (b *synchronizedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.data.String()
}
func (b *synchronizedBuffer) Len() int { b.mu.Lock(); defer b.mu.Unlock(); return b.data.Len() }

var cliPromptPattern = regexp.MustCompile(`(?m)(?:[>#]|\])\s*$`)

func waitForCLIPrompt(output *synchronizedBuffer, start int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		value := output.String()
		if start < len(value) && cliPromptPattern.MatchString(value[start:]) {
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	return errors.New("CLI prompt timed out")
}

func cleanCLIError(output string, err error) string {
	message := strings.TrimSpace(output)
	if len(message) > 500 {
		message = message[len(message)-500:]
	}
	if err != nil {
		message += ": " + err.Error()
	}
	return message
}

func sha256Fingerprint(raw []byte) string {
	digest := sha256.Sum256(raw)
	return "SHA256:" + base64.RawStdEncoding.EncodeToString(digest[:])
}

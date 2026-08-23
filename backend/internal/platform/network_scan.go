package platform

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"event-network-manager/backend/internal/domain"
)

const keychainService = "app.eventnetwork.manager"

func (m *MultiAdapter) ScanNetwork(ctx context.Context, request domain.NetworkScanRequest) (domain.NetworkScanReport, error) {
	m.scanMu.Lock()
	defer m.scanMu.Unlock()
	ranges := request.Ranges
	if len(ranges) == 0 {
		ranges = localEventRanges()
	}
	addresses, normalized, err := expandScanRanges(ranges)
	if err != nil {
		return domain.NetworkScanReport{}, err
	}
	if len(addresses) == 0 {
		return domain.NetworkScanReport{}, errors.New("kein lokaler IPv4-Scanbereich gefunden; bitte den Event-Netzwerkadapter verbinden")
	}

	type probeResult struct {
		address string
		found   bool
	}
	jobs := make(chan string)
	results := make(chan probeResult, len(addresses))
	workers := 48
	if len(addresses) < workers {
		workers = len(addresses)
	}
	var wg sync.WaitGroup
	for worker := 0; worker < workers; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for address := range jobs {
				results <- probeResult{address: address, found: probeHTTPS(ctx, address)}
			}
		}()
	}
	go func() {
		defer close(results)
		for _, address := range addresses {
			select {
			case jobs <- address:
			case <-ctx.Done():
				close(jobs)
				wg.Wait()
				return
			}
		}
		close(jobs)
		wg.Wait()
	}()

	discovered := []domain.DiscoveredSwitch{}
	for result := range results {
		if !result.found {
			continue
		}
		if existing, ok := m.deviceByAddress(result.address); ok {
			name := result.address
			if topology, topologyErr := existing.Telemetry.Topology(ctx); topologyErr == nil && len(topology.Switches) > 0 {
				name = topology.Switches[0].Name
			}
			discovered = append(discovered, domain.DiscoveredSwitch{Address: result.address, Name: name, Status: "connected"})
			continue
		}
		password := keychainPassword(result.address)
		if password == "" {
			discovered = append(discovered, domain.DiscoveredSwitch{Address: result.address, Status: "credentials-required", Message: "Cisco-Gerät gefunden – Anmeldung erforderlich"})
			continue
		}
		item, connectErr := m.connect(ctx, domain.SwitchCredentialRequest{Address: result.address, Username: "admin", Password: password})
		if connectErr != nil {
			discovered = append(discovered, domain.DiscoveredSwitch{Address: result.address, Status: "credentials-required", Message: "Gespeichertes Passwort wurde abgelehnt"})
			continue
		}
		discovered = append(discovered, item)
	}
	sort.Slice(discovered, func(i, j int) bool { return ipLess(discovered[i].Address, discovered[j].Address) })
	topology, topologyErr := m.Topology(ctx)
	if topologyErr != nil && len(discovered) == 0 {
		return domain.NetworkScanReport{}, topologyErr
	}
	return domain.NetworkScanReport{Ranges: normalized, Results: discovered, Switches: topology.Switches}, nil
}

func (m *MultiAdapter) ConnectDiscoveredSwitch(ctx context.Context, request domain.SwitchCredentialRequest) (domain.DiscoveredSwitch, error) {
	request.Address = strings.TrimSpace(request.Address)
	request.Username = strings.TrimSpace(request.Username)
	if request.Username == "" {
		request.Username = "admin"
	}
	if net.ParseIP(request.Address) == nil || request.Password == "" || !isLocalScanAddress(net.ParseIP(request.Address)) {
		return domain.DiscoveredSwitch{}, errors.New("Adresse, Benutzername oder Passwort ist ungültig")
	}
	item, err := m.connect(ctx, request)
	if err != nil {
		return domain.DiscoveredSwitch{}, fmt.Errorf("Anmeldung an %s fehlgeschlagen; bitte Passwort prüfen: %w", request.Address, err)
	}
	if request.Remember {
		if err := saveKeychainPassword(ctx, request.Address, request.Password); err != nil {
			item.Message = "Verbunden; Passwort konnte nicht im macOS-Schlüsselbund gespeichert werden"
		}
	}
	return item, nil
}

func (m *MultiAdapter) connect(ctx context.Context, request domain.SwitchCredentialRequest) (domain.DiscoveredSwitch, error) {
	if existing, ok := m.deviceByAddress(request.Address); ok {
		topology, err := existing.Telemetry.Topology(ctx)
		if err != nil {
			return domain.DiscoveredSwitch{}, err
		}
		name := request.Address
		if len(topology.Switches) > 0 {
			name = topology.Switches[0].Name
		}
		return domain.DiscoveredSwitch{Address: request.Address, Name: name, Status: "connected"}, nil
	}
	services, err := NewSG350WebServices(SG350WebConfig{
		Address: request.Address, Username: request.Username, Password: request.Password,
		AllowInsecureTLS: m.allowInsecureTLS, Snapshots: m.snapshots,
	})
	if err != nil {
		return domain.DiscoveredSwitch{}, err
	}
	services.Preferences = m.preferences
	m.mu.Lock()
	m.devices = append(m.devices, services)
	m.mu.Unlock()
	topology, err := services.Telemetry.Topology(ctx)
	if err != nil {
		return domain.DiscoveredSwitch{}, err
	}
	name := request.Address
	if len(topology.Switches) > 0 {
		name = topology.Switches[0].Name
	}
	return domain.DiscoveredSwitch{Address: request.Address, Name: name, Status: "connected", Message: "Switch verbunden"}, nil
}

func (m *MultiAdapter) deviceByAddress(address string) (Services, bool) {
	switchID := switchIDForAddress(address)
	for _, device := range m.devicesSnapshot() {
		status, err := device.Configurator.Status(context.Background(), switchID)
		if err == nil && status.SwitchID == switchID {
			return device, true
		}
	}
	return Services{}, false
}

func probeHTTPS(parent context.Context, address string) bool {
	ctx, cancel := context.WithTimeout(parent, 900*time.Millisecond)
	defer cancel()
	dialer := &net.Dialer{Timeout: 450 * time.Millisecond}
	transport := &http.Transport{
		DialContext:       dialer.DialContext,
		TLSClientConfig:   &tls.Config{MinVersion: tls.VersionTLS12, InsecureSkipVerify: true}, // #nosec G402 -- discovery only, no credentials
		DisableKeepAlives: true,
	}
	client := &http.Client{Transport: transport, Timeout: 850 * time.Millisecond}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://"+address+"/", nil)
	if err != nil {
		return false
	}
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	path := strings.ToLower(resp.Request.URL.Path)
	server := strings.ToLower(resp.Header.Get("Server"))
	return strings.Contains(path, "/mts/") || strings.Contains(path, "log_off") || strings.Contains(server, "cisco")
}

func localEventRanges() []string {
	interfaces, _ := net.Interfaces()
	ranges := []string{}
	preferred := []string{}
	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addresses, _ := iface.Addrs()
		for _, address := range addresses {
			ip, _, err := net.ParseCIDR(address.String())
			if err != nil || ip.To4() == nil || !isLocalScanAddress(ip) {
				continue
			}
			v4 := ip.To4()
			// Event switches are managed in the local /24 around the Mac even
			// when the adapter itself has a broader /16 mask.
			value := fmt.Sprintf("%d.%d.%d.1-254", v4[0], v4[1], v4[2])
			ranges = append(ranges, value)
			if v4[0] == 192 && v4[1] == 168 {
				preferred = append(preferred, value)
			}
		}
	}
	if len(preferred) > 0 {
		return uniqueStrings(preferred)
	}
	return uniqueStrings(ranges)
}

func expandScanRanges(ranges []string) ([]string, []string, error) {
	addresses := []string{}
	normalized := []string{}
	seen := map[string]bool{}
	for _, rawRange := range ranges {
		for _, raw := range strings.Split(rawRange, ",") {
			value := strings.TrimSpace(raw)
			if value == "" {
				continue
			}
			parts := strings.Split(value, ".")
			if len(parts) != 4 || !strings.Contains(parts[3], "-") {
				return nil, nil, fmt.Errorf("Scanbereich %q muss wie 192.168.250.1-254 angegeben werden", value)
			}
			bounds := strings.Split(parts[3], "-")
			if len(bounds) != 2 {
				return nil, nil, fmt.Errorf("ungültiger Scanbereich %q", value)
			}
			start, startErr := strconv.Atoi(bounds[0])
			end, endErr := strconv.Atoi(bounds[1])
			prefixIP := net.ParseIP(strings.Join(parts[:3], ".") + ".1")
			if startErr != nil || endErr != nil || start < 1 || end > 254 || start > end || prefixIP == nil || !isLocalScanAddress(prefixIP) {
				return nil, nil, fmt.Errorf("Scanbereich %q ist ungültig oder kein privates lokales Netz", value)
			}
			normal := fmt.Sprintf("%s.%d-%d", strings.Join(parts[:3], "."), start, end)
			normalized = append(normalized, normal)
			for last := start; last <= end; last++ {
				address := fmt.Sprintf("%s.%d", strings.Join(parts[:3], "."), last)
				if !seen[address] {
					seen[address] = true
					addresses = append(addresses, address)
				}
			}
		}
	}
	if len(addresses) > 1024 {
		return nil, nil, errors.New("maximal 1024 lokale Adressen pro Scan")
	}
	return addresses, uniqueStrings(normalized), nil
}

func isLocalScanAddress(ip net.IP) bool {
	if ip == nil {
		return false
	}
	return ip.IsPrivate() || ip.IsLinkLocalUnicast()
}

func saveKeychainPassword(ctx context.Context, address, password string) error {
	if len(password) > 255 || strings.ContainsAny(address+password, "\r\n\x00") {
		return errors.New("ungültige Schlüsselbunddaten")
	}
	command := exec.CommandContext(ctx, "/usr/bin/security", "add-generic-password", "-U", "-s", keychainService, "-a", address, "-w", password)
	if output, err := command.CombinedOutput(); err != nil {
		return fmt.Errorf("Schlüsselbund: %s", strings.TrimSpace(string(output)))
	}
	return nil
}

func uniqueStrings(values []string) []string {
	seen := map[string]bool{}
	result := []string{}
	for _, value := range values {
		if value != "" && !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	return result
}

func ipLess(left, right string) bool {
	l, r := net.ParseIP(left).To4(), net.ParseIP(right).To4()
	if l == nil || r == nil {
		return left < right
	}
	for index := 0; index < 4; index++ {
		if l[index] != r[index] {
			return l[index] < r[index]
		}
	}
	return false
}

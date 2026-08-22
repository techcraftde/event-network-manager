package platform

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"encoding/xml"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/http/cookiejar"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"event-network-manager/backend/internal/domain"
)

type SG350WebConfig struct {
	Address          string
	Username         string
	Password         string
	AllowInsecureTLS bool
	Snapshots        SnapshotStore
}

// SG350WebAdapter reads the embedded management API used by Cisco's web UI.
// It is deliberately read-only; configuration changes will use SSH later.
type SG350WebAdapter struct {
	config     SG350WebConfig
	client     *http.Client
	baseURL    string
	mtsPath    string
	mu         sync.Mutex
	loggedInAt time.Time
	counterMu  sync.Mutex
	counters   map[int]counterSample
	topologyMu sync.Mutex
	static     staticDeviceData
}

type counterSample struct {
	inOctets  uint64
	outOctets uint64
	at        time.Time
}

type staticDeviceData struct {
	fetchedAt      time.Time
	name           string
	description    string
	firmware       string
	hardware       string
	serial         string
	mac            string
	uptimeSeconds  int64
	temperatureC   float64
	poeBudgetWatts float64
	poeUsageWatts  float64
	portVLANs      map[int]vlanPortData
	portPoE        map[int]poePortData
	vlans          []domain.VLAN
	neighbors      []neighborData
}

type vlanPortData struct {
	pvid     int
	tagged   []int
	untagged []int
	mode     string
}

type poePortData struct {
	enabled bool
	watts   float64
}

type neighborData struct {
	localPort int
	name      string
	model     string
	port      string
	protocol  string
}

type readOnlyConfigurator struct{}

func (readOnlyConfigurator) Status(context.Context, string) (domain.ConfigStatus, error) {
	return domain.ConfigStatus{Available: false, Message: "SSH-Konfiguration ist nicht eingerichtet"}, nil
}
func (readOnlyConfigurator) TrustHostKey(context.Context, domain.HostKeyTrust) (domain.ConfigStatus, error) {
	return domain.ConfigStatus{}, errors.New("SSH configuration is not configured")
}
func (readOnlyConfigurator) DantePlan(_ context.Context, request domain.DantePlanRequest) (domain.ConfigPlan, error) {
	return buildDantePlan(request)
}
func (readOnlyConfigurator) VLANPlan(_ context.Context, request domain.VLANPlanRequest) (domain.ConfigPlan, error) {
	return buildVLANPlan(request)
}
func (readOnlyConfigurator) DanteHealth(context.Context, domain.DanteHealthRequest) (domain.DanteHealth, error) {
	return domain.DanteHealth{}, errors.New("SSH-Konfiguration ist nicht eingerichtet")
}
func (readOnlyConfigurator) EventBaselineStatus(context.Context, string, []domain.RoleProfile) (domain.EventBaselineStatus, error) {
	return domain.EventBaselineStatus{}, errors.New("SSH-Konfiguration ist nicht eingerichtet")
}
func (readOnlyConfigurator) CaptureSnapshot(context.Context, string) (domain.Snapshot, error) {
	return domain.Snapshot{}, errors.New("SSH configuration is not configured")
}
func (readOnlyConfigurator) SaveStartup(context.Context, string) error {
	return errors.New("Startkonfiguration kann ohne SSH nicht gespeichert werden")
}

func (readOnlyConfigurator) Apply(context.Context, domain.ConfigChange) (domain.Snapshot, error) {
	return domain.Snapshot{}, errors.New("configuration is disabled for the read-only SG350 web adapter")
}
func (readOnlyConfigurator) Rollback(context.Context, string) error {
	return errors.New("rollback is disabled for the read-only SG350 web adapter")
}

func NewSG350WebServices(config SG350WebConfig) (Services, error) {
	if config.Address == "" || config.Username == "" || config.Password == "" {
		return Services{}, errors.New("address, username and password are required")
	}
	jar, err := cookiejar.New(nil)
	if err != nil {
		return Services{}, err
	}
	host := strings.TrimSuffix(config.Address, "/")
	if !strings.HasPrefix(host, "http://") && !strings.HasPrefix(host, "https://") {
		host = "https://" + host
	}
	adapter := &SG350WebAdapter{
		config:   config,
		baseURL:  host,
		counters: make(map[int]counterSample),
		client: &http.Client{
			Timeout: 12 * time.Second,
			Jar:     jar,
			Transport: &http.Transport{TLSClientConfig: &tls.Config{
				MinVersion: tls.VersionTLS12,
				// SG350 factory certificates are normally self-signed. This is
				// explicitly controlled by ENM_SWITCH_VERIFY_TLS.
				InsecureSkipVerify: config.AllowInsecureTLS, // #nosec G402
			}},
		},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := adapter.login(ctx); err != nil {
		return Services{}, err
	}
	snapshots := config.Snapshots
	if snapshots == nil {
		snapshots = &MockAdapter{}
	}
	var configurator Configurator = readOnlyConfigurator{}
	if store, ok := snapshots.(interface {
		SnapshotStore
		HostKeyStore
	}); ok {
		address := strings.TrimPrefix(strings.TrimPrefix(host, "https://"), "http://")
		sshConfigurator, sshErr := NewSG350SSHConfigurator(SG350SSHConfig{
			SwitchID: switchIDForAddress(address), Address: address,
			Username: config.Username, Password: config.Password, Store: store,
		})
		if sshErr == nil {
			configurator = sshConfigurator
		}
	}
	return Services{
		Mode: "sg350-web", Discovery: adapter, Telemetry: adapter,
		Configurator: configurator, Snapshots: snapshots,
		Inventory: func() DeviceInventory {
			if inventory, ok := configurator.(DeviceInventory); ok {
				return inventory
			}
			return nil
		}(),
		StateReader: func() ConfigStateReader {
			if reader, ok := configurator.(ConfigStateReader); ok {
				return reader
			}
			return nil
		}(),
	}, nil
}

var mtsPathPattern = regexp.MustCompile(`(/[^/]+/mts)/config/log_off_page\.htm`)
var mtsAuthenticatedPathPattern = regexp.MustCompile(`(/[^/]+/mts)/(?:home|main)\.htm`)

func (a *SG350WebAdapter) login(ctx context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if time.Since(a.loggedInAt) < 10*time.Minute {
		return nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.baseURL+"/", nil)
	if err != nil {
		return err
	}
	resp, err := a.client.Do(req)
	if err != nil {
		return fmt.Errorf("open login page: %w", err)
	}
	finalPath := resp.Request.URL.Path
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	match := mtsPathPattern.FindStringSubmatch(finalPath)
	if len(match) != 2 {
		if authenticated := mtsAuthenticatedPathPattern.FindStringSubmatch(finalPath); len(authenticated) == 2 {
			a.mtsPath = authenticated[1]
			a.loggedInAt = time.Now()
			return nil
		}
		return fmt.Errorf("unerwarteter Cisco-Anmeldepfad %q", finalPath)
	}
	a.mtsPath = match[1]

	var encryption encryptionResponse
	if err := a.getXML(ctx, a.mtsPath+"/config/device/wcd?%7BEncryptionSetting%7D", &encryption); err != nil {
		return err
	}
	if encryption.Setting.Enabled != "1" {
		return errors.New("unencrypted Cisco login is not supported")
	}
	block, _ := pem.Decode([]byte(strings.TrimSpace(encryption.Setting.PublicKey)))
	if block == nil {
		return errors.New("invalid Cisco RSA public key")
	}
	publicKey, err := x509.ParsePKCS1PublicKey(block.Bytes)
	if err != nil {
		parsed, parseErr := x509.ParsePKIXPublicKey(block.Bytes)
		if parseErr != nil {
			return fmt.Errorf("parse Cisco public key: %w", err)
		}
		var ok bool
		publicKey, ok = parsed.(*rsa.PublicKey)
		if !ok {
			return errors.New("Cisco login key is not RSA")
		}
	}
	plain := fmt.Sprintf("user=%s&password=%s&ssd=true&token=%s&", escapeCisco(a.config.Username), escapeCisco(a.config.Password), encryption.Setting.Token)
	ciphertext, err := rsa.EncryptPKCS1v15(rand.Reader, publicKey, []byte(plain))
	if err != nil {
		return fmt.Errorf("encrypt Cisco login: %w", err)
	}
	var result actionResponse
	loginPath := a.mtsPath + "/config/system.xml?action=login&cred=" + hex.EncodeToString(ciphertext)
	if err := a.getXML(ctx, loginPath, &result); err != nil {
		return err
	}
	if result.Status.Code != "0" {
		return fmt.Errorf("Cisco login rejected: %s", result.Status.Message)
	}
	a.loggedInAt = time.Now()
	return nil
}

func escapeCisco(value string) string {
	return strings.NewReplacer("%", "%25", "#", "%23", "&", "%26", "+", "%2B").Replace(value)
}

func (a *SG350WebAdapter) getXML(ctx context.Context, path string, target any) error {
	body, err := a.getBody(ctx, path)
	if err != nil {
		return err
	}
	if err := xml.NewDecoder(strings.NewReader(body)).Decode(target); err != nil {
		return fmt.Errorf("decode %s: %w", path, err)
	}
	return nil
}

func (a *SG350WebAdapter) getBody(ctx context.Context, path string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.baseURL+path, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/xml,text/xml")
	resp, err := a.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("GET %s: %w", path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GET %s: HTTP %d", path, resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return "", fmt.Errorf("read %s: %w", path, err)
	}
	return string(body), nil
}

func (a *SG350WebAdapter) Discover(ctx context.Context) ([]domain.Switch, error) {
	topology, err := a.Topology(ctx)
	return topology.Switches, err
}

func (a *SG350WebAdapter) Topology(ctx context.Context) (domain.Topology, error) {
	a.topologyMu.Lock()
	defer a.topologyMu.Unlock()
	if err := a.login(ctx); err != nil {
		return domain.Topology{}, err
	}
	if a.static.fetchedAt.IsZero() || time.Since(a.static.fetchedAt) > 30*time.Second {
		if err := a.refreshStaticData(ctx); err != nil {
			a.invalidateLogin()
			if loginErr := a.login(ctx); loginErr != nil {
				return domain.Topology{}, loginErr
			}
			if retryErr := a.refreshStaticData(ctx); retryErr != nil && a.static.fetchedAt.IsZero() {
				return domain.Topology{}, retryErr
			}
		}
	}
	var response portsResponse
	if err := a.getXML(ctx, a.mtsPath+"/polling/wcd?%7BPorts%7D", &response); err != nil {
		return domain.Topology{}, err
	}
	if len(response.Ports.Data.Database.Ports) == 0 {
		a.invalidateLogin()
		if err := a.login(ctx); err != nil {
			return domain.Topology{}, err
		}
		if err := a.getXML(ctx, a.mtsPath+"/polling/wcd?%7BPorts%7D", &response); err != nil {
			return domain.Topology{}, err
		}
		if len(response.Ports.Data.Database.Ports) == 0 {
			return domain.Topology{}, errors.New("Cisco-Websession lieferte keine Portdaten")
		}
	}
	cpuPercent := a.readCPU(ctx)
	ports := make([]domain.Port, 0, 28)
	for _, raw := range response.Ports.Data.Database.Ports {
		if raw.Index < 1 || raw.Index > 28 {
			continue
		}
		vlanData := a.static.portVLANs[raw.Index]
		poeData := a.static.portPoE[raw.Index]
		vlans := unionVLANs(vlanData.tagged, vlanData.untagged, vlanData.pvid)
		role := "Access"
		if vlanData.mode == "Trunk" || len(vlanData.tagged) > 0 {
			role = "Trunk"
		}
		port := domain.Port{
			Index: raw.Index, Name: raw.Name, Link: raw.OperStatus == 1,
			SpeedMbps: raw.Speed, Role: role, VLANs: vlans, PVID: vlanData.pvid,
			TaggedVLANs: vlanData.tagged, UntaggedVLANs: vlanData.untagged,
			VLANMode: vlanData.mode, PoEEnabled: poeData.enabled, PoEWatts: poeData.watts,
		}
		if port.Link {
			port.RxMbps, port.TxMbps, port.Errors = a.readInterfaceCounters(ctx, port.Index)
		}
		ports = append(ports, port)
	}
	address := strings.TrimPrefix(strings.TrimPrefix(a.baseURL, "https://"), "http://")
	switchID := switchIDForAddress(address)
	name := a.static.name
	if name == "" {
		name = "SG350 @ " + address
	}
	model := modelFromDescription(a.static.description)
	sw := domain.Switch{
		ID: switchID, Name: name, Model: model, Address: address, Status: "online",
		FirmwareVersion: a.static.firmware, HardwareVersion: a.static.hardware,
		SerialNumber: a.static.serial, MACAddress: a.static.mac,
		UptimeSeconds: a.static.uptimeSeconds, CPUPercent: cpuPercent,
		TemperatureC: a.static.temperatureC, PoEBudgetWatts: a.static.poeBudgetWatts,
		PoEUsageWatts: a.static.poeUsageWatts, Ports: ports,
	}
	switches := []domain.Switch{sw}
	links := make([]domain.Link, 0, len(a.static.neighbors))
	devices := make([]domain.ConnectedDevice, 0, len(a.static.neighbors))
	for i, neighbor := range a.static.neighbors {
		neighborID := fmt.Sprintf("neighbor-%d-%s", i, safeID(neighbor.name))
		devices = append(devices, domain.ConnectedDevice{ID: neighborID, SwitchID: switchID, PortIndex: neighbor.localPort, Name: neighbor.name, Model: neighbor.model, SuggestedRole: inferDeviceRole(neighbor.name + " " + neighbor.model), Protocol: neighbor.protocol})
		links = append(links, domain.Link{
			ID:             fmt.Sprintf("%s-%s-%d", switchID, neighborID, neighbor.localPort),
			SourceSwitchID: switchID, SourcePort: neighbor.localPort,
			TargetDeviceID: neighborID, Protocol: neighbor.protocol,
		})
	}
	if persister, ok := a.config.Snapshots.(interface {
		UpsertSwitches(context.Context, []domain.Switch) error
	}); ok {
		_ = persister.UpsertSwitches(ctx, switches)
	}
	return domain.Topology{
		Switches: switches, Links: links, UpdatedAt: time.Now(),
		Source: "Cisco HTTPS · live", VLANs: a.static.vlans, Devices: devices,
	}, nil
}

func inferDeviceRole(value string) string {
	lower := strings.ToLower(value)
	for _, candidate := range []struct {
		role  string
		words []string
	}{
		{"Dante/Audio", []string{"dante", "audinate", "audio", "mixer", "console"}},
		{"Lighting", []string{"art-net", "artnet", "sacn", "lighting", "light", "dmx"}},
		{"Video", []string{"video", "ndi", "camera", "encoder", "decoder"}},
		{"Switch-Management", []string{"switch", "cisco", "netgear", "aruba"}},
		{"Control", []string{"control", "controller", "processor"}},
	} {
		for _, word := range candidate.words {
			if strings.Contains(lower, word) {
				return candidate.role
			}
		}
	}
	return ""
}

func (a *SG350WebAdapter) invalidateLogin() {
	a.mu.Lock()
	a.loggedInAt = time.Time{}
	a.mu.Unlock()
}

func switchIDForAddress(address string) string {
	return "sg350-" + strings.NewReplacer(".", "-", ":", "-").Replace(address)
}

func (a *SG350WebAdapter) refreshStaticData(ctx context.Context) error {
	query := "%7BSystemGlobalSetting%7D%7BDiagnosticsUnitList%7D%7BVLANMembershipInterfaceList%7D%7BPoEPSEInterfaceList%7D%7BPoEPSEUnitList%7D"
	var response staticResponse
	if err := a.getXML(ctx, a.mtsPath+"/polling/wcd?"+query, &response); err != nil {
		return err
	}
	if response.Device.System.Name == "" && response.Device.System.Description == "" && len(response.Device.VLANMembership.Entries) == 0 {
		return errors.New("Cisco-Websession lieferte keine Inventar- oder VLAN-Daten")
	}
	data := staticDeviceData{
		fetchedAt:     time.Now(),
		name:          response.Device.System.Name,
		description:   response.Device.System.Description,
		firmware:      response.Device.System.Firmware,
		hardware:      response.Device.System.Hardware,
		serial:        response.Device.System.Serial,
		mac:           response.Device.System.MAC,
		uptimeSeconds: response.Device.System.Uptime / 100,
		portVLANs:     make(map[int]vlanPortData),
		portPoE:       make(map[int]poePortData),
	}
	if len(response.Device.Diagnostics.Entries) > 0 {
		entry := response.Device.Diagnostics.Entries[0]
		data.temperatureC = entry.Temperature
		if data.uptimeSeconds == 0 {
			data.uptimeSeconds = entry.Uptime / 100
		}
	}
	for _, entry := range response.Device.VLANMembership.Entries {
		if entry.InterfaceID < 1 || entry.InterfaceID > 28 || !strings.HasPrefix(strings.ToLower(entry.InterfaceName), "gi") {
			continue
		}
		mode := "Access"
		if entry.PortVLANMode == 12 || entry.CurrentTagged != "" {
			mode = "Trunk"
		}
		data.portVLANs[entry.InterfaceID] = vlanPortData{
			pvid: entry.PVID, tagged: parseVLANRange(entry.CurrentTagged),
			untagged: parseVLANRange(entry.CurrentUntagged), mode: mode,
		}
	}
	for _, vlanID := range parseVLANRange(response.Device.VLANMembership.CurrentVLANs) {
		data.vlans = append(data.vlans, domain.VLAN{ID: vlanID, Name: fmt.Sprintf("VLAN %d", vlanID)})
	}
	for _, entry := range response.Device.PoEInterfaces.Entries {
		if entry.InterfaceID >= 1 && entry.InterfaceID <= 28 {
			data.portPoE[entry.InterfaceID] = poePortData{enabled: entry.AdminEnable == 1, watts: entry.OutputPower / 1000}
		}
	}
	if len(response.Device.PoEUnits.Entries) > 0 {
		data.poeBudgetWatts = response.Device.PoEUnits.Entries[0].NominalPower
		data.poeUsageWatts = response.Device.PoEUnits.Entries[0].ConsumptionPower
	}
	data.neighbors = append(data.neighbors, a.readLLDPNeighbors(ctx)...)
	data.neighbors = append(data.neighbors, a.readCDPNeighbors(ctx)...)
	a.static = data
	return nil
}

func (a *SG350WebAdapter) readCPU(ctx context.Context) float64 {
	var response cpuResponse
	if err := a.getXML(ctx, a.mtsPath+"/device/cpu.xml", &response); err != nil {
		return 0
	}
	return response.Percent
}

func (a *SG350WebAdapter) readLLDPNeighbors(ctx context.Context) []neighborData {
	var response lldpResponse
	path := a.mtsPath + "/gw/wcd?%7BLLDPMEDNeighborList%26entryCount=en%26COUNT=51%7D"
	if err := a.getXML(ctx, path, &response); err != nil {
		return nil
	}
	neighbors := make([]neighborData, 0, len(response.Device.Neighbors.Entries))
	for _, entry := range response.Device.Neighbors.Entries {
		name := strings.TrimSpace(entry.SystemName)
		if name == "" {
			name = strings.TrimSpace(entry.DeviceID)
		}
		if name == "" {
			name = "LLDP Neighbor"
		}
		neighbors = append(neighbors, neighborData{
			localPort: entry.InterfaceID, name: name,
			model: entry.SystemDescription, port: entry.AdvertisedPortID,
			protocol: "LLDP",
		})
	}
	return neighbors
}

var cdpFieldPattern = regexp.MustCompile(`(?i)NAME=([A-Za-z0-9]+)\$repeat\?([0-9]+)[^>]*value="([^"]*)"`)

func (a *SG350WebAdapter) readCDPNeighbors(ctx context.Context) []neighborData {
	path := a.mtsPath + "/cdp/cdp_neighbor_m.htm?%5BneighborCount%5DQuery:rlMibTableInstancesInfoTableName=cdpCacheTable%5BcdpCacheVT%5DpartialKey:-Counter-100"
	body, err := a.getBody(ctx, path)
	if err != nil {
		return nil
	}
	rows := map[int]map[string]string{}
	for _, match := range cdpFieldPattern.FindAllStringSubmatch(body, -1) {
		index, parseErr := strconv.Atoi(match[2])
		if parseErr != nil {
			continue
		}
		if rows[index] == nil {
			rows[index] = map[string]string{}
		}
		rows[index][match[1]] = html.UnescapeString(match[3])
	}
	indices := make([]int, 0, len(rows))
	for index := range rows {
		indices = append(indices, index)
	}
	sort.Ints(indices)
	neighbors := make([]neighborData, 0, len(indices))
	for _, index := range indices {
		row := rows[index]
		deviceID := row["cdpCacheDeviceId"]
		if deviceID == "" {
			continue
		}
		localPort, _ := strconv.Atoi(row["cdpCacheIfIndex"])
		name := row["cdpCacheSysName"]
		if name == "" {
			name = deviceID
		}
		neighbors = append(neighbors, neighborData{
			localPort: localPort, name: name, model: row["cdpCachePlatform"],
			port: row["cdpCacheDevicePort"], protocol: "CDP",
		})
	}
	return neighbors
}

func parseVLANRange(value string) []int {
	seen := map[int]bool{}
	for _, part := range strings.Split(strings.TrimSpace(value), ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		bounds := strings.SplitN(part, "-", 2)
		start, err := strconv.Atoi(bounds[0])
		if err != nil || start < 1 || start > 4094 {
			continue
		}
		end := start
		if len(bounds) == 2 {
			end, err = strconv.Atoi(bounds[1])
			if err != nil || end < start || end > 4094 {
				continue
			}
		}
		for vlanID := start; vlanID <= end; vlanID++ {
			seen[vlanID] = true
		}
	}
	result := make([]int, 0, len(seen))
	for vlanID := range seen {
		result = append(result, vlanID)
	}
	sort.Ints(result)
	return result
}

func unionVLANs(groups ...any) []int {
	seen := map[int]bool{}
	for _, group := range groups {
		switch value := group.(type) {
		case []int:
			for _, vlanID := range value {
				seen[vlanID] = true
			}
		case int:
			if value > 0 {
				seen[value] = true
			}
		}
	}
	result := make([]int, 0, len(seen))
	for vlanID := range seen {
		result = append(result, vlanID)
	}
	sort.Ints(result)
	return result
}

func modelFromDescription(description string) string {
	fields := strings.Fields(description)
	if len(fields) > 0 && strings.HasPrefix(strings.ToUpper(fields[0]), "SG350-") {
		return fields[0]
	}
	return "SG350-28P"
}

func safeID(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = regexp.MustCompile(`[^a-z0-9]+`).ReplaceAllString(value, "-")
	return strings.Trim(value, "-")
}

var (
	inOctetsPattern  = regexp.MustCompile(`writeFormattedNumber\("([0-9]+)",\s*pgTkn,\s*"TotalBytesNum"`)
	outOctetsPattern = regexp.MustCompile(`writeFormattedNumber\("([0-9]+)",\s*pgTkn,\s*"TotalBytesBottomNum"`)
	inErrorsPattern  = regexp.MustCompile(`(?i)NAME=ifInErrors\$query[^>]*value="([0-9]+)"`)
)

func (a *SG350WebAdapter) readInterfaceCounters(ctx context.Context, index int) (float64, float64, int64) {
	path := fmt.Sprintf("%s/ifstats/statist_interfacestat_interface_m.htm?%%5BstatInterface%%5DQuery:ifIndex=%d", a.mtsPath, index)
	body, err := a.getBody(ctx, path)
	if err != nil {
		return 0, 0, 0
	}
	inOctets, okIn := extractUint(inOctetsPattern, body)
	outOctets, okOut := extractUint(outOctetsPattern, body)
	errorCount, _ := extractUint(inErrorsPattern, body)
	if !okIn || !okOut {
		return 0, 0, int64(errorCount)
	}
	now := time.Now()
	a.counterMu.Lock()
	previous, exists := a.counters[index]
	a.counters[index] = counterSample{inOctets: inOctets, outOctets: outOctets, at: now}
	a.counterMu.Unlock()
	if !exists || !now.After(previous.at) || inOctets < previous.inOctets || outOctets < previous.outOctets {
		return 0, 0, int64(errorCount)
	}
	seconds := now.Sub(previous.at).Seconds()
	rxMbps := float64(inOctets-previous.inOctets) * 8 / seconds / 1_000_000
	txMbps := float64(outOctets-previous.outOctets) * 8 / seconds / 1_000_000
	return rxMbps, txMbps, int64(errorCount)
}

func extractUint(pattern *regexp.Regexp, value string) (uint64, bool) {
	match := pattern.FindStringSubmatch(value)
	if len(match) != 2 {
		return 0, false
	}
	number, err := strconv.ParseUint(match[1], 10, 64)
	return number, err == nil
}

type staticResponse struct {
	Device struct {
		System struct {
			Name        string `xml:"systemName"`
			Description string `xml:"systemDescription"`
			Uptime      int64  `xml:"systemUpTime"`
			MAC         string `xml:"MACAddress"`
			Firmware    string `xml:"firmwareVersion"`
			Hardware    string `xml:"hardwareVersion"`
			Serial      string `xml:"serialNumber"`
		} `xml:"SystemGlobalSetting"`
		Diagnostics struct {
			Entries []struct {
				Temperature float64 `xml:"tempSensorValue"`
				Uptime      int64   `xml:"upTime"`
			} `xml:"Entry"`
		} `xml:"DiagnosticsUnitList"`
		VLANMembership struct {
			CurrentVLANs string `xml:"currentVLANs"`
			Entries      []struct {
				InterfaceName   string `xml:"interfaceName"`
				InterfaceID     int    `xml:"interfaceID"`
				PortVLANMode    int    `xml:"portVLANMode"`
				PVID            int    `xml:"PVID"`
				CurrentTagged   string `xml:"currentTaggedVLANs"`
				CurrentUntagged string `xml:"currentUntaggedVLANs"`
			} `xml:"Entry"`
		} `xml:"VLANMembershipInterfaceList"`
		PoEInterfaces struct {
			Entries []struct {
				InterfaceID int     `xml:"interfaceID"`
				AdminEnable int     `xml:"adminEnable"`
				OutputPower float64 `xml:"outputPower"`
			} `xml:"Interface"`
		} `xml:"PoEPSEInterfaceList"`
		PoEUnits struct {
			Entries []struct {
				NominalPower     float64 `xml:"nominalPower"`
				ConsumptionPower float64 `xml:"consumptionPower"`
			} `xml:"UnitEntry"`
		} `xml:"PoEPSEUnitList"`
	} `xml:"DeviceConfiguration"`
}

type cpuResponse struct {
	Percent float64 `xml:"BODY>FORM>CpuDuringLastSecond"`
}

type lldpResponse struct {
	Device struct {
		Neighbors struct {
			Entries []struct {
				InterfaceID       int    `xml:"interfaceID"`
				DeviceID          string `xml:"deviceID"`
				AdvertisedPortID  string `xml:"advertisedPortID"`
				SystemName        string `xml:"systemName"`
				SystemDescription string `xml:"systemDescription"`
			} `xml:"NeighborEntry"`
		} `xml:"LLDPMEDNeighborList"`
	} `xml:"DeviceConfiguration"`
}

type encryptionResponse struct {
	Setting struct {
		Enabled   string `xml:"passwEncryptEnable"`
		PublicKey string `xml:"rsaPublicKey"`
		Token     string `xml:"loginToken"`
	} `xml:"DeviceConfiguration>EncryptionSetting"`
}

type actionResponse struct {
	Status struct {
		Code    string `xml:"statusCode"`
		Message string `xml:"statusString"`
	} `xml:"ActionStatus"`
}

type portsResponse struct {
	Ports struct {
		Data struct {
			Database struct {
				Ports []struct {
					POE         int    `xml:"POESupported"`
					Index       int    `xml:"ifIndex"`
					Name        string `xml:"portName"`
					Speed       int    `xml:"ifSpeed"`
					OperStatus  int    `xml:"operStatus"`
					AdminStatus int    `xml:"adminStatus"`
				} `xml:"inBandPortTable>port"`
			} `xml:"portsDataBase"`
		} `xml:"data"`
	} `xml:"DeviceConfiguration>Ports"`
}

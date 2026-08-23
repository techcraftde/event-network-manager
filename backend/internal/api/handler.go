package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"event-network-manager/backend/internal/domain"
	"event-network-manager/backend/internal/platform"
)

type Handler struct {
	services platform.Services
	monitor  *alarmMonitor
}

func NewHandler(services platform.Services) http.Handler {
	h := &Handler{services: services, monitor: newAlarmMonitor()}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", h.health)
	mux.HandleFunc("GET /api/topology", h.topology)
	mux.HandleFunc("GET /api/alarms", h.alarms)
	mux.HandleFunc("POST /api/discovery", h.discovery)
	mux.HandleFunc("POST /api/discovery/credentials", h.discoveryCredentials)
	mux.HandleFunc("GET /api/snapshots", h.snapshots)
	mux.HandleFunc("POST /api/snapshots", h.captureSnapshot)
	mux.HandleFunc("GET /api/config/status", h.configStatus)
	mux.HandleFunc("POST /api/config/trust-host-key", h.trustHostKey)
	mux.HandleFunc("POST /api/config/dante-plan", h.dantePlan)
	mux.HandleFunc("POST /api/config/vlan-plan", h.vlanPlan)
	mux.HandleFunc("GET /api/roles", h.roles)
	mux.HandleFunc("PUT /api/roles", h.saveRoles)
	mux.HandleFunc("GET /api/port-settings", h.portSettings)
	mux.HandleFunc("PUT /api/switches/name", h.saveSwitchName)
	mux.HandleFunc("POST /api/config/role-plan", h.rolePlan)
	mux.HandleFunc("POST /api/config/apply-role", h.applyRole)
	mux.HandleFunc("GET /api/config/event-baseline", h.eventBaselineStatus)
	mux.HandleFunc("POST /api/config/event-baseline-plan", h.eventBaselinePlan)
	mux.HandleFunc("POST /api/config/apply-event-baseline", h.applyEventBaseline)
	mux.HandleFunc("POST /api/config/apply-reference-reset", h.applyReferenceReset)
	mux.HandleFunc("POST /api/config/dante-health", h.danteHealth)
	mux.HandleFunc("POST /api/config/save-startup", h.saveStartup)
	mux.HandleFunc("POST /api/config/apply", h.apply)
	mux.HandleFunc("POST /api/config/rollback/{id}", h.rollback)
	return cors(mux)
}
func (h *Handler) applyReferenceReset(w http.ResponseWriter, r *http.Request) {
	var request struct {
		SwitchID string `json:"switchId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil || strings.TrimSpace(request.SwitchID) == "" {
		write(w, 400, map[string]string{"error": "ungültige Anfrage"})
		return
	}
	planner, ok := h.services.Configurator.(platform.ReferenceResetPlanner)
	if !ok {
		write(w, 503, map[string]string{"error": "ME-Standard-Wiederherstellung ist in diesem Betriebsmodus nicht verfügbar"})
		return
	}
	// Always rebuild from the live running configuration immediately before the
	// change. This proves that a static management address exists and prevents a
	// browser from supplying arbitrary CLI commands.
	plan, err := planner.ReferenceResetPlan(r.Context(), request.SwitchID)
	if err != nil {
		respond(w, nil, err)
		return
	}
	snapshot, err := h.services.Configurator.Apply(r.Context(), domain.ConfigChange{
		SwitchID: plan.SwitchID, Description: plan.Description, Commands: plan.Commands,
	})
	respond(w, snapshot, err)
}
func (h *Handler) eventBaselineStatus(w http.ResponseWriter, r *http.Request) {
	profiles, err := h.services.Preferences.RoleProfiles(r.Context())
	if err != nil {
		respond(w, nil, err)
		return
	}
	status, err := h.services.Configurator.EventBaselineStatus(r.Context(), r.URL.Query().Get("switchId"), profiles)
	respond(w, status, err)
}
func (h *Handler) eventBaselinePlan(w http.ResponseWriter, r *http.Request) {
	var request struct {
		SwitchID string `json:"switchId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		write(w, 400, map[string]string{"error": "ungültige Anfrage"})
		return
	}
	profiles, err := h.services.Preferences.RoleProfiles(r.Context())
	if err != nil {
		respond(w, nil, err)
		return
	}
	topology, err := h.services.Telemetry.Topology(r.Context())
	if err != nil {
		respond(w, nil, err)
		return
	}
	plan, err := platform.BuildEventBaselinePlan(request.SwitchID, profiles, topology.VLANs)
	respond(w, plan, err)
}
func (h *Handler) applyEventBaseline(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Plan domain.ConfigPlan `json:"plan"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		write(w, 400, map[string]string{"error": "ungültige Anfrage"})
		return
	}
	profiles, err := h.services.Preferences.RoleProfiles(r.Context())
	if err != nil {
		respond(w, nil, err)
		return
	}
	topology, err := h.services.Telemetry.Topology(r.Context())
	if err != nil {
		respond(w, nil, err)
		return
	}
	rebuilt, err := platform.BuildEventBaselinePlan(request.Plan.SwitchID, profiles, topology.VLANs)
	if err != nil {
		respond(w, nil, err)
		return
	}
	if strings.Join(rebuilt.Commands, "\n") != strings.Join(request.Plan.Commands, "\n") {
		write(w, 409, map[string]string{"error": "Die Switch-Daten haben sich geändert. Bitte erneut prüfen."})
		return
	}
	snapshot, err := h.services.Configurator.Apply(r.Context(), domain.ConfigChange{SwitchID: rebuilt.SwitchID, Description: rebuilt.Description, Commands: rebuilt.Commands})
	respond(w, snapshot, err)
}
func (h *Handler) alarms(w http.ResponseWriter, r *http.Request) {
	topology, err := h.services.Telemetry.Topology(r.Context())
	if err == nil {
		err = h.decorateTopology(r, &topology)
	}
	if err == nil {
		h.enrichDevices(r, &topology)
	}
	if err != nil {
		respond(w, nil, err)
		return
	}
	respond(w, h.monitor.evaluate(topology), nil)
}
func (h *Handler) saveSwitchName(w http.ResponseWriter, r *http.Request) {
	if h.services.Preferences == nil {
		write(w, 503, map[string]string{"error": "Switch-Einstellungen sind nicht verfügbar"})
		return
	}
	var request domain.SwitchNameRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		write(w, 400, map[string]string{"error": "ungültige Anfrage"})
		return
	}
	plan, actualName, err := platform.BuildSwitchNamePlan(request.SwitchID, request.Name)
	if err != nil {
		write(w, 400, map[string]string{"error": err.Error()})
		return
	}
	request.Name = actualName
	if h.services.StateReader != nil {
		_, err = h.services.Configurator.Apply(r.Context(), domain.ConfigChange{SwitchID: plan.SwitchID, Description: plan.Description, Commands: plan.Commands})
	} else {
		err = h.services.Preferences.SaveSwitchDisplayName(r.Context(), request.SwitchID, request.Name)
	}
	respond(w, request, err)
}
func (h *Handler) roles(w http.ResponseWriter, r *http.Request) {
	if h.services.Preferences == nil {
		write(w, 503, map[string]string{"error": "Rollen-Einstellungen sind nicht verfügbar"})
		return
	}
	v, err := h.services.Preferences.RoleProfiles(r.Context())
	respond(w, v, err)
}
func (h *Handler) saveRoles(w http.ResponseWriter, r *http.Request) {
	if h.services.Preferences == nil {
		write(w, 503, map[string]string{"error": "Rollen-Einstellungen sind nicht verfügbar"})
		return
	}
	var profiles []domain.RoleProfile
	if err := json.NewDecoder(r.Body).Decode(&profiles); err != nil {
		write(w, 400, map[string]string{"error": "ungültige Rollen-Einstellungen"})
		return
	}
	if err := platform.ValidateRoleProfiles(profiles); err != nil {
		write(w, 400, map[string]string{"error": err.Error()})
		return
	}
	err := h.services.Preferences.SaveRoleProfiles(r.Context(), profiles)
	respond(w, profiles, err)
}
func (h *Handler) portSettings(w http.ResponseWriter, r *http.Request) {
	if h.services.Preferences == nil {
		write(w, 503, map[string]string{"error": "Port-Einstellungen sind nicht verfügbar"})
		return
	}
	v, err := h.services.Preferences.PortSettings(r.Context(), r.URL.Query().Get("switchId"))
	respond(w, v, err)
}
func (h *Handler) rolePlan(w http.ResponseWriter, r *http.Request) {
	if h.services.Preferences == nil {
		write(w, 503, map[string]string{"error": "Rollen-Einstellungen sind nicht verfügbar"})
		return
	}
	var request domain.RolePlanRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		write(w, 400, map[string]string{"error": "ungültige Anfrage"})
		return
	}
	profiles, err := h.services.Preferences.RoleProfiles(r.Context())
	if err != nil {
		respond(w, nil, err)
		return
	}
	topology, err := h.services.Telemetry.Topology(r.Context())
	if err != nil {
		respond(w, nil, err)
		return
	}
	plan, err := platform.BuildRolePlan(request, profiles, topology.VLANs)
	respond(w, plan, err)
}
func (h *Handler) applyRole(w http.ResponseWriter, r *http.Request) {
	if h.services.Preferences == nil {
		write(w, 503, map[string]string{"error": "Port-Einstellungen sind nicht verfügbar"})
		return
	}
	var request domain.RoleApplyRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		write(w, 400, map[string]string{"error": "ungültige Anfrage"})
		return
	}
	profiles, err := h.services.Preferences.RoleProfiles(r.Context())
	if err != nil {
		respond(w, nil, err)
		return
	}
	topology, err := h.services.Telemetry.Topology(r.Context())
	if err != nil {
		respond(w, nil, err)
		return
	}
	rebuilt, err := platform.BuildRolePlan(domain.RolePlanRequest{SwitchID: request.Plan.SwitchID, Ports: request.Settings}, profiles, topology.VLANs)
	if err != nil {
		respond(w, nil, err)
		return
	}
	if strings.Join(rebuilt.Commands, "\n") != strings.Join(request.Plan.Commands, "\n") {
		write(w, 409, map[string]string{"error": "Die Switch-Daten haben sich geändert. Bitte die Änderung erneut prüfen."})
		return
	}
	snapshot, err := h.services.Configurator.Apply(r.Context(), domain.ConfigChange{SwitchID: rebuilt.SwitchID, Description: rebuilt.Description, Commands: rebuilt.Commands})
	if err != nil {
		respond(w, nil, err)
		return
	}
	if h.services.StateReader != nil {
		respond(w, snapshot, nil)
		return
	}
	current, err := h.services.Preferences.PortSettings(r.Context(), rebuilt.SwitchID)
	if err != nil {
		respond(w, nil, err)
		return
	}
	currentByPort := map[int]domain.PortSetting{}
	for _, setting := range current {
		currentByPort[setting.PortIndex] = setting
	}
	before := make([]domain.PortSetting, 0, len(request.Settings))
	settings := make([]domain.PortSetting, 0, len(request.Settings))
	for _, setting := range request.Settings {
		if previous, found := currentByPort[setting.PortIndex]; found {
			before = append(before, previous)
		} else {
			before = append(before, domain.PortSetting{SwitchID: rebuilt.SwitchID, PortIndex: setting.PortIndex})
		}
		settings = append(settings, domain.PortSetting{SwitchID: request.Plan.SwitchID, PortIndex: setting.PortIndex, DisplayName: strings.TrimSpace(setting.DisplayName), RoleID: setting.RoleID})
	}
	if err := h.services.Preferences.SaveRoleSettingRollback(r.Context(), snapshot.ID, before); err != nil {
		respond(w, nil, err)
		return
	}
	if err := h.services.Preferences.SavePortSettings(r.Context(), settings); err != nil {
		respond(w, nil, err)
		return
	}
	respond(w, snapshot, nil)
}
func (h *Handler) vlanPlan(w http.ResponseWriter, r *http.Request) {
	var request domain.VLANPlanRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		write(w, 400, map[string]string{"error": "ungültige Anfrage"})
		return
	}
	v, err := h.services.Configurator.VLANPlan(r.Context(), request)
	respond(w, v, err)
}
func (h *Handler) danteHealth(w http.ResponseWriter, r *http.Request) {
	var request domain.DanteHealthRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		write(w, 400, map[string]string{"error": "ungültige Anfrage"})
		return
	}
	v, err := h.services.Configurator.DanteHealth(r.Context(), request)
	respond(w, v, err)
}
func (h *Handler) configStatus(w http.ResponseWriter, r *http.Request) {
	v, err := h.services.Configurator.Status(r.Context(), r.URL.Query().Get("switchId"))
	respond(w, v, err)
}
func (h *Handler) trustHostKey(w http.ResponseWriter, r *http.Request) {
	var trust domain.HostKeyTrust
	if err := json.NewDecoder(r.Body).Decode(&trust); err != nil {
		write(w, 400, map[string]string{"error": "invalid request"})
		return
	}
	v, err := h.services.Configurator.TrustHostKey(r.Context(), trust)
	respond(w, v, err)
}
func (h *Handler) dantePlan(w http.ResponseWriter, r *http.Request) {
	var request domain.DantePlanRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		write(w, 400, map[string]string{"error": "invalid request"})
		return
	}
	v, err := h.services.Configurator.DantePlan(r.Context(), request)
	respond(w, v, err)
}

func (h *Handler) health(w http.ResponseWriter, _ *http.Request) {
	write(w, http.StatusOK, map[string]any{"status": "ok", "mode": h.services.Mode})
}
func (h *Handler) topology(w http.ResponseWriter, r *http.Request) {
	v, err := h.services.Telemetry.Topology(r.Context())
	if err == nil {
		err = h.decorateTopology(r, &v)
	}
	if err == nil {
		h.enrichDevices(r, &v)
	}
	respond(w, v, err)
}
func (h *Handler) enrichDevices(r *http.Request, topology *domain.Topology) {
	if h.services.Inventory == nil {
		return
	}
	for _, sw := range topology.Switches {
		items, err := h.services.Inventory.ConnectedDevices(r.Context(), sw.ID)
		if err != nil {
			continue
		}
		for _, item := range items {
			var port *domain.Port
			for pi := range sw.Ports {
				if sw.Ports[pi].Index == item.PortIndex {
					port = &sw.Ports[pi]
					break
				}
			}
			if port == nil || strings.EqualFold(port.VLANMode, "trunk") {
				continue
			}
			merged := false
			for i := range topology.Devices {
				if topology.Devices[i].SwitchID == item.SwitchID && topology.Devices[i].PortIndex == item.PortIndex {
					if topology.Devices[i].IPAddress == "" {
						topology.Devices[i].IPAddress = item.IPAddress
					}
					if topology.Devices[i].MACAddress == "" {
						topology.Devices[i].MACAddress = item.MACAddress
					}
					if topology.Devices[i].SuggestedRole == "" && port.RoleID != "" {
						topology.Devices[i].SuggestedRole = port.Role
					}
					merged = true
					break
				}
			}
			if merged {
				continue
			}
			if item.SuggestedRole == "" && port.RoleID != "" {
				item.SuggestedRole = port.Role
			}
			topology.Devices = append(topology.Devices, item)
			topology.Links = append(topology.Links, domain.Link{ID: sw.ID + "-" + item.ID, SourceSwitchID: sw.ID, SourcePort: item.PortIndex, TargetDeviceID: item.ID, Protocol: item.Protocol})
		}
	}
}
func (h *Handler) decorateTopology(r *http.Request, topology *domain.Topology) error {
	profiles := domain.DefaultRoleProfiles()
	if h.services.Preferences != nil {
		stored, err := h.services.Preferences.RoleProfiles(r.Context())
		if err != nil {
			return err
		}
		profiles = stored
	}
	profileByID := map[string]domain.RoleProfile{}
	for _, profile := range profiles {
		profileByID[profile.ID] = profile
	}
	for si := range topology.Switches {
		settings := map[int]domain.PortSetting{}
		liveState := false
		if h.services.StateReader != nil {
			state, err := h.services.StateReader.ConfigurationState(r.Context(), topology.Switches[si].ID, profiles)
			if err == nil {
				liveState = true
				if state.Name != "" {
					topology.Switches[si].Name = state.Name
				}
				for _, setting := range state.PortSettings {
					settings[setting.PortIndex] = setting
				}
			}
		}
		if !liveState && h.services.Preferences != nil {
			if name, found, err := h.services.Preferences.SwitchDisplayName(r.Context(), topology.Switches[si].ID); err != nil {
				return err
			} else if found {
				topology.Switches[si].Name = name
			}
		}
		if !liveState && h.services.Preferences != nil {
			stored, err := h.services.Preferences.PortSettings(r.Context(), topology.Switches[si].ID)
			if err != nil {
				return err
			}
			for _, setting := range stored {
				settings[setting.PortIndex] = setting
			}
		}
		for pi := range topology.Switches[si].Ports {
			port := &topology.Switches[si].Ports[pi]
			port.DisplayName = "Port " + fmt.Sprint(port.Index)
			if setting, ok := settings[port.Index]; ok {
				if setting.DisplayName != "" {
					port.DisplayName = setting.DisplayName
				}
				port.RoleID = setting.RoleID
				if profile, found := profileByID[setting.RoleID]; found {
					port.Role = profile.Name
				}
			}
			if port.Role == "" {
				port.Role = "Nicht zugewiesen"
			}
		}
	}
	return nil
}
func (h *Handler) discovery(w http.ResponseWriter, r *http.Request) {
	if scanner, ok := h.services.Discovery.(platform.NetworkScanner); ok {
		var request domain.NetworkScanRequest
		if r.Body != nil {
			decoderErr := json.NewDecoder(r.Body).Decode(&request)
			if decoderErr != nil && decoderErr != io.EOF {
				write(w, 400, map[string]string{"error": "ungültiger Scanbereich"})
				return
			}
		}
		v, err := scanner.ScanNetwork(r.Context(), request)
		respond(w, v, err)
		return
	}
	v, err := h.services.Discovery.Discover(r.Context())
	respond(w, v, err)
}
func (h *Handler) discoveryCredentials(w http.ResponseWriter, r *http.Request) {
	scanner, ok := h.services.Discovery.(platform.NetworkScanner)
	if !ok {
		write(w, 503, map[string]string{"error": "Anmeldung gefundener Switches ist in diesem Modus nicht verfügbar"})
		return
	}
	var request domain.SwitchCredentialRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		write(w, 400, map[string]string{"error": "ungültige Zugangsdaten"})
		return
	}
	v, err := scanner.ConnectDiscoveredSwitch(r.Context(), request)
	respond(w, v, err)
}
func (h *Handler) snapshots(w http.ResponseWriter, r *http.Request) {
	v, err := h.services.Snapshots.List(r.Context(), r.URL.Query().Get("switchId"))
	respond(w, v, err)
}
func (h *Handler) captureSnapshot(w http.ResponseWriter, r *http.Request) {
	var request struct {
		SwitchID string `json:"switchId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		write(w, 400, map[string]string{"error": "invalid request"})
		return
	}
	v, err := h.services.Configurator.CaptureSnapshot(r.Context(), request.SwitchID)
	respond(w, v, err)
}
func (h *Handler) saveStartup(w http.ResponseWriter, r *http.Request) {
	var request struct {
		SwitchID string `json:"switchId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		write(w, 400, map[string]string{"error": "ungültige Anfrage"})
		return
	}
	err := h.services.Configurator.SaveStartup(r.Context(), request.SwitchID)
	respond(w, map[string]bool{"ok": err == nil}, err)
}
func (h *Handler) apply(w http.ResponseWriter, r *http.Request) {
	var c domain.ConfigChange
	if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
		write(w, 400, map[string]string{"error": "invalid request"})
		return
	}
	v, err := h.services.Configurator.Apply(r.Context(), c)
	respond(w, v, err)
}
func (h *Handler) rollback(w http.ResponseWriter, r *http.Request) {
	err := h.services.Configurator.Rollback(r.Context(), r.PathValue("id"))
	if err == nil && h.services.Preferences != nil {
		err = h.services.Preferences.RestoreRoleSettings(r.Context(), r.PathValue("id"))
	}
	respond(w, map[string]bool{"ok": err == nil}, err)
}
func respond(w http.ResponseWriter, v any, err error) {
	if err != nil {
		write(w, 500, map[string]string{"error": err.Error()})
		return
	}
	write(w, 200, v)
}
func write(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		allowedOrigins := map[string]bool{
			"http://localhost:5173":   true,
			"http://127.0.0.1:5173":   true,
			"tauri://localhost":       true,
			"https://tauri.localhost": true,
		}
		if allowedOrigins[origin] {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
		} else if origin != "" {
			write(w, http.StatusForbidden, map[string]string{"error": "origin is not allowed"})
			return
		}
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.Header().Set("Access-Control-Allow-Methods", "GET,POST,PUT,OPTIONS")
		if strings.EqualFold(r.Method, "OPTIONS") {
			w.WriteHeader(204)
			return
		}
		next.ServeHTTP(w, r)
	})
}

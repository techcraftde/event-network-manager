package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"event-network-manager/backend/internal/domain"
	"event-network-manager/backend/internal/platform"
)

type Handler struct{ services platform.Services }

func NewHandler(services platform.Services) http.Handler {
	h := &Handler{services: services}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", h.health)
	mux.HandleFunc("GET /api/topology", h.topology)
	mux.HandleFunc("POST /api/discovery", h.discovery)
	mux.HandleFunc("GET /api/snapshots", h.snapshots)
	mux.HandleFunc("POST /api/snapshots", h.captureSnapshot)
	mux.HandleFunc("GET /api/config/status", h.configStatus)
	mux.HandleFunc("POST /api/config/trust-host-key", h.trustHostKey)
	mux.HandleFunc("POST /api/config/dante-plan", h.dantePlan)
	mux.HandleFunc("POST /api/config/vlan-plan", h.vlanPlan)
	mux.HandleFunc("POST /api/config/dante-health", h.danteHealth)
	mux.HandleFunc("POST /api/config/apply", h.apply)
	mux.HandleFunc("POST /api/config/rollback/{id}", h.rollback)
	return cors(mux)
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
	respond(w, v, err)
}
func (h *Handler) discovery(w http.ResponseWriter, r *http.Request) {
	v, err := h.services.Discovery.Discover(r.Context())
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
		w.Header().Set("Access-Control-Allow-Methods", "GET,POST,OPTIONS")
		if strings.EqualFold(r.Method, "OPTIONS") {
			w.WriteHeader(204)
			return
		}
		next.ServeHTTP(w, r)
	})
}

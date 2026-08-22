package api

import (
	"event-network-manager/backend/internal/platform"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTopology(t *testing.T) {
	r := httptest.NewRequest("GET", "/api/topology", nil)
	w := httptest.NewRecorder()
	NewHandler(platform.NewMockServices()).ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatalf("status = %d", w.Code)
	}
	if w.Body.Len() == 0 {
		t.Fatal("empty response")
	}
}

func TestVLANPlanEndpoint(t *testing.T) {
	r := httptest.NewRequest("POST", "/api/config/vlan-plan", strings.NewReader(`{"switchId":"demo","vlanId":20,"ports":[1,2],"mode":"access"}`))
	w := httptest.NewRecorder()
	NewHandler(platform.NewMockServices()).ServeHTTP(w, r)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "switchport access vlan 20") {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestDanteHealthEndpoint(t *testing.T) {
	r := httptest.NewRequest("POST", "/api/config/dante-health", strings.NewReader(`{"switchId":"demo","vlanId":10,"ports":[1,2]}`))
	w := httptest.NewRecorder()
	NewHandler(platform.NewMockServices()).ServeHTTP(w, r)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"healthy":true`) {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestCORSAllowsLoopbackFrontend(t *testing.T) {
	r := httptest.NewRequest("GET", "/api/health", nil)
	r.Header.Set("Origin", "http://127.0.0.1:5173")
	w := httptest.NewRecorder()
	NewHandler(platform.NewMockServices()).ServeHTTP(w, r)
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "http://127.0.0.1:5173" {
		t.Fatalf("allow origin = %q", got)
	}
}

func TestCORSRejectsUntrustedOrigin(t *testing.T) {
	r := httptest.NewRequest("POST", "/api/config/apply", nil)
	r.Header.Set("Origin", "https://attacker.example")
	w := httptest.NewRecorder()
	NewHandler(platform.NewMockServices()).ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatalf("status = %d", w.Code)
	}
}

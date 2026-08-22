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

func TestRolePlanEndpoint(t *testing.T) {
	r := httptest.NewRequest("POST", "/api/config/role-plan", strings.NewReader(`{"switchId":"foh","ports":[{"portIndex":2,"displayName":"Lichtpult","roleId":"lighting"}]}`))
	w := httptest.NewRecorder()
	NewHandler(platform.NewMockServices()).ServeHTTP(w, r)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `description \"Lichtpult\"`) || !strings.Contains(w.Body.String(), "Lighting") {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestSwitchRenameDecoratesTopology(t *testing.T) {
	h := NewHandler(platform.NewMockServices())
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("PUT", "/api/switches/name", strings.NewReader(`{"switchId":"foh","name":"FOH Rack"}`)))
	if w.Code != 200 {
		t.Fatalf("rename status=%d body=%s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/topology", nil))
	if !strings.Contains(w.Body.String(), `"name":"FOH Rack"`) || !strings.Contains(w.Body.String(), `"displayName":"Port 1"`) {
		t.Fatalf("body=%s", w.Body.String())
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

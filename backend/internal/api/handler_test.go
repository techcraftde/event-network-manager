package api

import (
	"event-network-manager/backend/internal/domain"
	"event-network-manager/backend/internal/platform"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestInferPortRoleFromLiveVLANMembership(t *testing.T) {
	profiles := domain.DefaultRoleProfiles()
	tests := []struct {
		name string
		port domain.Port
		want string
	}{
		{name: "untagged membership overrides stale pvid", port: domain.Port{VLANMode: "Access", PVID: 1, VLANs: []int{1, 2}, UntaggedVLANs: []int{2}}, want: "control"},
		{name: "pvid fallback", port: domain.Port{VLANMode: "Access", PVID: 3}, want: "lighting"},
		{name: "single membership fallback", port: domain.Port{VLANMode: "Access", VLANs: []int{5}}, want: "video"},
		{name: "management trunk", port: domain.Port{VLANMode: "Trunk", PVID: 4000, VLANs: []int{1, 2, 3, 4, 4000}, TaggedVLANs: []int{1, 2, 3, 4}, UntaggedVLANs: []int{4000}}, want: "trunk"},
		{name: "foreign trunk remains unassigned", port: domain.Port{VLANMode: "Trunk", PVID: 1, VLANs: []int{1, 2}, TaggedVLANs: []int{2}}, want: ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			profile, found := inferPortRoleFromVLAN(test.port, profiles)
			if test.want == "" {
				if found {
					t.Fatalf("unexpected role %q", profile.ID)
				}
				return
			}
			if !found || profile.ID != test.want {
				t.Fatalf("role = %q, found = %v; want %q", profile.ID, found, test.want)
			}
		})
	}
}

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

func TestSaveStartupEndpoint(t *testing.T) {
	r := httptest.NewRequest("POST", "/api/config/save-startup", strings.NewReader(`{"switchId":"foh"}`))
	w := httptest.NewRecorder()
	NewHandler(platform.NewMockServices()).ServeHTTP(w, r)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"ok":true`) {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestIdentifyAndEventModeEndpoints(t *testing.T) {
	h := NewHandler(platform.NewMockServices())
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("POST", "/api/switches/identify", strings.NewReader(`{"switchId":"foh","durationSeconds":30}`)))
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"ok":true`) {
		t.Fatalf("identify status=%d body=%s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("PUT", "/api/event-mode", strings.NewReader(`{"enabled":true}`)))
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"enabled":true`) {
		t.Fatalf("event mode status=%d body=%s", w.Code, w.Body.String())
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

func TestEventBaselineEndpoints(t *testing.T) {
	h := NewHandler(platform.NewMockServices())
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/config/event-baseline?switchId=foh", nil))
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"healthy":true`) {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("POST", "/api/config/event-baseline-plan", strings.NewReader(`{"switchId":"foh"}`)))
	if w.Code != 200 || !strings.Contains(w.Body.String(), "bridge multicast filtering") {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestReferenceResetEndpointBuildsPlanServerSide(t *testing.T) {
	r := httptest.NewRequest("POST", "/api/config/apply-reference-reset", strings.NewReader(`{"switchId":"foh","commands":["hostname attacker"]}`))
	w := httptest.NewRecorder()
	NewHandler(platform.NewMockServices()).ServeHTTP(w, r)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"switchId":"foh"`) || !strings.Contains(w.Body.String(), `"sizeBytes":`) {
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
	if !strings.Contains(w.Body.String(), `"name":"FOH-Rack"`) || !strings.Contains(w.Body.String(), `"displayName":"Port 1"`) {
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

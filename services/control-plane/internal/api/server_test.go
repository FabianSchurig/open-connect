package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/FabianSchurig/open-connect/services/control-plane/internal/claims"
	"github.com/FabianSchurig/open-connect/services/control-plane/internal/clock"
	"github.com/FabianSchurig/open-connect/services/control-plane/internal/devices"
	natsx "github.com/FabianSchurig/open-connect/services/control-plane/internal/nats"
	"github.com/FabianSchurig/open-connect/services/control-plane/internal/rbac"
)

func newTestServer(t *testing.T) *Server {
	t.Helper()
	clk := &clock.Fixed{T: time.Date(2026, 4, 23, 0, 0, 0, 0, time.UTC)}
	res := rbac.StaticResolver{
		"alice": {rbac.RolePipelineCreate, rbac.RolePipelineRead, rbac.RoleDeviceRegister, rbac.RoleDeviceRead, rbac.RoleDeviceTag, rbac.RoleDeviceRetire},
		"eve":   {}, // unprivileged
	}
	s := &Server{
		Devices:  devices.NewMemStore(),
		Claims:   claims.New(clk, natsx.NewMemBus(), NewClaimID, NewLeaseID),
		Resolver: res,
	}
	return s
}

func do(t *testing.T, h http.Handler, method, path, subject string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatal(err)
		}
	}
	req := httptest.NewRequest(method, path, &buf)
	if subject != "" {
		req.Header.Set("X-Subject", subject)
	}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

func TestDevicesAndClaimsHappyPath(t *testing.T) {
	s := newTestServer(t)
	h := s.Router()

	// Register a device.
	rr := do(t, h, "POST", "/v1/devices", "alice", map[string]any{
		"serial": "DEV-1", "public_key": "PEM",
		"initial_tags": []string{"yocto-wic-ab", "x86"},
	})
	if rr.Code != http.StatusCreated {
		t.Fatalf("create device: %d %s", rr.Code, rr.Body.String())
	}

	// List by tag.
	rr = do(t, h, "GET", "/v1/devices?tag=yocto-wic-ab&tag=x86", "alice", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("list: %d %s", rr.Code, rr.Body.String())
	}

	// Create a claim.
	rr = do(t, h, "POST", "/v1/claims", "alice", map[string]any{
		"count": 1, "tags": []string{"yocto-wic-ab"},
		"desired_version": "2.4.0", "ttl_seconds": 60,
	})
	if rr.Code != http.StatusCreated {
		t.Fatalf("create claim: %d %s", rr.Code, rr.Body.String())
	}
	var c claimDTO
	if err := json.Unmarshal(rr.Body.Bytes(), &c); err != nil {
		t.Fatal(err)
	}
	if c.State != "Open" {
		t.Fatalf("state=%s want Open", c.State)
	}

	// Release the claim.
	rr = do(t, h, "DELETE", "/v1/claims/"+c.ID, "alice", nil)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("delete: %d %s", rr.Code, rr.Body.String())
	}
}

// FR-16 — claim creation requires pipeline:create-claim role.
func TestCreateClaim_RBACForbidden(t *testing.T) {
	s := newTestServer(t)
	h := s.Router()
	rr := do(t, h, "POST", "/v1/claims", "eve", map[string]any{
		"count": 1, "tags": []string{"x"}, "ttl_seconds": 1,
	})
	if rr.Code != http.StatusForbidden {
		t.Fatalf("want 403, got %d %s", rr.Code, rr.Body.String())
	}
}

// Missing subject -> 401.
func TestCreateClaim_Unauthorized(t *testing.T) {
	s := newTestServer(t)
	h := s.Router()
	rr := do(t, h, "POST", "/v1/claims", "", map[string]any{"count": 1, "tags": []string{"x"}, "ttl_seconds": 1})
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", rr.Code)
	}
}

// FR-17 — claim status response includes per-device sub-state map.
func TestGetClaim_IncludesPerDeviceState(t *testing.T) {
	s := newTestServer(t)
	h := s.Router()

	rr := do(t, h, "POST", "/v1/claims", "alice", map[string]any{
		"count": 1, "tags": []string{"x"}, "ttl_seconds": 60,
	})
	var c claimDTO
	_ = json.Unmarshal(rr.Body.Bytes(), &c)

	// Lock from a fake device path (server-internal API).
	res, err := s.Claims.TryLock(c.ID, "DEV-1", []string{"x"})
	if err != nil || !res.Granted {
		t.Fatalf("lock failed: %v / %+v", err, res)
	}

	rr = do(t, h, "GET", "/v1/claims/"+c.ID, "alice", nil)
	var got claimDTO
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Devices) != 1 || got.Devices[0].State == "" {
		t.Fatalf("FR-17 violated: devices=%+v", got.Devices)
	}
	if got.Devices[0].LastUpdate.IsZero() {
		t.Fatal("FR-17 last_update_timestamp must be populated")
	}
}

func TestHealthz(t *testing.T) {
	s := newTestServer(t)
	rr := do(t, s.Router(), "GET", "/healthz", "", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("healthz=%d", rr.Code)
	}
}

package rbac

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRequire_GrantsWhenRolePresent(t *testing.T) {
	r := StaticResolver{"alice": {RolePipelineCreate}}
	h := Require(r, RolePipelineCreate)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodPost, "/v1/claims", nil)
	req.Header.Set("X-Subject", "alice")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("want 204, got %d", rr.Code)
	}
}

func TestRequire_ForbidsWhenRoleMissing(t *testing.T) {
	r := StaticResolver{"alice": {RoleDeviceRead}}
	h := Require(r, RolePipelineCreate)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("handler should not be reached")
	}))

	req := httptest.NewRequest(http.MethodPost, "/v1/claims", nil)
	req.Header.Set("X-Subject", "alice")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("want 403, got %d", rr.Code)
	}
}

func TestRequire_RejectsMissingSubject(t *testing.T) {
	h := Require(StaticResolver{}, RolePipelineCreate)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("handler should not be reached")
	}))

	req := httptest.NewRequest(http.MethodPost, "/v1/claims", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", rr.Code)
	}
}

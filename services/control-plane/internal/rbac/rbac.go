// Package rbac provides the MVP RBAC stub described in Epic D / FR-16.
//
// Roles are loaded from a static dev file (or environment variable in tests);
// the real OIDC/JWT integration is post-MVP. The Middleware extracts a subject
// from the X-Subject header (test mode) or a JWT (future) and resolves roles.
package rbac

import (
	"context"
	"net/http"

	"github.com/FabianSchurig/open-connect/services/control-plane/internal/httperr"
)

// Role identifiers used by the MVP. Must match arc42 §5.6 RBAC column.
const (
	RoleDeviceRegister  = "device:register"
	RoleDeviceRead      = "device:read"
	RoleDeviceTag       = "device:tag"
	RoleDeviceRetire    = "device:retire"
	RolePipelineCreate  = "pipeline:create-claim"
	RolePipelineRead    = "pipeline:read-claim"
	RoleReleasePublish  = "release:publish"
	RoleReleaseRead     = "release:read"
	RoleAuditRead       = "audit:read"
	RoleAuditExport     = "audit:export"
)

type contextKey int

const subjectKey contextKey = 1

// Resolver maps a subject (e.g. JWT sub) to a set of roles.
type Resolver interface {
	Roles(subject string) []string
}

// StaticResolver is the dev/test implementation: a fixed map.
type StaticResolver map[string][]string

func (s StaticResolver) Roles(subject string) []string { return s[subject] }

// SubjectFromRequest extracts the authenticated subject. In MVP test mode we
// trust the X-Subject header (the mTLS+JWT middleware will replace this).
func SubjectFromRequest(r *http.Request) string {
	if v := r.Context().Value(subjectKey); v != nil {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return r.Header.Get("X-Subject")
}

// WithSubject injects an authenticated subject into the request context.
// Used by the auth middleware (or by tests).
func WithSubject(r *http.Request, subject string) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), subjectKey, subject))
}

// Require returns middleware enforcing the named role.
func Require(res Resolver, role string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			subject := SubjectFromRequest(r)
			if subject == "" {
				httperr.Write(w, http.StatusUnauthorized, "unauthorized", "missing subject")
				return
			}
			for _, have := range res.Roles(subject) {
				if have == role {
					next.ServeHTTP(w, r)
					return
				}
			}
			httperr.Write(w, http.StatusForbidden, "forbidden", "missing role: "+role)
		})
	}
}

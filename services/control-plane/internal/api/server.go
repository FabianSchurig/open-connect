// Package api wires the chi router for the control-plane MVP (arc42 §5.6).
//
// In MVP authentication is the X-Subject header (RBAC stub); the production
// mTLS+JWT middleware is a drop-in replacement at the router level.
package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/FabianSchurig/open-connect/services/control-plane/internal/claims"
	"github.com/FabianSchurig/open-connect/services/control-plane/internal/devices"
	"github.com/FabianSchurig/open-connect/services/control-plane/internal/httperr"
	"github.com/FabianSchurig/open-connect/services/control-plane/internal/rbac"
)

type Server struct {
	Devices  devices.Store
	Claims   *claims.Service
	Resolver rbac.Resolver
}

// Router builds the API. The caller is responsible for binding it to a
// listener (TLS or plain in tests).
func (s *Server) Router() http.Handler {
	r := chi.NewRouter()

	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	})
	r.Get("/readyz", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	})
	r.Get("/metrics", func(w http.ResponseWriter, _ *http.Request) {
		// Placeholder; Prometheus exposition wired in Epic R.
		_, _ = w.Write([]byte("# open-connect metrics placeholder\n"))
	})

	r.Route("/v1/devices", func(r chi.Router) {
		r.With(rbac.Require(s.Resolver, rbac.RoleDeviceRegister)).Post("/", s.createDevice)
		r.With(rbac.Require(s.Resolver, rbac.RoleDeviceRead)).Get("/", s.listDevices)
		r.With(rbac.Require(s.Resolver, rbac.RoleDeviceRead)).Get("/{serial}", s.getDevice)
		r.With(rbac.Require(s.Resolver, rbac.RoleDeviceTag)).Patch("/{serial}/tags", s.patchTags)
		r.With(rbac.Require(s.Resolver, rbac.RoleDeviceRetire)).Post("/{serial}/retire", s.retireDevice)
	})

	r.Route("/v1/claims", func(r chi.Router) {
		r.With(rbac.Require(s.Resolver, rbac.RolePipelineCreate)).Post("/", s.createClaim)
		r.With(rbac.Require(s.Resolver, rbac.RolePipelineRead)).Get("/", s.listClaims)
		r.With(rbac.Require(s.Resolver, rbac.RolePipelineRead)).Get("/{id}", s.getClaim)
		r.With(rbac.Require(s.Resolver, rbac.RolePipelineCreate)).Delete("/{id}", s.releaseClaim)
	})

	return r
}

// --- Devices ---------------------------------------------------------------

type deviceDTO struct {
	Serial          string    `json:"serial"`
	Tags            []string  `json:"tags"`
	PublicKey       string    `json:"public_key"`
	NATSNKey        string    `json:"nats_nkey,omitempty"`
	AgentVersion    string    `json:"agent_version,omitempty"`
	ActivePartition string    `json:"active_partition,omitempty"`
	LastHeartbeat   time.Time `json:"last_heartbeat,omitempty"`
	Retired         bool      `json:"retired,omitempty"`
}

func toDeviceDTO(d devices.Device) deviceDTO {
	return deviceDTO{
		Serial: d.Serial, Tags: d.Tags, PublicKey: d.PublicKey,
		NATSNKey: d.NATSNKey, AgentVersion: d.AgentVersion,
		ActivePartition: d.ActivePartition, LastHeartbeat: d.LastHeartbeat,
		Retired: d.Retired,
	}
}

func (s *Server) createDevice(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Serial      string   `json:"serial"`
		PublicKey   string   `json:"public_key"`
		NATSNKey    string   `json:"nats_nkey,omitempty"`
		InitialTags []string `json:"initial_tags,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		httperr.Write(w, http.StatusBadRequest, "invalid body", err.Error())
		return
	}
	if in.Serial == "" || in.PublicKey == "" {
		httperr.Write(w, http.StatusBadRequest, "missing fields", "serial and public_key are required")
		return
	}
	d := devices.Device{
		Serial: in.Serial, PublicKey: in.PublicKey,
		NATSNKey: in.NATSNKey, Tags: in.InitialTags,
	}
	if err := s.Devices.Create(d); err != nil {
		if errors.Is(err, devices.ErrAlreadyExists) {
			httperr.Write(w, http.StatusConflict, "already exists", err.Error())
			return
		}
		httperr.Write(w, http.StatusBadRequest, "create failed", err.Error())
		return
	}
	got, _ := s.Devices.Get(in.Serial)
	writeJSON(w, http.StatusCreated, toDeviceDTO(got))
}

func (s *Server) getDevice(w http.ResponseWriter, r *http.Request) {
	d, err := s.Devices.Get(chi.URLParam(r, "serial"))
	if err != nil {
		httperr.Write(w, http.StatusNotFound, "not found", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, toDeviceDTO(d))
}

func (s *Server) listDevices(w http.ResponseWriter, r *http.Request) {
	tags := r.URL.Query()["tag"]
	got, _ := s.Devices.List(tags)
	out := make([]deviceDTO, 0, len(got))
	for _, d := range got {
		out = append(out, toDeviceDTO(d))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) patchTags(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Add    []string `json:"add"`
		Remove []string `json:"remove"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		httperr.Write(w, http.StatusBadRequest, "invalid body", err.Error())
		return
	}
	d, err := s.Devices.UpdateTags(chi.URLParam(r, "serial"), in.Add, in.Remove)
	if err != nil {
		httperr.Write(w, http.StatusNotFound, "not found", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, toDeviceDTO(d))
}

func (s *Server) retireDevice(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Reason string `json:"reason"`
	}
	_ = json.NewDecoder(r.Body).Decode(&in)
	if err := s.Devices.Retire(chi.URLParam(r, "serial"), in.Reason); err != nil {
		httperr.Write(w, http.StatusNotFound, "not found", err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- Claims ----------------------------------------------------------------

type claimDeviceDTO struct {
	Serial          string    `json:"serial"`
	State           string    `json:"state"`
	LastUpdate      time.Time `json:"last_update_timestamp"`
}

type claimDTO struct {
	ID                       string           `json:"claim_id"`
	State                    string           `json:"state"`
	Count                    uint32           `json:"count"`
	RequiredTags             []string         `json:"required_tags"`
	DesiredVersion           string           `json:"desired_version,omitempty"`
	TTLSeconds               uint32           `json:"ttl_seconds"`
	PreparationTimeoutSecs   uint32           `json:"preparation_timeout_seconds,omitempty"`
	RequestedBy              string           `json:"requested_by,omitempty"`
	CreatedAt                time.Time        `json:"created_at"`
	ExpiresAt                time.Time        `json:"expires_at"`
	Devices                  []claimDeviceDTO `json:"devices"`
}

func toClaimDTO(c *claims.Claim) claimDTO {
	d := make([]claimDeviceDTO, 0, len(c.Devices))
	for _, dl := range c.Devices {
		d = append(d, claimDeviceDTO{
			Serial: dl.Serial, State: string(dl.State), LastUpdate: dl.UpdatedAt,
		})
	}
	return claimDTO{
		ID: c.ID, State: string(c.State), Count: c.Count,
		RequiredTags: c.RequiredTags, DesiredVersion: c.DesiredVersion,
		TTLSeconds:             uint32(c.TTL.Seconds()),
		PreparationTimeoutSecs: uint32(c.PreparationTimeout.Seconds()),
		RequestedBy:            c.RequestedBy,
		CreatedAt:              c.CreatedAt,
		ExpiresAt:              c.ExpiresAt,
		Devices:                d,
	}
}

func (s *Server) createClaim(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Count                  uint32   `json:"count"`
		Tags                   []string `json:"tags"`
		DesiredVersion         string   `json:"desired_version"`
		TTLSeconds             uint32   `json:"ttl_seconds"`
		PreparationTimeoutSecs uint32   `json:"preparation_timeout_seconds"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		httperr.Write(w, http.StatusBadRequest, "invalid body", err.Error())
		return
	}
	c, err := s.Claims.Create(claims.CreateRequest{
		Count: in.Count, RequiredTags: in.Tags, DesiredVersion: in.DesiredVersion,
		TTL:                time.Duration(in.TTLSeconds) * time.Second,
		PreparationTimeout: time.Duration(in.PreparationTimeoutSecs) * time.Second,
		RequestedBy:        rbac.SubjectFromRequest(r),
	})
	if err != nil {
		httperr.Write(w, http.StatusBadRequest, "invalid claim", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, toClaimDTO(c))
}

func (s *Server) getClaim(w http.ResponseWriter, r *http.Request) {
	c, err := s.Claims.Get(chi.URLParam(r, "id"))
	if err != nil {
		httperr.Write(w, http.StatusNotFound, "not found", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, toClaimDTO(c))
}

func (s *Server) listClaims(w http.ResponseWriter, _ *http.Request) {
	cs := s.Claims.List()
	out := make([]claimDTO, 0, len(cs))
	for _, c := range cs {
		out = append(out, toClaimDTO(c))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) releaseClaim(w http.ResponseWriter, r *http.Request) {
	if err := s.Claims.Release(chi.URLParam(r, "id")); err != nil {
		switch {
		case errors.Is(err, claims.ErrNotFound):
			httperr.Write(w, http.StatusNotFound, "not found", err.Error())
		case errors.Is(err, claims.ErrAlreadyClosed):
			httperr.Write(w, http.StatusConflict, "already closed", err.Error())
		default:
			httperr.Write(w, http.StatusInternalServerError, "release failed", err.Error())
		}
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- helpers ---------------------------------------------------------------

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// NewClaimID returns a UUIDv4-based claim ID.
func NewClaimID() string { return uuid.NewString() }

// NewLeaseID returns a UUIDv4-based lease ID.
func NewLeaseID() string { return strings.ReplaceAll(uuid.NewString(), "-", "") }

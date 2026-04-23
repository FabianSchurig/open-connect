// Package claims implements the Claim Registry (arc42 §5.7.2, FR-07/15/17,
// NFR-08/09, ADR-0005).
//
// Design notes:
//
//   - The state machine is the one drawn in §5.7.2 of the building-block view.
//   - Lock acquisition is atomic: the in-memory implementation uses a per-claim
//     mutex; the eventual Postgres implementation MUST use SELECT … FOR UPDATE
//     SKIP LOCKED or a Postgres advisory lock, satisfying NFR-09 (linearizable
//     locks). Both implementations satisfy the same Store contract (and the
//     same property test).
//   - TTL sweeping is a Sweep() method invoked either by an internal goroutine
//     or by the Postgres-leader-elected loop; it transitions Open|Locked|InUse
//     claims past their deadline to Expired/Released. The clock is injected
//     for tests (NFR-08 force-release timing).
//   - The Service emits NATS offers when claims open (claim.offer.<tag>) and
//     release notifications when claims close (claim.release.<claim_id>) via
//     the injected nats.Publisher.
package claims

import (
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/FabianSchurig/open-connect/services/control-plane/internal/clock"
	natsx "github.com/FabianSchurig/open-connect/services/control-plane/internal/nats"
)

// State is the server-side claim state machine of §5.7.2.
type State string

const (
	StateOpen            State = "Open"
	StateOffered         State = "Offered"
	StatePartiallyLocked State = "PartiallyLocked"
	StateLocked          State = "Locked"
	StatePreparing       State = "Preparing"
	StateReady           State = "Ready"
	StateInUse           State = "InUse"
	StateReleased        State = "Released"
	StateExpired         State = "Expired"
)

// DeviceState is the per-device sub-state reported via FR-17.
type DeviceState string

const (
	DevicePending   DeviceState = "Pending"
	DevicePreparing DeviceState = "Preparing"
	DeviceReady     DeviceState = "Ready"
	DeviceFailed    DeviceState = "Failed"
	DeviceInUse     DeviceState = "InUse"
	DeviceReleased  DeviceState = "Released"
)

var (
	ErrNotFound      = errors.New("claim not found")
	ErrNoSlots       = errors.New("no slots remaining")
	ErrTagsMismatch  = errors.New("device tags do not satisfy claim required tags")
	ErrAlreadyClosed = errors.New("claim already terminated")
	ErrInvalidLease  = errors.New("invalid or expired lease")
)

// Claim is the persisted aggregate.
type Claim struct {
	ID                       string
	Count                    uint32
	RequiredTags             []string
	DesiredVersion           string
	TTL                      time.Duration
	PreparationTimeout       time.Duration
	RequestedBy              string
	State                    State
	CreatedAt                time.Time
	ExpiresAt                time.Time
	Devices                  map[string]*DeviceLock // serial -> lock
}

// DeviceLock tracks one slot in a claim.
type DeviceLock struct {
	Serial      string
	LeaseID     string
	State       DeviceState
	LockedAt    time.Time
	UpdatedAt   time.Time
}

// SlotsRemaining returns how many additional locks may still be granted.
func (c *Claim) SlotsRemaining() uint32 {
	return c.Count - uint32(len(c.Devices))
}

// LockResult is what TryLock returns.
type LockResult struct {
	Granted                  bool
	LeaseID                  string
	PreparationTimeoutSecs   uint32
	Reason                   string
}

// Service is the in-memory MVP implementation of the Claim Registry.
// The Postgres implementation behind the same interface arrives in a follow-up.
type Service struct {
	mu      sync.Mutex
	claims  map[string]*Claim
	clock   clock.Clock
	pub     natsx.Publisher
	idGen   func() string
	leaseID func() string
}

// New constructs a Service. Pass nil for pub to disable NATS notifications
// (used by some unit tests).
func New(c clock.Clock, pub natsx.Publisher, idGen, leaseGen func() string) *Service {
	return &Service{
		claims:  map[string]*Claim{},
		clock:   c,
		pub:     pub,
		idGen:   idGen,
		leaseID: leaseGen,
	}
}

// Create accepts a ClaimRequest, persists it, and emits offer notifications
// to claim.offer.<tag>. Claim state begins as Open.
func (s *Service) Create(req CreateRequest) (*Claim, error) {
	if req.Count == 0 {
		return nil, fmt.Errorf("count must be >= 1")
	}
	if len(req.RequiredTags) == 0 {
		return nil, fmt.Errorf("required_tags must be non-empty")
	}
	if req.TTL <= 0 {
		return nil, fmt.Errorf("ttl_seconds must be > 0")
	}
	now := s.clock.Now()
	c := &Claim{
		ID:                 s.idGen(),
		Count:              req.Count,
		RequiredTags:       append([]string(nil), req.RequiredTags...),
		DesiredVersion:     req.DesiredVersion,
		TTL:                req.TTL,
		PreparationTimeout: req.PreparationTimeout,
		RequestedBy:        req.RequestedBy,
		State:              StateOpen,
		CreatedAt:          now,
		ExpiresAt:          now.Add(req.TTL),
		Devices:            map[string]*DeviceLock{},
	}
	sort.Strings(c.RequiredTags)
	s.mu.Lock()
	s.claims[c.ID] = c
	s.mu.Unlock()
	s.publishOffer(c)
	return c.cloneLocked(), nil
}

type CreateRequest struct {
	Count              uint32
	RequiredTags       []string
	DesiredVersion     string
	TTL                time.Duration
	PreparationTimeout time.Duration
	RequestedBy        string
}

// Get returns a deep copy.
func (s *Service) Get(id string) (*Claim, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.claims[id]
	if !ok {
		return nil, ErrNotFound
	}
	return c.cloneLocked(), nil
}

// List returns all claims (sorted by CreatedAt desc); used by the API.
func (s *Service) List() []*Claim {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]*Claim, 0, len(s.claims))
	for _, c := range s.claims {
		out = append(out, c.cloneLocked())
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out
}

// TryLock attempts to grant one slot of claim `id` to device `serial`.
//
// This is the linearizable lock acquisition required by NFR-09. In MVP the
// linearizability comes from the single Service mutex; the Postgres
// implementation must use SELECT FOR UPDATE SKIP LOCKED to get the same
// guarantee across processes.
func (s *Service) TryLock(id, serial string, deviceTags []string) (LockResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.claims[id]
	if !ok {
		return LockResult{}, ErrNotFound
	}
	if isTerminal(c.State) {
		return LockResult{Granted: false, Reason: "claim closed"}, nil
	}
	if c.ExpiresAt.Before(s.clock.Now()) {
		c.State = StateExpired
		return LockResult{Granted: false, Reason: "expired"}, nil
	}
	if !hasAllTags(deviceTags, c.RequiredTags) {
		return LockResult{Granted: false, Reason: ErrTagsMismatch.Error()}, nil
	}
	if _, already := c.Devices[serial]; already {
		// Idempotent re-lock returns the existing lease.
		dl := c.Devices[serial]
		return LockResult{
			Granted: true,
			LeaseID: dl.LeaseID,
			PreparationTimeoutSecs: uint32(c.PreparationTimeout.Seconds()),
		}, nil
	}
	if c.SlotsRemaining() == 0 {
		return LockResult{Granted: false, Reason: ErrNoSlots.Error()}, nil
	}
	now := s.clock.Now()
	dl := &DeviceLock{
		Serial:    serial,
		LeaseID:   s.leaseID(),
		State:     DevicePreparing,
		LockedAt:  now,
		UpdatedAt: now,
	}
	c.Devices[serial] = dl
	transitionAfterLock(c)
	return LockResult{
		Granted: true,
		LeaseID: dl.LeaseID,
		PreparationTimeoutSecs: uint32(c.PreparationTimeout.Seconds()),
	}, nil
}

// ReportDeviceState is called when an agent reports progress on its lock
// (e.g. Preparing -> Ready). Returns the updated claim snapshot.
func (s *Service) ReportDeviceState(id, leaseID string, ds DeviceState) (*Claim, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.claims[id]
	if !ok {
		return nil, ErrNotFound
	}
	dl, ok := findLeaseLocked(c, leaseID)
	if !ok {
		return nil, ErrInvalidLease
	}
	dl.State = ds
	dl.UpdatedAt = s.clock.Now()
	transitionAfterReport(c)
	return c.cloneLocked(), nil
}

// Release is called by the pipeline (DELETE /v1/claims/{id}). It marks the
// claim Released and emits claim.release.<id>.
func (s *Service) Release(id string) error {
	s.mu.Lock()
	c, ok := s.claims[id]
	if !ok {
		s.mu.Unlock()
		return ErrNotFound
	}
	if isTerminal(c.State) {
		s.mu.Unlock()
		return ErrAlreadyClosed
	}
	c.State = StateReleased
	for _, dl := range c.Devices {
		dl.State = DeviceReleased
		dl.UpdatedAt = s.clock.Now()
	}
	id2 := c.ID
	s.mu.Unlock()
	s.publishRelease(id2)
	return nil
}

// Sweep force-releases expired claims (NFR-08). Should run on a sub-30s tick
// in MVP via Service.RunSweeper.
func (s *Service) Sweep() (released, expired int) {
	s.mu.Lock()
	now := s.clock.Now()
	type closer struct {
		id     string
		notify bool
	}
	var toNotify []closer
	for _, c := range s.claims {
		if isTerminal(c.State) {
			continue
		}
		// Per-claim TTL.
		if c.ExpiresAt.Before(now) {
			if c.State == StateInUse || c.State == StateReady || c.State == StatePreparing {
				c.State = StateReleased
				released++
				toNotify = append(toNotify, closer{c.ID, true})
			} else {
				c.State = StateExpired
				expired++
			}
			for _, dl := range c.Devices {
				dl.State = DeviceReleased
				dl.UpdatedAt = now
			}
			continue
		}
		// Per-lock preparation timeout — reclaim slow lockers (FR-15).
		if c.PreparationTimeout > 0 {
			for serial, dl := range c.Devices {
				if dl.State == DevicePreparing && now.Sub(dl.LockedAt) > c.PreparationTimeout {
					delete(c.Devices, serial)
					released++
				}
			}
			recalcStateAfterReclaim(c)
		}
	}
	s.mu.Unlock()
	for _, n := range toNotify {
		if n.notify {
			s.publishRelease(n.id)
		}
	}
	return
}

// RunSweeper runs Sweep every interval until ctx-equivalent stop chan closes.
// Designed so the caller can leader-elect via Postgres lease (post-MVP) or
// just run it on a single replica (MVP).
func (s *Service) RunSweeper(interval time.Duration, stop <-chan struct{}) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-stop:
			return
		case <-t.C:
			s.Sweep()
		}
	}
}

func (s *Service) publishOffer(c *Claim) {
	if s.pub == nil {
		return
	}
	for _, tag := range c.RequiredTags {
		_ = s.pub.Publish("claim.offer."+tag, []byte(c.ID))
	}
}

func (s *Service) publishRelease(id string) {
	if s.pub == nil {
		return
	}
	_ = s.pub.Publish("claim.release."+id, []byte(id))
}

// --- internal helpers ---------------------------------------------------

func transitionAfterLock(c *Claim) {
	if uint32(len(c.Devices)) >= c.Count {
		c.State = StateLocked
	} else if len(c.Devices) > 0 {
		c.State = StatePartiallyLocked
	}
}

func transitionAfterReport(c *Claim) {
	if c.State == StateInUse || c.State == StateReleased || c.State == StateExpired {
		return
	}
	preparing := 0
	ready := 0
	for _, dl := range c.Devices {
		if dl.State == DevicePreparing {
			preparing++
		} else if dl.State == DeviceReady {
			ready++
		}
	}
	if uint32(ready) == c.Count {
		c.State = StateReady
		return
	}
	if preparing > 0 {
		c.State = StatePreparing
	}
}

func recalcStateAfterReclaim(c *Claim) {
	switch {
	case len(c.Devices) == 0:
		c.State = StateOpen
	case uint32(len(c.Devices)) < c.Count:
		c.State = StatePartiallyLocked
	}
}

func isTerminal(s State) bool { return s == StateReleased || s == StateExpired }

func findLeaseLocked(c *Claim, leaseID string) (*DeviceLock, bool) {
	for _, dl := range c.Devices {
		if dl.LeaseID == leaseID {
			return dl, true
		}
	}
	return nil, false
}

func hasAllTags(have, need []string) bool {
	set := map[string]struct{}{}
	for _, t := range have {
		set[t] = struct{}{}
	}
	for _, t := range need {
		if _, ok := set[t]; !ok {
			return false
		}
	}
	return true
}

func (c *Claim) cloneLocked() *Claim {
	out := *c
	out.RequiredTags = append([]string(nil), c.RequiredTags...)
	out.Devices = make(map[string]*DeviceLock, len(c.Devices))
	for k, v := range c.Devices {
		dl := *v
		out.Devices[k] = &dl
	}
	return &out
}

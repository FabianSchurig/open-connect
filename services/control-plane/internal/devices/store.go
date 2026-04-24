// Package devices provides the device-registration & tagging service (FR-22).
//
// The store is interface-driven so that the in-memory implementation backs
// unit tests and the Postgres implementation backs production. Both must
// satisfy the same contract tests.
package devices

import (
	"errors"
	"sort"
	"sync"
	"time"
)

var (
	ErrAlreadyExists = errors.New("device already exists")
	ErrNotFound      = errors.New("device not found")
)

// Device mirrors the otap.v1.Device proto for storage purposes.
// The proto type is the wire contract; this struct is the storage row.
type Device struct {
	Serial         string
	Tags           []string
	PublicKey      string // PEM-encoded Ed25519
	NATSNKey       string // optional in MVP
	AgentVersion   string
	ActivePartition string
	LastHeartbeat  time.Time
	Retired        bool
	RetiredReason  string
}

// Store abstracts the persistence layer.
type Store interface {
	Create(d Device) error
	Get(serial string) (Device, error)
	List(tagFilterAND []string) ([]Device, error)
	UpdateTags(serial string, add, remove []string) (Device, error)
	Retire(serial, reason string) error
}

// MemStore is the in-process implementation used by tests and dev.
type MemStore struct {
	mu      sync.RWMutex
	devices map[string]Device
}

func NewMemStore() *MemStore { return &MemStore{devices: map[string]Device{}} }

func (m *MemStore) Create(d Device) error {
	if d.Serial == "" {
		return errors.New("serial required")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.devices[d.Serial]; ok {
		return ErrAlreadyExists
	}
	d.Tags = uniqueSorted(d.Tags)
	m.devices[d.Serial] = d
	return nil
}

func (m *MemStore) Get(serial string) (Device, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	d, ok := m.devices[serial]
	if !ok {
		return Device{}, ErrNotFound
	}
	return d, nil
}

func (m *MemStore) List(tagFilterAND []string) ([]Device, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]Device, 0, len(m.devices))
	for _, d := range m.devices {
		if d.Retired {
			continue
		}
		if hasAllTags(d.Tags, tagFilterAND) {
			out = append(out, d)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Serial < out[j].Serial })
	return out, nil
}

func (m *MemStore) UpdateTags(serial string, add, remove []string) (Device, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	d, ok := m.devices[serial]
	if !ok {
		return Device{}, ErrNotFound
	}
	tagSet := map[string]struct{}{}
	for _, t := range d.Tags {
		tagSet[t] = struct{}{}
	}
	for _, t := range add {
		tagSet[t] = struct{}{}
	}
	for _, t := range remove {
		delete(tagSet, t)
	}
	d.Tags = make([]string, 0, len(tagSet))
	for t := range tagSet {
		d.Tags = append(d.Tags, t)
	}
	d.Tags = uniqueSorted(d.Tags)
	m.devices[serial] = d
	return d, nil
}

func (m *MemStore) Retire(serial, reason string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	d, ok := m.devices[serial]
	if !ok {
		return ErrNotFound
	}
	d.Retired = true
	d.RetiredReason = reason
	m.devices[serial] = d
	return nil
}

func hasAllTags(have, need []string) bool {
	if len(need) == 0 {
		return true
	}
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

func uniqueSorted(in []string) []string {
	set := map[string]struct{}{}
	for _, t := range in {
		if t == "" {
			continue
		}
		set[t] = struct{}{}
	}
	out := make([]string, 0, len(set))
	for t := range set {
		out = append(out, t)
	}
	sort.Strings(out)
	return out
}

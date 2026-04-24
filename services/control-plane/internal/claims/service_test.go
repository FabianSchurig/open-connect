package claims

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/FabianSchurig/open-connect/services/control-plane/internal/clock"
	natsx "github.com/FabianSchurig/open-connect/services/control-plane/internal/nats"
)

func newSvc(t *testing.T, c clock.Clock) (*Service, *natsx.MemBus) {
	t.Helper()
	bus := natsx.NewMemBus()
	var i, j atomic.Uint64
	id := func() string { return fmt.Sprintf("claim-%d", i.Add(1)) }
	lease := func() string { return fmt.Sprintf("lease-%d", j.Add(1)) }
	return New(c, bus, id, lease), bus
}

// NFR-09 — under N concurrent lockers exactly `count` succeed.
func TestTryLock_LinearizableUnderConcurrency(t *testing.T) {
	const N = 200
	const count = 7
	clk := &clock.Fixed{T: time.Date(2026, 4, 23, 0, 0, 0, 0, time.UTC)}
	svc, _ := newSvc(t, clk)

	c, err := svc.Create(CreateRequest{
		Count:        count,
		RequiredTags: []string{"yocto-wic-ab"},
		TTL:          time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}

	var granted atomic.Uint64
	var wg sync.WaitGroup
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			res, err := svc.TryLock(c.ID, fmt.Sprintf("DEV-%03d", i), []string{"yocto-wic-ab", "x86"})
			if err != nil {
				t.Errorf("unexpected: %v", err)
				return
			}
			if res.Granted {
				granted.Add(1)
			}
		}(i)
	}
	wg.Wait()
	if g := granted.Load(); g != count {
		t.Fatalf("NFR-09 violated: granted=%d want=%d", g, count)
	}

	got, _ := svc.Get(c.ID)
	if got.State != StateLocked {
		t.Fatalf("state=%s want=Locked", got.State)
	}
	if uint32(len(got.Devices)) != count {
		t.Fatalf("devices=%d want=%d", len(got.Devices), count)
	}
}

func TestTryLock_TagsMustMatch(t *testing.T) {
	clk := &clock.Fixed{T: time.Now().UTC()}
	svc, _ := newSvc(t, clk)
	c, _ := svc.Create(CreateRequest{Count: 1, RequiredTags: []string{"yocto-wic-ab"}, TTL: time.Hour})

	r, _ := svc.TryLock(c.ID, "DEV-1", []string{"some-other-tag"})
	if r.Granted {
		t.Fatal("must not grant when tags missing")
	}
}

// NFR-08 — TTL expiry triggers force-release within the configured budget.
func TestSweep_ForceReleaseExpiredClaim(t *testing.T) {
	clk := &clock.Fixed{T: time.Date(2026, 4, 23, 0, 0, 0, 0, time.UTC)}
	svc, _ := newSvc(t, clk)
	c, _ := svc.Create(CreateRequest{Count: 1, RequiredTags: []string{"x"}, TTL: 10 * time.Second})
	_, _ = svc.TryLock(c.ID, "DEV-1", []string{"x"})
	_, _ = svc.ReportDeviceState(c.ID, "lease-1", DeviceReady)
	_, _ = svc.ReportDeviceState(c.ID, "lease-1", DeviceInUse)

	// Advance well past the 10s TTL so the sweep MUST force-release the
	// claim (NFR-08: release latency budget is 30s past expiry).
	clk.Add(41 * time.Second)
	released, _ := svc.Sweep()
	if released == 0 {
		t.Fatalf("expected sweep to release; released=%d", released)
	}
	got, _ := svc.Get(c.ID)
	if got.State != StateReleased && got.State != StateExpired {
		t.Fatalf("state=%s want Released/Expired", got.State)
	}
}

// FR-15 — preparation timeout reclaims slow lockers.
func TestSweep_PreparationTimeoutReclaimsLock(t *testing.T) {
	clk := &clock.Fixed{T: time.Date(2026, 4, 23, 0, 0, 0, 0, time.UTC)}
	svc, _ := newSvc(t, clk)
	c, _ := svc.Create(CreateRequest{
		Count: 1, RequiredTags: []string{"x"},
		TTL: time.Hour, PreparationTimeout: 30 * time.Second,
	})
	r, _ := svc.TryLock(c.ID, "DEV-slow", []string{"x"})
	if !r.Granted {
		t.Fatal("first lock must succeed")
	}

	clk.Add(45 * time.Second) // exceed prep timeout
	svc.Sweep()

	got, _ := svc.Get(c.ID)
	if len(got.Devices) != 0 {
		t.Fatalf("slow lock not reclaimed; devices=%d", len(got.Devices))
	}
	if got.State != StateOpen {
		t.Fatalf("state after reclaim=%s want Open", got.State)
	}

	// A different device must now be able to lock.
	r, _ = svc.TryLock(c.ID, "DEV-fast", []string{"x"})
	if !r.Granted {
		t.Fatal("re-lock after reclaim must succeed")
	}
}

func TestRelease_PublishesNotification(t *testing.T) {
	clk := &clock.Fixed{T: time.Now().UTC()}
	svc, bus := newSvc(t, clk)
	c, _ := svc.Create(CreateRequest{Count: 1, RequiredTags: []string{"x"}, TTL: time.Hour})
	if err := svc.Release(c.ID); err != nil {
		t.Fatal(err)
	}
	if msgs := bus.PublishedOn("claim.release." + c.ID); len(msgs) == 0 {
		t.Fatal("expected claim.release notification")
	}
}

func TestCreate_PublishesOfferPerTag(t *testing.T) {
	clk := &clock.Fixed{T: time.Now().UTC()}
	svc, bus := newSvc(t, clk)
	_, _ = svc.Create(CreateRequest{Count: 2, RequiredTags: []string{"yocto", "x86"}, TTL: time.Hour})
	if len(bus.PublishedOn("claim.offer.yocto")) != 1 {
		t.Fatal("missing offer for yocto")
	}
	if len(bus.PublishedOn("claim.offer.x86")) != 1 {
		t.Fatal("missing offer for x86")
	}
}

func TestReadyTransition(t *testing.T) {
	clk := &clock.Fixed{T: time.Now().UTC()}
	svc, _ := newSvc(t, clk)
	c, _ := svc.Create(CreateRequest{Count: 2, RequiredTags: []string{"x"}, TTL: time.Hour})
	r1, _ := svc.TryLock(c.ID, "A", []string{"x"})
	r2, _ := svc.TryLock(c.ID, "B", []string{"x"})
	_, _ = svc.ReportDeviceState(c.ID, r1.LeaseID, DeviceReady)
	got, _ := svc.ReportDeviceState(c.ID, r2.LeaseID, DeviceReady)
	if got.State != StateReady {
		t.Fatalf("state=%s want Ready", got.State)
	}
}

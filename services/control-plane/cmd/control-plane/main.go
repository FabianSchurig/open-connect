// Open-Connect control-plane API server (MVP).
//
// In MVP this binary boots an in-memory store and an in-memory NATS bus so
// that it is fully runnable for demos, integration tests, and CI without
// external dependencies. The Postgres + real NATS adapters drop in behind the
// same interfaces in a follow-up PR.
package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/FabianSchurig/open-connect/services/control-plane/internal/api"
	"github.com/FabianSchurig/open-connect/services/control-plane/internal/claims"
	"github.com/FabianSchurig/open-connect/services/control-plane/internal/clock"
	"github.com/FabianSchurig/open-connect/services/control-plane/internal/devices"
	natsx "github.com/FabianSchurig/open-connect/services/control-plane/internal/nats"
	"github.com/FabianSchurig/open-connect/services/control-plane/internal/rbac"
)

func main() {
	addr := flag.String("addr", ":8080", "HTTP listen address")
	sweepEvery := flag.Duration("sweep", 5*time.Second, "TTL sweeper interval (NFR-08)")
	flag.Parse()

	bus := natsx.NewMemBus()
	svc := claims.New(clock.Real{}, bus, api.NewClaimID, api.NewLeaseID)

	// Dev RBAC: a single privileged subject. Production wires this to OIDC.
	res := rbac.StaticResolver{
		"dev-admin": {
			rbac.RoleDeviceRegister, rbac.RoleDeviceRead, rbac.RoleDeviceTag, rbac.RoleDeviceRetire,
			rbac.RolePipelineCreate, rbac.RolePipelineRead,
			rbac.RoleReleasePublish, rbac.RoleReleaseRead,
			rbac.RoleAuditRead, rbac.RoleAuditExport,
		},
	}
	srv := &api.Server{
		Devices:  devices.NewMemStore(),
		Claims:   svc,
		Resolver: res,
	}
	httpSrv := &http.Server{
		Addr:              *addr,
		Handler:           srv.Router(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	stop := make(chan struct{})
	go svc.RunSweeper(*sweepEvery, stop)

	go func() {
		log.Printf("open-connect control-plane listening on %s", *addr)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %v", err)
		}
	}()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	close(stop)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = httpSrv.Shutdown(ctx)
}

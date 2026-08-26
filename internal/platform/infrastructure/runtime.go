package infrastructure

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"sync/atomic"
	"time"

	algapp "github.com/enterprise-labs/quota-decision-plane/internal/algorithm/application"
	auditapp "github.com/enterprise-labs/quota-decision-plane/internal/audit/application"
	auditmem "github.com/enterprise-labs/quota-decision-plane/internal/audit/infrastructure"
	cfgapp "github.com/enterprise-labs/quota-decision-plane/internal/configuration/application"
	cfgmem "github.com/enterprise-labs/quota-decision-plane/internal/configuration/infrastructure"
	idemapp "github.com/enterprise-labs/quota-decision-plane/internal/idempotency/application"
	idemmem "github.com/enterprise-labs/quota-decision-plane/internal/idempotency/infrastructure"
	leaseapp "github.com/enterprise-labs/quota-decision-plane/internal/lease/application"
	platformadapter "github.com/enterprise-labs/quota-decision-plane/internal/platform/adapter"
	platformapp "github.com/enterprise-labs/quota-decision-plane/internal/platform/application"
	platformdomain "github.com/enterprise-labs/quota-decision-plane/internal/platform/domain"
	quotaapp "github.com/enterprise-labs/quota-decision-plane/internal/quota/application"
	quota "github.com/enterprise-labs/quota-decision-plane/internal/quota/domain"
	resapp "github.com/enterprise-labs/quota-decision-plane/internal/reservation/application"
	resmem "github.com/enterprise-labs/quota-decision-plane/internal/reservation/infrastructure"
	shardapp "github.com/enterprise-labs/quota-decision-plane/internal/shard/application"
	shard "github.com/enterprise-labs/quota-decision-plane/internal/shard/domain"
)

type Runtime struct {
	Config             platformapp.Config
	Logger             *slog.Logger
	Server             *http.Server
	Configuration      *cfgapp.Service
	Engine             *quotaapp.Engine
	AuditSink          *auditmem.MemorySink
	AuditBus           *auditapp.Bus
	ReservationService *resapp.Service
	LeaseManager       *leaseapp.Manager
	Ring               *shardapp.Ring
	ready              atomic.Bool
}

func NewRuntime(ctx context.Context, cfg platformapp.Config, logger *slog.Logger) (*Runtime, error) {
	clock := platformdomain.RealClock{}
	configRepo := cfgmem.NewMemoryRepository()
	configuration := cfgapp.NewService(configRepo, clock)
	if err := configuration.Bootstrap(ctx, DefaultDefinitions()); err != nil {
		return nil, fmt.Errorf("bootstrap definitions: %w", err)
	}
	auditSink := auditmem.NewMemorySink(10000)
	auditBus := auditapp.NewBus(auditSink, 2048)
	algorithms := algapp.NewRegistry()
	ring := shardapp.NewRing(128, 2)
	ring.SetNodes([]shard.Node{{ID: cfg.NodeID, Address: cfg.Address, Weight: 2, Healthy: true, LastSeen: clock.Now()}, {ID: "quota-node-standby", Address: ":18334", Weight: 1, Healthy: true, LastSeen: clock.Now()}})
	leaseManager := leaseapp.NewManager(clock)
	engine := quotaapp.NewEngine(configuration, quotaapp.NewMatcher(), algorithms, auditBus, clock, func(key string) uint64 {
		assignment, ok := ring.Locate(key)
		if !ok {
			return 0
		}
		l, err := leaseManager.Acquire(assignment.Shard, cfg.NodeID, 30*time.Second)
		if err != nil {
			return assignment.Epoch
		}
		return l.Epoch
	})
	reservationRepo := resmem.NewMemoryRepository()
	reservationService := resapp.NewService(reservationRepo, clock, engine.Release)
	engine.SetReservations(reservationService)
	idempotency := idemapp.NewService(idemmem.NewMemory(), 24*time.Hour)
	metrics := platformapp.NewMetrics()
	runtime := &Runtime{Config: cfg, Logger: logger, Configuration: configuration, Engine: engine, AuditSink: auditSink, AuditBus: auditBus, ReservationService: reservationService, LeaseManager: leaseManager, Ring: ring}
	api := platformadapter.NewAPI(platformadapter.Dependencies{Engine: engine, Configuration: configuration, Idempotency: idempotency, Audit: auditSink, Ring: ring, Leases: leaseManager, Metrics: metrics, AuditDropped: auditBus.Dropped, Logger: logger, Ready: runtime.ready.Load})
	middleware := platformadapter.NewMiddleware(cfg, logger, metrics)
	runtime.Server = &http.Server{Addr: cfg.Address, Handler: middleware.Wrap(api.Routes()), ReadHeaderTimeout: 3 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 32 << 10}
	runtime.ready.Store(true)
	return runtime, nil
}
func (r *Runtime) RunBackground(ctx context.Context) {
	go r.AuditBus.Run(ctx)
	go r.ReservationService.RunReaper(ctx, r.Config.ReservationReap)
}
func (r *Runtime) Shutdown(ctx context.Context) error {
	r.ready.Store(false)
	if err := r.Server.Shutdown(ctx); err != nil {
		return fmt.Errorf("shutdown HTTP server: %w", err)
	}
	return nil
}
func DefaultDefinitions() []quota.Definition {
	return []quota.Definition{{ID: "global-api", TenantID: "demo", Resource: "api_calls", Version: 1, Algorithm: quota.FixedWindow, Limit: 100, Window: time.Minute, Dimensions: quota.Dimensions{Service: "*", Method: "*", Region: "*", Customer: "*"}, Mode: "strong", Enabled: true, Description: "global tenant API budget"}, {ID: "payments-post", TenantID: "demo", Resource: "api_calls", Version: 1, Algorithm: quota.TokenBucket, Limit: 10, Burst: 5, Window: time.Minute, Dimensions: quota.Dimensions{Service: "payments", Method: "POST", Region: "*", Customer: "*"}, ParentID: "global-api", Mode: "strong", Enabled: true, Description: "payment write token bucket"}, {ID: "search-customer", TenantID: "demo", Resource: "search", Version: 1, Algorithm: quota.SlidingWindow, Limit: 20, Window: time.Minute, Dimensions: quota.Dimensions{Service: "catalog", Method: "GET", Region: "*", Customer: "*"}, Mode: "bounded", MaxOverage: 2, Enabled: true}, {ID: "jobs-concurrent", TenantID: "demo", Resource: "jobs", Version: 1, Algorithm: quota.Semaphore, Limit: 3, Dimensions: quota.Dimensions{Service: "worker", Region: "*"}, Mode: "strong", Enabled: true}, {ID: "ingest-leaky", TenantID: "demo", Resource: "ingest", Version: 1, Algorithm: quota.LeakyBucket, Limit: 60, Burst: 10, Window: time.Minute, Dimensions: quota.Dimensions{Service: "collector", Region: "*"}, Mode: "bounded", MaxOverage: 2, Enabled: true}}
}

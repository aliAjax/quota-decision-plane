package infrastructure

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	audit "github.com/enterprise-labs/quota-decision-plane/internal/audit/domain"
	platform "github.com/enterprise-labs/quota-decision-plane/internal/platform/application"
)

func TestRuntimeShutdownClosesAuditBus(t *testing.T) {
	runCtx, cancelRun := context.WithCancel(context.Background())
	defer cancelRun()
	runtime, err := NewRuntime(runCtx, platform.DefaultConfig(), slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	runtime.RunBackground(runCtx)
	runtime.AuditBus.Publish(audit.Event{ID: "shutdown-audit"})

	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), time.Second)
	defer cancelShutdown()
	if err := runtime.Shutdown(shutdownCtx); err != nil {
		t.Fatal(err)
	}
	if !runtime.AuditBus.Closed() {
		t.Fatal("runtime shutdown left audit bus accepting events")
	}
	events, err := runtime.AuditSink.List(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].ID != "shutdown-audit" {
		t.Fatalf("events after shutdown=%+v", events)
	}
}

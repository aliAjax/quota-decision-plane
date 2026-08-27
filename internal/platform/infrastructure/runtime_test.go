package infrastructure

import (
	"bytes"
	"context"
	"encoding/json"
	platform "github.com/enterprise-labs/quota-decision-plane/internal/platform/application"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHTTPDecisionAndIdempotency(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cfg := platform.DefaultConfig()
	runtime, err := NewRuntime(ctx, cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	runtime.RunBackground(ctx)
	server := httptest.NewServer(runtime.Server.Handler)
	defer server.Close()
	payload := map[string]any{"tenant_id": "demo", "resource": "api_calls", "dimensions": map[string]string{"service": "payments", "method": "POST", "region": "ap-northeast-1", "customer": "c1"}, "cost": 1, "idempotency_key": "same"}
	raw, _ := json.Marshal(payload)
	send := func(body []byte) (*http.Response, []byte) {
		req, _ := http.NewRequest(http.MethodPost, server.URL+"/v1/check", bytes.NewReader(body))
		req.Header.Set("X-API-Key", cfg.APIKey)
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		result, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return resp, result
	}
	first, body := send(raw)
	if first.StatusCode != 200 {
		t.Fatalf("first=%d %s", first.StatusCode, body)
	}
	var decision struct {
		QuotaID string `json:"quota_id"`
	}
	if err := json.Unmarshal(body, &decision); err != nil {
		t.Fatal(err)
	}
	if decision.QuotaID != "payments-post" {
		t.Fatalf("expected leaf quota, got %q", decision.QuotaID)
	}
	second, _ := send(raw)
	if second.Header.Get("X-Idempotent-Replay") != "true" {
		t.Fatal("request was not replayed")
	}
	payload["cost"] = 2
	changed, _ := json.Marshal(payload)
	conflict, body := send(changed)
	if conflict.StatusCode != http.StatusConflict {
		t.Fatalf("conflict=%d %s", conflict.StatusCode, body)
	}
}

func TestRequestIDIsReturnedAndLoggedAtTheBoundary(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cfg := platform.DefaultConfig()
	runtime, err := NewRuntime(ctx, cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	runtime.Server.Handler.ServeHTTP(response, request)
	if response.Header().Get("X-Request-ID") == "" {
		t.Fatal("missing generated request ID")
	}
	provided := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	provided.Header.Set("X-Request-ID", "req-known")
	providedResponse := httptest.NewRecorder()
	runtime.Server.Handler.ServeHTTP(providedResponse, provided)
	if providedResponse.Header().Get("X-Request-ID") != "req-known" {
		t.Fatalf("request ID was not preserved: %q", providedResponse.Header().Get("X-Request-ID"))
	}
}

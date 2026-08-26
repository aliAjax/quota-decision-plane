CREATE TABLE quota_versions (version BIGINT PRIMARY KEY, published_at TIMESTAMPTZ NOT NULL, note TEXT NOT NULL DEFAULT '', document JSONB NOT NULL);
CREATE TABLE reservations (id TEXT PRIMARY KEY, tenant_id TEXT NOT NULL, resource TEXT NOT NULL, quota_id TEXT NOT NULL, counter_key TEXT NOT NULL, cost BIGINT NOT NULL CHECK (cost > 0), status TEXT NOT NULL CHECK (status IN ('pending','committed','cancelled','expired')), fencing_epoch BIGINT NOT NULL, expires_at TIMESTAMPTZ NOT NULL, created_at TIMESTAMPTZ NOT NULL, updated_at TIMESTAMPTZ NOT NULL);
CREATE INDEX reservations_pending_expiry_idx ON reservations(expires_at) WHERE status = 'pending';
CREATE TABLE shard_leases (shard BIGINT PRIMARY KEY, owner TEXT NOT NULL, fencing_epoch BIGINT NOT NULL, expires_at TIMESTAMPTZ NOT NULL);
CREATE TABLE idempotency_results (scope_key TEXT PRIMARY KEY, request_digest TEXT NOT NULL, status_code INTEGER NOT NULL, response_body JSONB NOT NULL, expires_at TIMESTAMPTZ NOT NULL);
CREATE INDEX idempotency_expiry_idx ON idempotency_results(expires_at);
CREATE TABLE audit_events (id TEXT PRIMARY KEY, event_type TEXT NOT NULL, tenant_id TEXT, resource TEXT, subject_id TEXT, outcome TEXT NOT NULL, request_id TEXT, metadata JSONB NOT NULL DEFAULT '{}', created_at TIMESTAMPTZ NOT NULL);
CREATE INDEX audit_events_tenant_time_idx ON audit_events(tenant_id, created_at DESC);


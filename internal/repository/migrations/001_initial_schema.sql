-- +goose Up

-- ============================================================================
-- Enum Types
-- ============================================================================

CREATE TYPE execution_status AS ENUM (
    'queued',
    'running',
    'paused',
    'completed',
    'failed',
    'cancelled',
    'timed_out'
);

CREATE TYPE step_status AS ENUM (
    'pending',
    'running',
    'completed',
    'failed',
    'skipped',
    'cancelled',
    'waiting_approval',
    'retrying'
);

-- ============================================================================
-- Tables
-- ============================================================================

-- 1. Tenants
CREATE TABLE tenants (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    slug       TEXT NOT NULL UNIQUE,
    name       TEXT NOT NULL,
    config     JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 2. Users
CREATE TABLE users (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email         TEXT NOT NULL UNIQUE,
    name          TEXT NOT NULL,
    idp_subject   TEXT,
    idp_issuer    TEXT,
    password_hash TEXT,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 3. Tenant Memberships
CREATE TABLE tenant_memberships (
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    tenant_id  UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    role       TEXT NOT NULL DEFAULT 'viewer',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, tenant_id)
);

-- 4. Roles
CREATE TABLE roles (
    id        UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID REFERENCES tenants(id) ON DELETE CASCADE,
    name      TEXT NOT NULL,
    level     TEXT NOT NULL CHECK (level IN ('system', 'tenant', 'workflow', 'node', 'operation')),
    UNIQUE (tenant_id, name)
);

-- 5. Permissions
CREATE TABLE permissions (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    role_id    UUID NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    resource   TEXT NOT NULL,
    action     TEXT NOT NULL,
    conditions JSONB,
    UNIQUE (role_id, resource, action)
);

-- 6. API Keys
CREATE TABLE api_keys (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    tenant_id  UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    key_hash   TEXT NOT NULL,
    name       TEXT NOT NULL,
    scopes     TEXT[] NOT NULL DEFAULT '{}',
    expires_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 7. Workflows (active_version_id FK added after workflow_versions exists)
CREATE TABLE workflows (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id         UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    slug              TEXT NOT NULL,
    name              TEXT NOT NULL,
    description       TEXT,
    active_version_id UUID,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, slug)
);

-- 8. Workflow Versions
CREATE TABLE workflow_versions (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workflow_id    UUID NOT NULL REFERENCES workflows(id) ON DELETE CASCADE,
    version        INT NOT NULL,
    schema_version TEXT NOT NULL,
    source_format  TEXT NOT NULL CHECK (source_format IN ('yaml', 'huml')),
    source_raw     TEXT NOT NULL,
    definition     JSONB NOT NULL,
    inputs_schema  JSONB,
    checksum       TEXT NOT NULL,
    published_at   TIMESTAMPTZ,
    published_by   UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (workflow_id, version)
);

-- Add FK from workflows.active_version_id -> workflow_versions.id
ALTER TABLE workflows
    ADD CONSTRAINT fk_workflows_active_version
    FOREIGN KEY (active_version_id) REFERENCES workflow_versions(id) ON DELETE SET NULL;

-- 9. Executions
CREATE TABLE executions (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id           UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    workflow_id         UUID NOT NULL REFERENCES workflows(id) ON DELETE CASCADE,
    workflow_version_id UUID NOT NULL REFERENCES workflow_versions(id),
    status              execution_status NOT NULL DEFAULT 'queued',
    idempotency_key     TEXT,
    inputs              JSONB,
    outputs             JSONB,
    context             JSONB,
    triggered_by        UUID REFERENCES users(id) ON DELETE SET NULL,
    trigger_type        TEXT,
    started_at          TIMESTAMPTZ,
    finished_at         TIMESTAMPTZ,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    checkpoint          JSONB,
    scheduler_node      TEXT,
    lock_acquired_at    TIMESTAMPTZ,
    UNIQUE (tenant_id, idempotency_key)
);

-- 10. Execution Steps
CREATE TABLE execution_steps (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    execution_id    UUID NOT NULL REFERENCES executions(id) ON DELETE CASCADE,
    step_id         TEXT NOT NULL,
    status          step_status NOT NULL DEFAULT 'pending',
    runner_type     TEXT,
    config          JSONB,
    inputs          JSONB,
    outputs         JSONB,
    error           TEXT,
    attempt         INT NOT NULL DEFAULT 1,
    max_retries     INT NOT NULL DEFAULT 0,
    timeout_seconds INT,
    started_at      TIMESTAMPTZ,
    finished_at     TIMESTAMPTZ,
    UNIQUE (execution_id, step_id, attempt)
);

-- 11. Step Logs
CREATE TABLE step_logs (
    id           BIGSERIAL PRIMARY KEY,
    execution_id UUID NOT NULL REFERENCES executions(id) ON DELETE CASCADE,
    step_id      TEXT NOT NULL,
    stream       TEXT NOT NULL CHECK (stream IN ('stdout', 'stderr', 'system')),
    line         TEXT NOT NULL,
    timestamp    TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 12. Approvals
CREATE TABLE approvals (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    execution_id   UUID NOT NULL REFERENCES executions(id) ON DELETE CASCADE,
    step_id        TEXT NOT NULL,
    tenant_id      UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    status         TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'approved', 'rejected')),
    required_roles TEXT[] NOT NULL DEFAULT '{}',
    requested_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    responded_at   TIMESTAMPTZ,
    responded_by   UUID REFERENCES users(id) ON DELETE SET NULL,
    comment        TEXT
);

-- 13. Cron Schedules
CREATE TABLE cron_schedules (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    workflow_id UUID NOT NULL REFERENCES workflows(id) ON DELETE CASCADE,
    expression  TEXT NOT NULL,
    inputs      JSONB,
    enabled     BOOLEAN NOT NULL DEFAULT true,
    next_run_at TIMESTAMPTZ,
    last_run_at TIMESTAMPTZ,
    created_by  UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 14. Audit Logs
CREATE TABLE audit_logs (
    id         BIGSERIAL PRIMARY KEY,
    tenant_id  UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    user_id    UUID REFERENCES users(id) ON DELETE SET NULL,
    action     TEXT NOT NULL,
    resource   TEXT NOT NULL,
    details    JSONB,
    ip_address INET,
    timestamp  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 15. Secrets
CREATE TABLE secrets (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name            TEXT NOT NULL,
    encrypted_value BYTEA NOT NULL,
    scope           TEXT NOT NULL,
    scope_id        UUID,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, name, scope, scope_id)
);

-- 16. Scheduler Nodes
CREATE TABLE scheduler_nodes (
    id           TEXT PRIMARY KEY,
    heartbeat_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    started_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    metadata     JSONB
);

-- ============================================================================
-- Indexes
-- ============================================================================

CREATE INDEX idx_executions_tenant_status ON executions(tenant_id, status);
CREATE INDEX idx_executions_scheduler ON executions(scheduler_node, status) WHERE status IN ('running', 'queued');
CREATE INDEX idx_executions_idempotency ON executions(tenant_id, idempotency_key) WHERE idempotency_key IS NOT NULL;
CREATE INDEX idx_execution_steps_exec ON execution_steps(execution_id, status);
CREATE INDEX idx_step_logs_exec_step ON step_logs(execution_id, step_id, timestamp);
CREATE INDEX idx_approvals_pending ON approvals(tenant_id, status) WHERE status = 'pending';
CREATE INDEX idx_cron_next_run ON cron_schedules(next_run_at) WHERE enabled = true;
CREATE INDEX idx_audit_tenant_time ON audit_logs(tenant_id, timestamp DESC);
CREATE INDEX idx_workflows_tenant ON workflows(tenant_id);
CREATE INDEX idx_scheduler_heartbeat ON scheduler_nodes(heartbeat_at);

-- ============================================================================
-- Triggers: pg_notify
-- ============================================================================

-- Notify on new execution queued
CREATE OR REPLACE FUNCTION notify_execution_queued() RETURNS trigger AS $$
BEGIN
    PERFORM pg_notify('execution_queued', json_build_object(
        'id', NEW.id,
        'tenant_id', NEW.tenant_id,
        'workflow_id', NEW.workflow_id,
        'status', NEW.status
    )::text);
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_execution_queued
    AFTER INSERT ON executions
    FOR EACH ROW
    EXECUTE FUNCTION notify_execution_queued();

-- Notify on approval response
CREATE OR REPLACE FUNCTION notify_approval_responded() RETURNS trigger AS $$
BEGIN
    IF NEW.status != 'pending' AND OLD.status = 'pending' THEN
        PERFORM pg_notify('approval_responded', json_build_object(
            'id', NEW.id,
            'execution_id', NEW.execution_id,
            'step_id', NEW.step_id,
            'status', NEW.status,
            'responded_by', NEW.responded_by
        )::text);
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_approval_responded
    AFTER UPDATE ON approvals
    FOR EACH ROW
    EXECUTE FUNCTION notify_approval_responded();

-- Notify on new step log inserted
CREATE OR REPLACE FUNCTION notify_step_log_inserted() RETURNS trigger AS $$
BEGIN
    PERFORM pg_notify('step_log_inserted', json_build_object(
        'id', NEW.id,
        'execution_id', NEW.execution_id,
        'step_id', NEW.step_id,
        'stream', NEW.stream
    )::text);
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_step_log_inserted
    AFTER INSERT ON step_logs
    FOR EACH ROW
    EXECUTE FUNCTION notify_step_log_inserted();

-- ============================================================================
-- Seed: Default System Roles and Permissions
-- ============================================================================

-- System roles (tenant_id IS NULL = global/system-level roles)
INSERT INTO roles (id, tenant_id, name, level) VALUES
    ('00000000-0000-0000-0000-000000000001', NULL, 'system_admin',   'system'),
    ('00000000-0000-0000-0000-000000000002', NULL, 'tenant_admin',   'tenant'),
    ('00000000-0000-0000-0000-000000000003', NULL, 'workflow_admin', 'workflow'),
    ('00000000-0000-0000-0000-000000000004', NULL, 'operator',       'operation'),
    ('00000000-0000-0000-0000-000000000005', NULL, 'viewer',         'node');

-- system_admin: full access to everything
INSERT INTO permissions (role_id, resource, action) VALUES
    ('00000000-0000-0000-0000-000000000001', '*', '*');

-- tenant_admin: manage tenant resources
INSERT INTO permissions (role_id, resource, action) VALUES
    ('00000000-0000-0000-0000-000000000002', 'tenant',     '*'),
    ('00000000-0000-0000-0000-000000000002', 'workflow',   '*'),
    ('00000000-0000-0000-0000-000000000002', 'execution',  '*'),
    ('00000000-0000-0000-0000-000000000002', 'secret',     '*'),
    ('00000000-0000-0000-0000-000000000002', 'user',       '*'),
    ('00000000-0000-0000-0000-000000000002', 'role',       '*'),
    ('00000000-0000-0000-0000-000000000002', 'api_key',    '*'),
    ('00000000-0000-0000-0000-000000000002', 'audit_log',  'read'),
    ('00000000-0000-0000-0000-000000000002', 'approval',   '*'),
    ('00000000-0000-0000-0000-000000000002', 'cron',       '*');

-- workflow_admin: manage workflows, executions, and related resources
INSERT INTO permissions (role_id, resource, action) VALUES
    ('00000000-0000-0000-0000-000000000003', 'workflow',   '*'),
    ('00000000-0000-0000-0000-000000000003', 'execution',  '*'),
    ('00000000-0000-0000-0000-000000000003', 'secret',     'read'),
    ('00000000-0000-0000-0000-000000000003', 'secret',     'create'),
    ('00000000-0000-0000-0000-000000000003', 'approval',   '*'),
    ('00000000-0000-0000-0000-000000000003', 'cron',       '*');

-- operator: run and monitor executions, respond to approvals
INSERT INTO permissions (role_id, resource, action) VALUES
    ('00000000-0000-0000-0000-000000000004', 'workflow',   'read'),
    ('00000000-0000-0000-0000-000000000004', 'execution',  'read'),
    ('00000000-0000-0000-0000-000000000004', 'execution',  'create'),
    ('00000000-0000-0000-0000-000000000004', 'execution',  'cancel'),
    ('00000000-0000-0000-0000-000000000004', 'approval',   'read'),
    ('00000000-0000-0000-0000-000000000004', 'approval',   'respond'),
    ('00000000-0000-0000-0000-000000000004', 'cron',       'read');

-- viewer: read-only access
INSERT INTO permissions (role_id, resource, action) VALUES
    ('00000000-0000-0000-0000-000000000005', 'workflow',   'read'),
    ('00000000-0000-0000-0000-000000000005', 'execution',  'read'),
    ('00000000-0000-0000-0000-000000000005', 'approval',   'read'),
    ('00000000-0000-0000-0000-000000000005', 'cron',       'read'),
    ('00000000-0000-0000-0000-000000000005', 'audit_log',  'read');

-- +goose Down

-- ============================================================================
-- Drop triggers and functions
-- ============================================================================

DROP TRIGGER IF EXISTS trg_step_log_inserted ON step_logs;
DROP FUNCTION IF EXISTS notify_step_log_inserted();

DROP TRIGGER IF EXISTS trg_approval_responded ON approvals;
DROP FUNCTION IF EXISTS notify_approval_responded();

DROP TRIGGER IF EXISTS trg_execution_queued ON executions;
DROP FUNCTION IF EXISTS notify_execution_queued();

-- ============================================================================
-- Drop tables in reverse dependency order
-- ============================================================================

DROP TABLE IF EXISTS scheduler_nodes;
DROP TABLE IF EXISTS secrets;
DROP TABLE IF EXISTS audit_logs;
DROP TABLE IF EXISTS cron_schedules;
DROP TABLE IF EXISTS approvals;
DROP TABLE IF EXISTS step_logs;
DROP TABLE IF EXISTS execution_steps;
DROP TABLE IF EXISTS executions;

-- Remove FK constraint before dropping workflow_versions
ALTER TABLE workflows DROP CONSTRAINT IF EXISTS fk_workflows_active_version;

DROP TABLE IF EXISTS workflow_versions;
DROP TABLE IF EXISTS workflows;
DROP TABLE IF EXISTS api_keys;
DROP TABLE IF EXISTS permissions;
DROP TABLE IF EXISTS roles;
DROP TABLE IF EXISTS tenant_memberships;
DROP TABLE IF EXISTS users;
DROP TABLE IF EXISTS tenants;

-- ============================================================================
-- Drop enum types
-- ============================================================================

DROP TYPE IF EXISTS step_status;
DROP TYPE IF EXISTS execution_status;

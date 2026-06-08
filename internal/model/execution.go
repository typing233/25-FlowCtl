package model

import (
	"time"

	"github.com/google/uuid"
)

type ExecutionStatus string

const (
	ExecutionStatusPending         ExecutionStatus = "pending"
	ExecutionStatusQueued          ExecutionStatus = "queued"
	ExecutionStatusRunning         ExecutionStatus = "running"
	ExecutionStatusWaitingApproval ExecutionStatus = "waiting_approval"
	ExecutionStatusPaused          ExecutionStatus = "paused"
	ExecutionStatusSucceeded       ExecutionStatus = "succeeded"
	ExecutionStatusFailed          ExecutionStatus = "failed"
	ExecutionStatusCancelled       ExecutionStatus = "cancelled"
	ExecutionStatusRetrying        ExecutionStatus = "retrying"
)

type StepStatus string

const (
	StepStatusPending         StepStatus = "pending"
	StepStatusQueued          StepStatus = "queued"
	StepStatusRunning         StepStatus = "running"
	StepStatusWaitingApproval StepStatus = "waiting_approval"
	StepStatusSucceeded       StepStatus = "succeeded"
	StepStatusFailed          StepStatus = "failed"
	StepStatusSkipped         StepStatus = "skipped"
	StepStatusCancelled       StepStatus = "cancelled"
	StepStatusRetrying        StepStatus = "retrying"
)

type Execution struct {
	ID                uuid.UUID       `json:"id" db:"id"`
	TenantID          uuid.UUID       `json:"tenant_id" db:"tenant_id"`
	WorkflowID        uuid.UUID       `json:"workflow_id" db:"workflow_id"`
	WorkflowVersionID uuid.UUID       `json:"workflow_version_id" db:"workflow_version_id"`
	Status            ExecutionStatus `json:"status" db:"status"`
	IdempotencyKey    string          `json:"idempotency_key,omitempty" db:"idempotency_key"`
	Inputs            map[string]any  `json:"inputs" db:"inputs"`
	Outputs           map[string]any  `json:"outputs,omitempty" db:"outputs"`
	Context           map[string]any  `json:"context,omitempty" db:"context"`
	TriggeredBy       *uuid.UUID      `json:"triggered_by,omitempty" db:"triggered_by"`
	TriggerType       string          `json:"trigger_type" db:"trigger_type"`
	StartedAt         *time.Time      `json:"started_at,omitempty" db:"started_at"`
	FinishedAt        *time.Time      `json:"finished_at,omitempty" db:"finished_at"`
	CreatedAt         time.Time       `json:"created_at" db:"created_at"`
	Checkpoint        *Checkpoint     `json:"checkpoint,omitempty" db:"checkpoint"`
	SchedulerNode     string          `json:"scheduler_node,omitempty" db:"scheduler_node"`
	LockAcquiredAt    *time.Time      `json:"lock_acquired_at,omitempty" db:"lock_acquired_at"`
}

type Checkpoint struct {
	CompletedSteps map[string]StepResult `json:"completed_steps"`
	PendingSteps   []string              `json:"pending_steps"`
	Context        map[string]any        `json:"context"`
	ResumeAfter    string                `json:"resume_after"`
	Version        int                   `json:"version"`
}

type StepResult struct {
	StepID     string         `json:"step_id"`
	Status     StepStatus     `json:"status"`
	Outputs    map[string]any `json:"outputs,omitempty"`
	Error      string         `json:"error,omitempty"`
	FinishedAt time.Time      `json:"finished_at"`
}

type ExecutionStep struct {
	ID             uuid.UUID      `json:"id" db:"id"`
	ExecutionID    uuid.UUID      `json:"execution_id" db:"execution_id"`
	StepID         string         `json:"step_id" db:"step_id"`
	Status         StepStatus     `json:"status" db:"status"`
	RunnerType     string         `json:"runner_type" db:"runner_type"`
	Config         map[string]any `json:"config" db:"config"`
	Inputs         map[string]any `json:"inputs,omitempty" db:"inputs"`
	Outputs        map[string]any `json:"outputs,omitempty" db:"outputs"`
	Error          string         `json:"error,omitempty" db:"error"`
	Attempt        int            `json:"attempt" db:"attempt"`
	MaxRetries     int            `json:"max_retries" db:"max_retries"`
	TimeoutSeconds int            `json:"timeout_seconds" db:"timeout_seconds"`
	StartedAt      *time.Time     `json:"started_at,omitempty" db:"started_at"`
	FinishedAt     *time.Time     `json:"finished_at,omitempty" db:"finished_at"`
}

type StepLog struct {
	ID          int64     `json:"id" db:"id"`
	ExecutionID uuid.UUID `json:"execution_id" db:"execution_id"`
	StepID      string    `json:"step_id" db:"step_id"`
	Stream      string    `json:"stream" db:"stream"`
	Line        string    `json:"line" db:"line"`
	Timestamp   time.Time `json:"timestamp" db:"timestamp"`
}

type ApprovalStatus string

const (
	ApprovalStatusPending  ApprovalStatus = "pending"
	ApprovalStatusApproved ApprovalStatus = "approved"
	ApprovalStatusRejected ApprovalStatus = "rejected"
)

type Approval struct {
	ID            uuid.UUID      `json:"id" db:"id"`
	ExecutionID   uuid.UUID      `json:"execution_id" db:"execution_id"`
	StepID        string         `json:"step_id" db:"step_id"`
	TenantID      uuid.UUID      `json:"tenant_id" db:"tenant_id"`
	Status        ApprovalStatus `json:"status" db:"status"`
	RequiredRoles []string       `json:"required_roles" db:"required_roles"`
	RequestedAt   time.Time      `json:"requested_at" db:"requested_at"`
	RespondedAt   *time.Time     `json:"responded_at,omitempty" db:"responded_at"`
	RespondedBy   *uuid.UUID     `json:"responded_by,omitempty" db:"responded_by"`
	Comment       string         `json:"comment,omitempty" db:"comment"`
}

type CronSchedule struct {
	ID         uuid.UUID  `json:"id" db:"id"`
	TenantID   uuid.UUID  `json:"tenant_id" db:"tenant_id"`
	WorkflowID uuid.UUID  `json:"workflow_id" db:"workflow_id"`
	Expression string     `json:"expression" db:"expression"`
	Inputs     map[string]any `json:"inputs" db:"inputs"`
	Enabled    bool       `json:"enabled" db:"enabled"`
	NextRunAt  time.Time  `json:"next_run_at" db:"next_run_at"`
	LastRunAt  *time.Time `json:"last_run_at,omitempty" db:"last_run_at"`
	CreatedBy  *uuid.UUID `json:"created_by,omitempty" db:"created_by"`
	CreatedAt  time.Time  `json:"created_at" db:"created_at"`
}

type AuditLog struct {
	ID        int64          `json:"id" db:"id"`
	TenantID  uuid.UUID      `json:"tenant_id" db:"tenant_id"`
	UserID    *uuid.UUID     `json:"user_id,omitempty" db:"user_id"`
	Action    string         `json:"action" db:"action"`
	Resource  string         `json:"resource" db:"resource"`
	Details   map[string]any `json:"details,omitempty" db:"details"`
	IPAddress string         `json:"ip_address,omitempty" db:"ip_address"`
	Timestamp time.Time      `json:"timestamp" db:"timestamp"`
}

type Secret struct {
	ID             uuid.UUID  `json:"id" db:"id"`
	TenantID       uuid.UUID  `json:"tenant_id" db:"tenant_id"`
	Name           string     `json:"name" db:"name"`
	EncryptedValue []byte     `json:"-" db:"encrypted_value"`
	Scope          string     `json:"scope" db:"scope"`
	ScopeID        *uuid.UUID `json:"scope_id,omitempty" db:"scope_id"`
	CreatedAt      time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at" db:"updated_at"`
}

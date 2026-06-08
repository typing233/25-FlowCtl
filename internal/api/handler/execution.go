package handler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/flowctl/flowctl/internal/api/middleware"
	"github.com/flowctl/flowctl/internal/model"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ExecutionHandler handles execution lifecycle operations.
type ExecutionHandler struct {
	pool *pgxpool.Pool
}

// NewExecutionHandler creates a new ExecutionHandler.
func NewExecutionHandler(pool *pgxpool.Pool) *ExecutionHandler {
	return &ExecutionHandler{pool: pool}
}

// List returns paginated executions for the current tenant.
func (h *ExecutionHandler) List(w http.ResponseWriter, r *http.Request) {
	tenantID := middleware.GetTenantID(r.Context())
	page, limit := parsePagination(r)
	offset := (page - 1) * limit

	// Optional filters
	statusFilter := r.URL.Query().Get("status")
	workflowFilter := r.URL.Query().Get("workflow_id")

	query := `SELECT id, tenant_id, workflow_id, workflow_version_id, status, idempotency_key,
	           inputs, outputs, triggered_by, trigger_type, started_at, finished_at, created_at
	          FROM executions WHERE tenant_id = $1`
	args := []any{tenantID}
	argIdx := 2

	if statusFilter != "" {
		query += ` AND status = $` + itoa(argIdx)
		args = append(args, statusFilter)
		argIdx++
	}
	if workflowFilter != "" {
		wfID, err := parseUUID(workflowFilter)
		if err == nil {
			query += ` AND workflow_id = $` + itoa(argIdx)
			args = append(args, wfID)
			argIdx++
		}
	}

	query += ` ORDER BY created_at DESC LIMIT $` + itoa(argIdx) + ` OFFSET $` + itoa(argIdx+1)
	args = append(args, limit, offset)

	rows, err := h.pool.Query(r.Context(), query, args...)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to list executions")
		return
	}
	defer rows.Close()

	var executions []model.Execution
	for rows.Next() {
		var e model.Execution
		if err := rows.Scan(&e.ID, &e.TenantID, &e.WorkflowID, &e.WorkflowVersionID, &e.Status,
			&e.IdempotencyKey, &e.Inputs, &e.Outputs, &e.TriggeredBy, &e.TriggerType,
			&e.StartedAt, &e.FinishedAt, &e.CreatedAt); err != nil {
			respondError(w, http.StatusInternalServerError, "failed to scan execution")
			return
		}
		executions = append(executions, e)
	}
	if executions == nil {
		executions = []model.Execution{}
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"data":  executions,
		"page":  page,
		"limit": limit,
	})
}

// Get returns a single execution by ID.
func (h *ExecutionHandler) Get(w http.ResponseWriter, r *http.Request) {
	tenantID := middleware.GetTenantID(r.Context())
	executionID, err := parseUUID(chi.URLParam(r, "executionID"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid execution ID")
		return
	}

	var e model.Execution
	err = h.pool.QueryRow(r.Context(),
		`SELECT id, tenant_id, workflow_id, workflow_version_id, status, idempotency_key,
		        inputs, outputs, triggered_by, trigger_type, started_at, finished_at, created_at
		 FROM executions WHERE id = $1 AND tenant_id = $2`,
		executionID, tenantID).Scan(&e.ID, &e.TenantID, &e.WorkflowID, &e.WorkflowVersionID,
		&e.Status, &e.IdempotencyKey, &e.Inputs, &e.Outputs, &e.TriggeredBy,
		&e.TriggerType, &e.StartedAt, &e.FinishedAt, &e.CreatedAt)
	if err != nil {
		respondError(w, http.StatusNotFound, "execution not found")
		return
	}

	respondJSON(w, http.StatusOK, e)
}

// Start creates a new execution in queued status.
func (h *ExecutionHandler) Start(w http.ResponseWriter, r *http.Request) {
	tenantID := middleware.GetTenantID(r.Context())
	claims := middleware.GetClaims(r.Context())

	// workflowID can come from URL param (when under /workflows/{workflowID}/run)
	workflowIDStr := chi.URLParam(r, "workflowID")

	var req struct {
		WorkflowID     string         `json:"workflow_id"`
		Inputs         map[string]any `json:"inputs"`
		IdempotencyKey string         `json:"idempotency_key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Use URL param if present, otherwise body field
	if workflowIDStr == "" {
		workflowIDStr = req.WorkflowID
	}
	workflowID, err := parseUUID(workflowIDStr)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid workflow_id")
		return
	}

	// Check idempotency
	if req.IdempotencyKey != "" {
		var existingID uuid.UUID
		err := h.pool.QueryRow(r.Context(),
			`SELECT id FROM executions WHERE idempotency_key = $1 AND tenant_id = $2`,
			req.IdempotencyKey, tenantID).Scan(&existingID)
		if err == nil {
			respondJSON(w, http.StatusOK, map[string]any{
				"id":         existingID,
				"idempotent": true,
			})
			return
		}
	}

	// Get the active version
	var versionID uuid.UUID
	err = h.pool.QueryRow(r.Context(),
		`SELECT active_version_id FROM workflows WHERE id = $1 AND tenant_id = $2 AND active_version_id IS NOT NULL`,
		workflowID, tenantID).Scan(&versionID)
	if err != nil {
		respondError(w, http.StatusBadRequest, "workflow has no published version")
		return
	}

	executionID := uuid.New()
	now := time.Now()
	if req.Inputs == nil {
		req.Inputs = map[string]any{}
	}

	_, err = h.pool.Exec(r.Context(),
		`INSERT INTO executions (id, tenant_id, workflow_id, workflow_version_id, status, idempotency_key, inputs, triggered_by, trigger_type, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 'api', $9)`,
		executionID, tenantID, workflowID, versionID, model.ExecutionStatusQueued,
		req.IdempotencyKey, req.Inputs, claims.UserID, now)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to create execution")
		return
	}

	respondJSON(w, http.StatusCreated, map[string]any{
		"id":     executionID,
		"status": model.ExecutionStatusQueued,
	})
}

// Cancel cancels a running execution.
func (h *ExecutionHandler) Cancel(w http.ResponseWriter, r *http.Request) {
	tenantID := middleware.GetTenantID(r.Context())
	executionID, err := parseUUID(chi.URLParam(r, "executionID"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid execution ID")
		return
	}

	tag, err := h.pool.Exec(r.Context(),
		`UPDATE executions SET status = $1, finished_at = now()
		 WHERE id = $2 AND tenant_id = $3 AND status NOT IN ('succeeded', 'failed', 'cancelled')`,
		model.ExecutionStatusCancelled, executionID, tenantID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to cancel execution")
		return
	}
	if tag.RowsAffected() == 0 {
		respondError(w, http.StatusConflict, "execution cannot be cancelled")
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"status": "cancelled"})
}

// Retry creates a new execution from the same workflow version as the original.
func (h *ExecutionHandler) Retry(w http.ResponseWriter, r *http.Request) {
	tenantID := middleware.GetTenantID(r.Context())
	claims := middleware.GetClaims(r.Context())
	executionID, err := parseUUID(chi.URLParam(r, "executionID"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid execution ID")
		return
	}

	// Get original execution details
	var workflowID, versionID uuid.UUID
	var inputs map[string]any
	err = h.pool.QueryRow(r.Context(),
		`SELECT workflow_id, workflow_version_id, inputs
		 FROM executions WHERE id = $1 AND tenant_id = $2`,
		executionID, tenantID).Scan(&workflowID, &versionID, &inputs)
	if err != nil {
		respondError(w, http.StatusNotFound, "execution not found")
		return
	}

	newID := uuid.New()
	now := time.Now()
	if inputs == nil {
		inputs = map[string]any{}
	}

	_, err = h.pool.Exec(r.Context(),
		`INSERT INTO executions (id, tenant_id, workflow_id, workflow_version_id, status, inputs, triggered_by, trigger_type, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, 'retry', $8)`,
		newID, tenantID, workflowID, versionID, model.ExecutionStatusQueued,
		inputs, claims.UserID, now)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to retry execution")
		return
	}

	respondJSON(w, http.StatusCreated, map[string]any{
		"id":                    newID,
		"status":                model.ExecutionStatusQueued,
		"original_execution_id": executionID,
	})
}

// ListSteps returns steps for an execution.
func (h *ExecutionHandler) ListSteps(w http.ResponseWriter, r *http.Request) {
	tenantID := middleware.GetTenantID(r.Context())
	executionID, err := parseUUID(chi.URLParam(r, "executionID"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid execution ID")
		return
	}

	// Verify execution belongs to tenant
	var exists bool
	err = h.pool.QueryRow(r.Context(),
		`SELECT EXISTS(SELECT 1 FROM executions WHERE id = $1 AND tenant_id = $2)`,
		executionID, tenantID).Scan(&exists)
	if err != nil || !exists {
		respondError(w, http.StatusNotFound, "execution not found")
		return
	}

	rows, err := h.pool.Query(r.Context(),
		`SELECT id, execution_id, step_id, status, runner_type, config, inputs, outputs, error, attempt, max_retries, timeout_seconds, started_at, finished_at
		 FROM execution_steps WHERE execution_id = $1 ORDER BY started_at ASC NULLS LAST`,
		executionID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to list steps")
		return
	}
	defer rows.Close()

	var steps []model.ExecutionStep
	for rows.Next() {
		var s model.ExecutionStep
		if err := rows.Scan(&s.ID, &s.ExecutionID, &s.StepID, &s.Status, &s.RunnerType, &s.Config,
			&s.Inputs, &s.Outputs, &s.Error, &s.Attempt, &s.MaxRetries, &s.TimeoutSeconds,
			&s.StartedAt, &s.FinishedAt); err != nil {
			respondError(w, http.StatusInternalServerError, "failed to scan step")
			return
		}
		steps = append(steps, s)
	}
	if steps == nil {
		steps = []model.ExecutionStep{}
	}

	respondJSON(w, http.StatusOK, steps)
}

// GetLogs returns step logs for an execution.
func (h *ExecutionHandler) GetLogs(w http.ResponseWriter, r *http.Request) {
	tenantID := middleware.GetTenantID(r.Context())
	executionID, err := parseUUID(chi.URLParam(r, "executionID"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid execution ID")
		return
	}

	// Verify execution belongs to tenant
	var exists bool
	err = h.pool.QueryRow(r.Context(),
		`SELECT EXISTS(SELECT 1 FROM executions WHERE id = $1 AND tenant_id = $2)`,
		executionID, tenantID).Scan(&exists)
	if err != nil || !exists {
		respondError(w, http.StatusNotFound, "execution not found")
		return
	}

	rows, err := h.pool.Query(r.Context(),
		`SELECT id, execution_id, step_id, stream, line, timestamp
		 FROM step_logs WHERE execution_id = $1 ORDER BY timestamp ASC, id ASC`,
		executionID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to get logs")
		return
	}
	defer rows.Close()

	var logs []model.StepLog
	for rows.Next() {
		var l model.StepLog
		if err := rows.Scan(&l.ID, &l.ExecutionID, &l.StepID, &l.Stream, &l.Line, &l.Timestamp); err != nil {
			respondError(w, http.StatusInternalServerError, "failed to scan log")
			return
		}
		logs = append(logs, l)
	}
	if logs == nil {
		logs = []model.StepLog{}
	}

	respondJSON(w, http.StatusOK, logs)
}

// itoa is a simple int-to-string helper for building query placeholders.
func itoa(i int) string {
	return strconv.Itoa(i)
}

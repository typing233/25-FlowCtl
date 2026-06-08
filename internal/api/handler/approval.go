package handler

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/flowctl/flowctl/internal/api/middleware"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ApprovalHandler handles approval operations.
type ApprovalHandler struct {
	pool *pgxpool.Pool
}

// NewApprovalHandler creates a new ApprovalHandler.
func NewApprovalHandler(pool *pgxpool.Pool) *ApprovalHandler {
	return &ApprovalHandler{pool: pool}
}

// List returns pending approvals for the current tenant.
func (h *ApprovalHandler) List(w http.ResponseWriter, r *http.Request) {
	tenantID := middleware.GetTenantID(r.Context())
	page, limit := parsePagination(r)
	offset := (page - 1) * limit

	rows, err := h.pool.Query(r.Context(),
		`SELECT id, execution_id, step_id, tenant_id, status, required_roles, requested_at, responded_at, responded_by, comment
		 FROM approvals WHERE tenant_id = $1 AND status = 'pending'
		 ORDER BY requested_at DESC LIMIT $2 OFFSET $3`,
		tenantID, limit, offset)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to list approvals")
		return
	}
	defer rows.Close()

	type approvalRow struct {
		ID            uuid.UUID  `json:"id"`
		ExecutionID   uuid.UUID  `json:"execution_id"`
		StepID        string     `json:"step_id"`
		TenantID      uuid.UUID  `json:"tenant_id"`
		Status        string     `json:"status"`
		RequiredRoles []string   `json:"required_roles"`
		RequestedAt   time.Time  `json:"requested_at"`
		RespondedAt   *time.Time `json:"responded_at,omitempty"`
		RespondedBy   *uuid.UUID `json:"responded_by,omitempty"`
		Comment       string     `json:"comment,omitempty"`
	}

	var approvals []approvalRow
	for rows.Next() {
		var a approvalRow
		if err := rows.Scan(&a.ID, &a.ExecutionID, &a.StepID, &a.TenantID, &a.Status,
			&a.RequiredRoles, &a.RequestedAt, &a.RespondedAt, &a.RespondedBy, &a.Comment); err != nil {
			respondError(w, http.StatusInternalServerError, "failed to scan approval")
			return
		}
		approvals = append(approvals, a)
	}
	if approvals == nil {
		approvals = []approvalRow{}
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"data":  approvals,
		"page":  page,
		"limit": limit,
	})
}

// Approve approves a pending approval and fires pg_notify.
func (h *ApprovalHandler) Approve(w http.ResponseWriter, r *http.Request) {
	tenantID := middleware.GetTenantID(r.Context())
	claims := middleware.GetClaims(r.Context())
	approvalID, err := parseUUID(chi.URLParam(r, "approvalID"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid approval ID")
		return
	}

	var req struct {
		Comment string `json:"comment"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	now := time.Now()
	tag, err := h.pool.Exec(r.Context(),
		`UPDATE approvals SET status = 'approved', responded_at = $1, responded_by = $2, comment = $3
		 WHERE id = $4 AND tenant_id = $5 AND status = 'pending'`,
		now, claims.UserID, req.Comment, approvalID, tenantID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to approve")
		return
	}
	if tag.RowsAffected() == 0 {
		respondError(w, http.StatusConflict, "approval not found or already resolved")
		return
	}

	// Fire pg_notify to unblock the execution - channel must match scheduler's LISTEN
	var executionID uuid.UUID
	_ = h.pool.QueryRow(r.Context(),
		`SELECT execution_id FROM approvals WHERE id = $1`, approvalID).Scan(&executionID)
	_, _ = h.pool.Exec(r.Context(),
		`SELECT pg_notify('approval_responded', $1::text)`, executionID.String())

	respondJSON(w, http.StatusOK, map[string]string{"status": "approved"})
}

// Reject rejects a pending approval and fires pg_notify.
func (h *ApprovalHandler) Reject(w http.ResponseWriter, r *http.Request) {
	tenantID := middleware.GetTenantID(r.Context())
	claims := middleware.GetClaims(r.Context())
	approvalID, err := parseUUID(chi.URLParam(r, "approvalID"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid approval ID")
		return
	}

	var req struct {
		Comment string `json:"comment"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	now := time.Now()
	tag, err := h.pool.Exec(r.Context(),
		`UPDATE approvals SET status = 'rejected', responded_at = $1, responded_by = $2, comment = $3
		 WHERE id = $4 AND tenant_id = $5 AND status = 'pending'`,
		now, claims.UserID, req.Comment, approvalID, tenantID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to reject")
		return
	}
	if tag.RowsAffected() == 0 {
		respondError(w, http.StatusConflict, "approval not found or already resolved")
		return
	}

	// Fire pg_notify to unblock the execution - channel must match scheduler's LISTEN
	var executionID uuid.UUID
	_ = h.pool.QueryRow(r.Context(),
		`SELECT execution_id FROM approvals WHERE id = $1`, approvalID).Scan(&executionID)
	_, _ = h.pool.Exec(r.Context(),
		`SELECT pg_notify('approval_responded', $1::text)`, executionID.String())

	respondJSON(w, http.StatusOK, map[string]string{"status": "rejected"})
}

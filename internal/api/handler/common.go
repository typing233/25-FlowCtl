package handler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/flowctl/flowctl/internal/api/middleware"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func respondJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if data != nil {
		json.NewEncoder(w).Encode(data)
	}
}

func respondError(w http.ResponseWriter, status int, message string) {
	respondJSON(w, status, map[string]string{"error": message})
}

func parsePagination(r *http.Request) (page, limit int) {
	page = 1
	limit = 20

	if p := r.URL.Query().Get("page"); p != "" {
		if v, err := strconv.Atoi(p); err == nil && v > 0 {
			page = v
		}
	}
	if l := r.URL.Query().Get("limit"); l != "" {
		if v, err := strconv.Atoi(l); err == nil && v > 0 && v <= 100 {
			limit = v
		}
	}
	return page, limit
}

func parseUUID(s string) (uuid.UUID, error) {
	return uuid.Parse(s)
}

// ListSecrets returns an http.HandlerFunc that lists secrets for the current tenant.
func ListSecrets(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID := middleware.GetTenantID(r.Context())
		page, limit := parsePagination(r)
		offset := (page - 1) * limit

		rows, err := pool.Query(r.Context(),
			`SELECT id, tenant_id, name, scope, scope_id, created_at
			 FROM secrets WHERE tenant_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3`,
			tenantID, limit, offset)
		if err != nil {
			respondError(w, http.StatusInternalServerError, "failed to list secrets")
			return
		}
		defer rows.Close()

		type secretRow struct {
			ID        uuid.UUID  `json:"id"`
			TenantID  uuid.UUID  `json:"tenant_id"`
			Name      string     `json:"name"`
			Scope     string     `json:"scope"`
			ScopeID   *uuid.UUID `json:"scope_id,omitempty"`
			CreatedAt time.Time  `json:"created_at"`
		}

		var secrets []secretRow
		for rows.Next() {
			var s secretRow
			if err := rows.Scan(&s.ID, &s.TenantID, &s.Name, &s.Scope, &s.ScopeID, &s.CreatedAt); err != nil {
				respondError(w, http.StatusInternalServerError, "failed to scan secret")
				return
			}
			secrets = append(secrets, s)
		}
		if secrets == nil {
			secrets = []secretRow{}
		}
		respondJSON(w, http.StatusOK, secrets)
	}
}

// CreateSecret returns an http.HandlerFunc that creates a new secret.
func CreateSecret(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID := middleware.GetTenantID(r.Context())

		var req struct {
			Name    string     `json:"name"`
			Value   string     `json:"value"`
			Scope   string     `json:"scope"`
			ScopeID *uuid.UUID `json:"scope_id,omitempty"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			respondError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		if req.Name == "" || req.Value == "" {
			respondError(w, http.StatusBadRequest, "name and value are required")
			return
		}

		id := uuid.New()
		_, err := pool.Exec(r.Context(),
			`INSERT INTO secrets (id, tenant_id, name, encrypted_value, scope, scope_id, created_at)
			 VALUES ($1, $2, $3, $4, $5, $6, now())`,
			id, tenantID, req.Name, []byte(req.Value), req.Scope, req.ScopeID)
		if err != nil {
			respondError(w, http.StatusInternalServerError, "failed to create secret")
			return
		}

		respondJSON(w, http.StatusCreated, map[string]any{
			"id":   id,
			"name": req.Name,
		})
	}
}

// DeleteSecret returns an http.HandlerFunc that deletes a secret by ID.
func DeleteSecret(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID := middleware.GetTenantID(r.Context())
		secretID, err := parseUUID(chi.URLParam(r, "secretID"))
		if err != nil {
			respondError(w, http.StatusBadRequest, "invalid secret ID")
			return
		}

		tag, err := pool.Exec(r.Context(),
			`DELETE FROM secrets WHERE id = $1 AND tenant_id = $2`,
			secretID, tenantID)
		if err != nil {
			respondError(w, http.StatusInternalServerError, "failed to delete secret")
			return
		}
		if tag.RowsAffected() == 0 {
			respondError(w, http.StatusNotFound, "secret not found")
			return
		}
		respondJSON(w, http.StatusNoContent, nil)
	}
}

// ListCronSchedules returns an http.HandlerFunc that lists cron schedules.
func ListCronSchedules(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID := middleware.GetTenantID(r.Context())
		page, limit := parsePagination(r)
		offset := (page - 1) * limit

		rows, err := pool.Query(r.Context(),
			`SELECT id, tenant_id, workflow_id, expression, inputs, enabled, next_run_at, last_run_at, created_by, created_at
			 FROM cron_schedules WHERE tenant_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3`,
			tenantID, limit, offset)
		if err != nil {
			respondError(w, http.StatusInternalServerError, "failed to list cron schedules")
			return
		}
		defer rows.Close()

		type cronRow struct {
			ID         uuid.UUID      `json:"id"`
			TenantID   uuid.UUID      `json:"tenant_id"`
			WorkflowID uuid.UUID      `json:"workflow_id"`
			Expression string         `json:"expression"`
			Inputs     map[string]any `json:"inputs"`
			Enabled    bool           `json:"enabled"`
			NextRunAt  time.Time      `json:"next_run_at"`
			LastRunAt  *time.Time     `json:"last_run_at,omitempty"`
			CreatedBy  *uuid.UUID     `json:"created_by,omitempty"`
			CreatedAt  time.Time      `json:"created_at"`
		}

		var schedules []cronRow
		for rows.Next() {
			var c cronRow
			if err := rows.Scan(&c.ID, &c.TenantID, &c.WorkflowID, &c.Expression, &c.Inputs, &c.Enabled, &c.NextRunAt, &c.LastRunAt, &c.CreatedBy, &c.CreatedAt); err != nil {
				respondError(w, http.StatusInternalServerError, "failed to scan cron schedule")
				return
			}
			schedules = append(schedules, c)
		}
		if schedules == nil {
			schedules = []cronRow{}
		}
		respondJSON(w, http.StatusOK, schedules)
	}
}

// CreateCronSchedule returns an http.HandlerFunc that creates a cron schedule.
func CreateCronSchedule(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID := middleware.GetTenantID(r.Context())
		claims := middleware.GetClaims(r.Context())

		var req struct {
			WorkflowID uuid.UUID      `json:"workflow_id"`
			Expression string         `json:"expression"`
			Inputs     map[string]any `json:"inputs"`
			Enabled    bool           `json:"enabled"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			respondError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		if req.Expression == "" {
			respondError(w, http.StatusBadRequest, "expression is required")
			return
		}

		id := uuid.New()
		_, err := pool.Exec(r.Context(),
			`INSERT INTO cron_schedules (id, tenant_id, workflow_id, expression, inputs, enabled, next_run_at, created_by, created_at)
			 VALUES ($1, $2, $3, $4, $5, $6, now(), $7, now())`,
			id, tenantID, req.WorkflowID, req.Expression, req.Inputs, req.Enabled, claims.UserID)
		if err != nil {
			respondError(w, http.StatusInternalServerError, "failed to create cron schedule")
			return
		}

		respondJSON(w, http.StatusCreated, map[string]any{"id": id})
	}
}

// UpdateCronSchedule returns an http.HandlerFunc that updates a cron schedule.
func UpdateCronSchedule(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID := middleware.GetTenantID(r.Context())
		cronID, err := parseUUID(chi.URLParam(r, "cronID"))
		if err != nil {
			respondError(w, http.StatusBadRequest, "invalid cron ID")
			return
		}

		var req struct {
			Expression string         `json:"expression"`
			Inputs     map[string]any `json:"inputs"`
			Enabled    *bool          `json:"enabled"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			respondError(w, http.StatusBadRequest, "invalid request body")
			return
		}

		tag, err := pool.Exec(r.Context(),
			`UPDATE cron_schedules SET expression = COALESCE(NULLIF($1, ''), expression),
			 inputs = COALESCE($2, inputs), enabled = COALESCE($3, enabled)
			 WHERE id = $4 AND tenant_id = $5`,
			req.Expression, req.Inputs, req.Enabled, cronID, tenantID)
		if err != nil {
			respondError(w, http.StatusInternalServerError, "failed to update cron schedule")
			return
		}
		if tag.RowsAffected() == 0 {
			respondError(w, http.StatusNotFound, "cron schedule not found")
			return
		}
		respondJSON(w, http.StatusOK, map[string]string{"status": "updated"})
	}
}

// DeleteCronSchedule returns an http.HandlerFunc that deletes a cron schedule.
func DeleteCronSchedule(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID := middleware.GetTenantID(r.Context())
		cronID, err := parseUUID(chi.URLParam(r, "cronID"))
		if err != nil {
			respondError(w, http.StatusBadRequest, "invalid cron ID")
			return
		}

		tag, err := pool.Exec(r.Context(),
			`DELETE FROM cron_schedules WHERE id = $1 AND tenant_id = $2`,
			cronID, tenantID)
		if err != nil {
			respondError(w, http.StatusInternalServerError, "failed to delete cron schedule")
			return
		}
		if tag.RowsAffected() == 0 {
			respondError(w, http.StatusNotFound, "cron schedule not found")
			return
		}
		respondJSON(w, http.StatusNoContent, nil)
	}
}

// ListAuditLogs returns an http.HandlerFunc that lists audit logs.
func ListAuditLogs(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID := middleware.GetTenantID(r.Context())
		page, limit := parsePagination(r)
		offset := (page - 1) * limit

		rows, err := pool.Query(r.Context(),
			`SELECT id, tenant_id, user_id, action, resource, details, ip_address, timestamp
			 FROM audit_logs WHERE tenant_id = $1 ORDER BY timestamp DESC LIMIT $2 OFFSET $3`,
			tenantID, limit, offset)
		if err != nil {
			respondError(w, http.StatusInternalServerError, "failed to list audit logs")
			return
		}
		defer rows.Close()

		type auditRow struct {
			ID        int64          `json:"id"`
			TenantID  uuid.UUID      `json:"tenant_id"`
			UserID    *uuid.UUID     `json:"user_id,omitempty"`
			Action    string         `json:"action"`
			Resource  string         `json:"resource"`
			Details   map[string]any `json:"details,omitempty"`
			IPAddress string         `json:"ip_address,omitempty"`
			Timestamp time.Time      `json:"timestamp"`
		}

		var logs []auditRow
		for rows.Next() {
			var l auditRow
			if err := rows.Scan(&l.ID, &l.TenantID, &l.UserID, &l.Action, &l.Resource, &l.Details, &l.IPAddress, &l.Timestamp); err != nil {
				respondError(w, http.StatusInternalServerError, "failed to scan audit log")
				return
			}
			logs = append(logs, l)
		}
		if logs == nil {
			logs = []auditRow{}
		}
		respondJSON(w, http.StatusOK, logs)
	}
}

// ListUsers returns an http.HandlerFunc that lists users in the tenant.
func ListUsers(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID := middleware.GetTenantID(r.Context())
		page, limit := parsePagination(r)
		offset := (page - 1) * limit

		rows, err := pool.Query(r.Context(),
			`SELECT u.id, u.email, u.name, tm.role, u.created_at
			 FROM users u
			 JOIN tenant_memberships tm ON tm.user_id = u.id
			 WHERE tm.tenant_id = $1
			 ORDER BY u.created_at DESC LIMIT $2 OFFSET $3`,
			tenantID, limit, offset)
		if err != nil {
			respondError(w, http.StatusInternalServerError, "failed to list users")
			return
		}
		defer rows.Close()

		type userRow struct {
			ID        uuid.UUID `json:"id"`
			Email     string    `json:"email"`
			Name      string    `json:"name"`
			Role      string    `json:"role"`
			CreatedAt time.Time `json:"created_at"`
		}

		var users []userRow
		for rows.Next() {
			var u userRow
			if err := rows.Scan(&u.ID, &u.Email, &u.Name, &u.Role, &u.CreatedAt); err != nil {
				respondError(w, http.StatusInternalServerError, "failed to scan user")
				return
			}
			users = append(users, u)
		}
		if users == nil {
			users = []userRow{}
		}
		respondJSON(w, http.StatusOK, users)
	}
}

// ListRoles returns an http.HandlerFunc that lists roles.
func ListRoles(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID := middleware.GetTenantID(r.Context())

		rows, err := pool.Query(r.Context(),
			`SELECT id, tenant_id, name, permissions, created_at
			 FROM roles WHERE tenant_id = $1 ORDER BY name`,
			tenantID)
		if err != nil {
			respondError(w, http.StatusInternalServerError, "failed to list roles")
			return
		}
		defer rows.Close()

		type roleRow struct {
			ID          uuid.UUID `json:"id"`
			TenantID    uuid.UUID `json:"tenant_id"`
			Name        string    `json:"name"`
			Permissions []string  `json:"permissions"`
			CreatedAt   time.Time `json:"created_at"`
		}

		var roles []roleRow
		for rows.Next() {
			var role roleRow
			if err := rows.Scan(&role.ID, &role.TenantID, &role.Name, &role.Permissions, &role.CreatedAt); err != nil {
				respondError(w, http.StatusInternalServerError, "failed to scan role")
				return
			}
			roles = append(roles, role)
		}
		if roles == nil {
			roles = []roleRow{}
		}
		respondJSON(w, http.StatusOK, roles)
	}
}

// CreateRole returns an http.HandlerFunc that creates a new role.
func CreateRole(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID := middleware.GetTenantID(r.Context())

		var req struct {
			Name        string   `json:"name"`
			Permissions []string `json:"permissions"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			respondError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		if req.Name == "" {
			respondError(w, http.StatusBadRequest, "name is required")
			return
		}

		id := uuid.New()
		_, err := pool.Exec(r.Context(),
			`INSERT INTO roles (id, tenant_id, name, permissions, created_at)
			 VALUES ($1, $2, $3, $4, now())`,
			id, tenantID, req.Name, req.Permissions)
		if err != nil {
			respondError(w, http.StatusInternalServerError, "failed to create role")
			return
		}

		respondJSON(w, http.StatusCreated, map[string]any{
			"id":   id,
			"name": req.Name,
		})
	}
}

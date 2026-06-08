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

// TenantHandler handles tenant CRUD and membership operations.
type TenantHandler struct {
	pool *pgxpool.Pool
}

// NewTenantHandler creates a new TenantHandler.
func NewTenantHandler(pool *pgxpool.Pool) *TenantHandler {
	return &TenantHandler{pool: pool}
}

// List returns all tenants the current user belongs to.
func (h *TenantHandler) List(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())

	rows, err := h.pool.Query(r.Context(),
		`SELECT t.id, t.name, t.slug, tm.role, t.created_at
		 FROM tenants t
		 JOIN tenant_memberships tm ON tm.tenant_id = t.id
		 WHERE tm.user_id = $1
		 ORDER BY t.name`,
		claims.UserID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to list tenants")
		return
	}
	defer rows.Close()

	type tenantRow struct {
		ID        uuid.UUID `json:"id"`
		Name      string    `json:"name"`
		Slug      string    `json:"slug"`
		Role      string    `json:"role"`
		CreatedAt time.Time `json:"created_at"`
	}

	var tenants []tenantRow
	for rows.Next() {
		var t tenantRow
		if err := rows.Scan(&t.ID, &t.Name, &t.Slug, &t.Role, &t.CreatedAt); err != nil {
			respondError(w, http.StatusInternalServerError, "failed to scan tenant")
			return
		}
		tenants = append(tenants, t)
	}
	if tenants == nil {
		tenants = []tenantRow{}
	}

	respondJSON(w, http.StatusOK, tenants)
}

// Get returns a single tenant by ID.
func (h *TenantHandler) Get(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	tenantID, err := parseUUID(chi.URLParam(r, "tenantID"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid tenant ID")
		return
	}

	type tenantDetail struct {
		ID        uuid.UUID `json:"id"`
		Name      string    `json:"name"`
		Slug      string    `json:"slug"`
		Role      string    `json:"role"`
		CreatedAt time.Time `json:"created_at"`
		UpdatedAt time.Time `json:"updated_at"`
	}

	var t tenantDetail
	err = h.pool.QueryRow(r.Context(),
		`SELECT t.id, t.name, t.slug, tm.role, t.created_at, t.updated_at
		 FROM tenants t
		 JOIN tenant_memberships tm ON tm.tenant_id = t.id
		 WHERE t.id = $1 AND tm.user_id = $2`,
		tenantID, claims.UserID).Scan(&t.ID, &t.Name, &t.Slug, &t.Role, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		respondError(w, http.StatusNotFound, "tenant not found")
		return
	}

	respondJSON(w, http.StatusOK, t)
}

// Create creates a new tenant and adds the current user as owner.
func (h *TenantHandler) Create(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())

	var req struct {
		Name string `json:"name"`
		Slug string `json:"slug"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" || req.Slug == "" {
		respondError(w, http.StatusBadRequest, "name and slug are required")
		return
	}

	tenantID := uuid.New()
	now := time.Now()

	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to begin transaction")
		return
	}
	defer tx.Rollback(r.Context())

	_, err = tx.Exec(r.Context(),
		`INSERT INTO tenants (id, name, slug, created_at, updated_at) VALUES ($1, $2, $3, $4, $5)`,
		tenantID, req.Name, req.Slug, now, now)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to create tenant")
		return
	}

	_, err = tx.Exec(r.Context(),
		`INSERT INTO tenant_memberships (user_id, tenant_id, role, joined_at) VALUES ($1, $2, 'owner', $3)`,
		claims.UserID, tenantID, now)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to add membership")
		return
	}

	if err := tx.Commit(r.Context()); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to commit transaction")
		return
	}

	respondJSON(w, http.StatusCreated, map[string]any{
		"id":   tenantID,
		"name": req.Name,
		"slug": req.Slug,
	})
}

// Update updates a tenant's name.
func (h *TenantHandler) Update(w http.ResponseWriter, r *http.Request) {
	tenantID, err := parseUUID(chi.URLParam(r, "tenantID"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid tenant ID")
		return
	}

	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	tag, err := h.pool.Exec(r.Context(),
		`UPDATE tenants SET name = COALESCE(NULLIF($1, ''), name), updated_at = now() WHERE id = $2`,
		req.Name, tenantID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to update tenant")
		return
	}
	if tag.RowsAffected() == 0 {
		respondError(w, http.StatusNotFound, "tenant not found")
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

// ListMembers lists members of a tenant.
func (h *TenantHandler) ListMembers(w http.ResponseWriter, r *http.Request) {
	tenantID, err := parseUUID(chi.URLParam(r, "tenantID"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid tenant ID")
		return
	}

	rows, err := h.pool.Query(r.Context(),
		`SELECT u.id, u.email, u.name, tm.role, tm.joined_at
		 FROM users u
		 JOIN tenant_memberships tm ON tm.user_id = u.id
		 WHERE tm.tenant_id = $1
		 ORDER BY tm.joined_at`,
		tenantID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to list members")
		return
	}
	defer rows.Close()

	type memberRow struct {
		ID       uuid.UUID `json:"id"`
		Email    string    `json:"email"`
		Name     string    `json:"name"`
		Role     string    `json:"role"`
		JoinedAt time.Time `json:"joined_at"`
	}

	var members []memberRow
	for rows.Next() {
		var m memberRow
		if err := rows.Scan(&m.ID, &m.Email, &m.Name, &m.Role, &m.JoinedAt); err != nil {
			respondError(w, http.StatusInternalServerError, "failed to scan member")
			return
		}
		members = append(members, m)
	}
	if members == nil {
		members = []memberRow{}
	}

	respondJSON(w, http.StatusOK, members)
}

// AddMember adds a user to a tenant with a specified role.
func (h *TenantHandler) AddMember(w http.ResponseWriter, r *http.Request) {
	tenantID, err := parseUUID(chi.URLParam(r, "tenantID"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid tenant ID")
		return
	}

	var req struct {
		Email string `json:"email"`
		Role  string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Email == "" || req.Role == "" {
		respondError(w, http.StatusBadRequest, "email and role are required")
		return
	}

	// Find user by email
	var userID uuid.UUID
	err = h.pool.QueryRow(r.Context(),
		`SELECT id FROM users WHERE email = $1`, req.Email).Scan(&userID)
	if err != nil {
		respondError(w, http.StatusNotFound, "user not found")
		return
	}

	now := time.Now()
	_, err = h.pool.Exec(r.Context(),
		`INSERT INTO tenant_memberships (user_id, tenant_id, role, joined_at)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT (user_id, tenant_id) DO UPDATE SET role = EXCLUDED.role`,
		userID, tenantID, req.Role, now)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to add member")
		return
	}

	respondJSON(w, http.StatusCreated, map[string]any{
		"user_id":   userID,
		"tenant_id": tenantID,
		"role":      req.Role,
	})
}

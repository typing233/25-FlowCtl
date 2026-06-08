package handler

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"time"

	"github.com/flowctl/flowctl/internal/api/middleware"
	"github.com/flowctl/flowctl/internal/huml"
	"github.com/flowctl/flowctl/internal/model"
	"github.com/flowctl/flowctl/internal/validator"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"gopkg.in/yaml.v3"
)

// WorkflowHandler handles workflow CRUD and versioning.
type WorkflowHandler struct {
	pool *pgxpool.Pool
}

// NewWorkflowHandler creates a new WorkflowHandler.
func NewWorkflowHandler(pool *pgxpool.Pool) *WorkflowHandler {
	return &WorkflowHandler{pool: pool}
}

// List returns paginated workflows for the current tenant.
func (h *WorkflowHandler) List(w http.ResponseWriter, r *http.Request) {
	tenantID := middleware.GetTenantID(r.Context())
	page, limit := parsePagination(r)
	offset := (page - 1) * limit

	rows, err := h.pool.Query(r.Context(),
		`SELECT id, tenant_id, slug, name, description, active_version_id, created_at, updated_at
		 FROM workflows WHERE tenant_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3`,
		tenantID, limit, offset)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to list workflows")
		return
	}
	defer rows.Close()

	var workflows []model.Workflow
	for rows.Next() {
		var wf model.Workflow
		if err := rows.Scan(&wf.ID, &wf.TenantID, &wf.Slug, &wf.Name, &wf.Description, &wf.ActiveVersionID, &wf.CreatedAt, &wf.UpdatedAt); err != nil {
			respondError(w, http.StatusInternalServerError, "failed to scan workflow")
			return
		}
		workflows = append(workflows, wf)
	}
	if workflows == nil {
		workflows = []model.Workflow{}
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"data":  workflows,
		"page":  page,
		"limit": limit,
	})
}

// Get returns a single workflow by ID.
func (h *WorkflowHandler) Get(w http.ResponseWriter, r *http.Request) {
	tenantID := middleware.GetTenantID(r.Context())
	workflowID, err := parseUUID(chi.URLParam(r, "workflowID"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid workflow ID")
		return
	}

	var wf model.Workflow
	err = h.pool.QueryRow(r.Context(),
		`SELECT id, tenant_id, slug, name, description, active_version_id, created_at, updated_at
		 FROM workflows WHERE id = $1 AND tenant_id = $2`,
		workflowID, tenantID).Scan(&wf.ID, &wf.TenantID, &wf.Slug, &wf.Name, &wf.Description, &wf.ActiveVersionID, &wf.CreatedAt, &wf.UpdatedAt)
	if err != nil {
		respondError(w, http.StatusNotFound, "workflow not found")
		return
	}

	respondJSON(w, http.StatusOK, wf)
}

// Create creates a new workflow.
func (h *WorkflowHandler) Create(w http.ResponseWriter, r *http.Request) {
	tenantID := middleware.GetTenantID(r.Context())

	var req struct {
		Slug        string `json:"slug"`
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Slug == "" || req.Name == "" {
		respondError(w, http.StatusBadRequest, "slug and name are required")
		return
	}

	id := uuid.New()
	now := time.Now()
	_, err := h.pool.Exec(r.Context(),
		`INSERT INTO workflows (id, tenant_id, slug, name, description, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		id, tenantID, req.Slug, req.Name, req.Description, now, now)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to create workflow")
		return
	}

	respondJSON(w, http.StatusCreated, model.Workflow{
		ID:          id,
		TenantID:    tenantID,
		Slug:        req.Slug,
		Name:        req.Name,
		Description: req.Description,
		CreatedAt:   now,
		UpdatedAt:   now,
	})
}

// Update updates a workflow's metadata.
func (h *WorkflowHandler) Update(w http.ResponseWriter, r *http.Request) {
	tenantID := middleware.GetTenantID(r.Context())
	workflowID, err := parseUUID(chi.URLParam(r, "workflowID"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid workflow ID")
		return
	}

	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	tag, err := h.pool.Exec(r.Context(),
		`UPDATE workflows SET name = COALESCE(NULLIF($1, ''), name),
		 description = COALESCE(NULLIF($2, ''), description), updated_at = now()
		 WHERE id = $3 AND tenant_id = $4`,
		req.Name, req.Description, workflowID, tenantID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to update workflow")
		return
	}
	if tag.RowsAffected() == 0 {
		respondError(w, http.StatusNotFound, "workflow not found")
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

// Delete deletes a workflow.
func (h *WorkflowHandler) Delete(w http.ResponseWriter, r *http.Request) {
	tenantID := middleware.GetTenantID(r.Context())
	workflowID, err := parseUUID(chi.URLParam(r, "workflowID"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid workflow ID")
		return
	}

	tag, err := h.pool.Exec(r.Context(),
		`DELETE FROM workflows WHERE id = $1 AND tenant_id = $2`,
		workflowID, tenantID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to delete workflow")
		return
	}
	if tag.RowsAffected() == 0 {
		respondError(w, http.StatusNotFound, "workflow not found")
		return
	}

	respondJSON(w, http.StatusNoContent, nil)
}

// Publish accepts source (yaml or huml), parses, validates, and creates a new version.
func (h *WorkflowHandler) Publish(w http.ResponseWriter, r *http.Request) {
	tenantID := middleware.GetTenantID(r.Context())
	workflowID, err := parseUUID(chi.URLParam(r, "workflowID"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid workflow ID")
		return
	}

	var req struct {
		Source string `json:"source"`
		Format string `json:"format"` // "yaml" or "huml"
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Source == "" || req.Format == "" {
		respondError(w, http.StatusBadRequest, "source and format are required")
		return
	}

	// Parse the workflow definition
	var def *model.WorkflowDefinition
	switch model.SourceFormat(req.Format) {
	case model.SourceFormatYAML:
		def = &model.WorkflowDefinition{}
		if err := yaml.Unmarshal([]byte(req.Source), def); err != nil {
			respondError(w, http.StatusBadRequest, "failed to parse YAML: "+err.Error())
			return
		}
	case model.SourceFormatHUML:
		def, err = huml.ParseHUML(req.Source)
		if err != nil {
			respondError(w, http.StatusBadRequest, "failed to parse HUML: "+err.Error())
			return
		}
	default:
		respondError(w, http.StatusBadRequest, "format must be 'yaml' or 'huml'")
		return
	}

	// Validate DAG
	result := validator.ValidateWorkflow(def)
	if !result.Valid {
		respondJSON(w, http.StatusUnprocessableEntity, map[string]any{
			"error":  "workflow validation failed",
			"errors": result.Errors,
		})
		return
	}

	// Verify workflow belongs to tenant
	var exists bool
	err = h.pool.QueryRow(r.Context(),
		`SELECT EXISTS(SELECT 1 FROM workflows WHERE id = $1 AND tenant_id = $2)`,
		workflowID, tenantID).Scan(&exists)
	if err != nil || !exists {
		respondError(w, http.StatusNotFound, "workflow not found")
		return
	}

	// Get next version number
	var currentVersion int
	_ = h.pool.QueryRow(r.Context(),
		`SELECT COALESCE(MAX(version), 0) FROM workflow_versions WHERE workflow_id = $1`,
		workflowID).Scan(&currentVersion)
	nextVersion := currentVersion + 1

	// Convert definition to JSONB
	defJSON, err := json.Marshal(def)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to marshal definition")
		return
	}
	var defMap map[string]any
	json.Unmarshal(defJSON, &defMap)

	// Compute checksum
	hash := sha256.Sum256([]byte(req.Source))
	checksum := hex.EncodeToString(hash[:])

	// Insert new version
	versionID := uuid.New()
	now := time.Now()
	_, err = h.pool.Exec(r.Context(),
		`INSERT INTO workflow_versions (id, workflow_id, version, schema_version, source_format, source_raw, definition, inputs_schema, checksum, published_at)
		 VALUES ($1, $2, $3, '1.0', $4, $5, $6, $7, $8, $9)`,
		versionID, workflowID, nextVersion, req.Format, req.Source, defMap, map[string]any{}, checksum, now)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to create version")
		return
	}

	// Update active version
	_, err = h.pool.Exec(r.Context(),
		`UPDATE workflows SET active_version_id = $1, updated_at = now() WHERE id = $2`,
		versionID, workflowID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to update active version")
		return
	}

	respondJSON(w, http.StatusCreated, map[string]any{
		"id":      versionID,
		"version": nextVersion,
	})
}

// Rollback sets the active version to a specified version number.
func (h *WorkflowHandler) Rollback(w http.ResponseWriter, r *http.Request) {
	tenantID := middleware.GetTenantID(r.Context())
	workflowID, err := parseUUID(chi.URLParam(r, "workflowID"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid workflow ID")
		return
	}

	var req struct {
		Version int `json:"version"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Version < 1 {
		respondError(w, http.StatusBadRequest, "version must be >= 1")
		return
	}

	// Find the version ID for the given version number
	var versionID uuid.UUID
	err = h.pool.QueryRow(r.Context(),
		`SELECT wv.id FROM workflow_versions wv
		 JOIN workflows w ON w.id = wv.workflow_id
		 WHERE wv.workflow_id = $1 AND wv.version = $2 AND w.tenant_id = $3`,
		workflowID, req.Version, tenantID).Scan(&versionID)
	if err != nil {
		respondError(w, http.StatusNotFound, "version not found")
		return
	}

	// Set active version
	_, err = h.pool.Exec(r.Context(),
		`UPDATE workflows SET active_version_id = $1, updated_at = now() WHERE id = $2 AND tenant_id = $3`,
		versionID, workflowID, tenantID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to rollback")
		return
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"active_version_id": versionID,
		"version":           req.Version,
	})
}

// ListVersions returns all versions for a workflow.
func (h *WorkflowHandler) ListVersions(w http.ResponseWriter, r *http.Request) {
	tenantID := middleware.GetTenantID(r.Context())
	workflowID, err := parseUUID(chi.URLParam(r, "workflowID"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid workflow ID")
		return
	}

	// Verify ownership
	var exists bool
	err = h.pool.QueryRow(r.Context(),
		`SELECT EXISTS(SELECT 1 FROM workflows WHERE id = $1 AND tenant_id = $2)`,
		workflowID, tenantID).Scan(&exists)
	if err != nil || !exists {
		respondError(w, http.StatusNotFound, "workflow not found")
		return
	}

	rows, err := h.pool.Query(r.Context(),
		`SELECT id, workflow_id, version, schema_version, source_format, source_raw, definition, checksum, published_at
		 FROM workflow_versions WHERE workflow_id = $1 ORDER BY version DESC`,
		workflowID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to list versions")
		return
	}
	defer rows.Close()

	type versionRow struct {
		ID            uuid.UUID      `json:"id"`
		WorkflowID    uuid.UUID      `json:"workflow_id"`
		Version       int            `json:"version"`
		SchemaVersion string         `json:"schema_version"`
		SourceFormat  string         `json:"source_format"`
		SourceRaw     string         `json:"source_raw"`
		Definition    map[string]any `json:"definition"`
		Checksum      string         `json:"checksum"`
		PublishedAt   *time.Time     `json:"published_at,omitempty"`
	}

	var versions []versionRow
	for rows.Next() {
		var v versionRow
		if err := rows.Scan(&v.ID, &v.WorkflowID, &v.Version, &v.SchemaVersion, &v.SourceFormat, &v.SourceRaw, &v.Definition, &v.Checksum, &v.PublishedAt); err != nil {
			respondError(w, http.StatusInternalServerError, "failed to scan version")
			return
		}
		versions = append(versions, v)
	}
	if versions == nil {
		versions = []versionRow{}
	}

	respondJSON(w, http.StatusOK, versions)
}

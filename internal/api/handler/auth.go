package handler

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"

	"github.com/flowctl/flowctl/internal/api/middleware"
	"github.com/flowctl/flowctl/internal/auth"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// AuthHandler handles authentication endpoints.
type AuthHandler struct {
	authService  *auth.Service
	oidcProvider *auth.OIDCProvider
	samlProvider *auth.SAMLProvider
	pool         *pgxpool.Pool
}

// NewAuthHandler creates a new AuthHandler.
func NewAuthHandler(authService *auth.Service, oidcProvider *auth.OIDCProvider, samlProvider *auth.SAMLProvider, pool *pgxpool.Pool) *AuthHandler {
	return &AuthHandler{
		authService:  authService,
		oidcProvider: oidcProvider,
		samlProvider: samlProvider,
		pool:         pool,
	}
}

// Login initiates the OIDC authentication flow.
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	// Generate a random state parameter
	stateBytes := make([]byte, 16)
	if _, err := rand.Read(stateBytes); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to generate state")
		return
	}
	state := hex.EncodeToString(stateBytes)

	// Store state in a short-lived record
	_, err := h.pool.Exec(r.Context(),
		`INSERT INTO auth_states (state, created_at) VALUES ($1, now())`,
		state)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to store auth state")
		return
	}

	authURL := h.oidcProvider.AuthURL(state)
	respondJSON(w, http.StatusOK, map[string]string{
		"auth_url": authURL,
		"state":    state,
	})
}

// OIDCCallback exchanges the authorization code for tokens.
func (h *AuthHandler) OIDCCallback(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")

	if code == "" || state == "" {
		respondError(w, http.StatusBadRequest, "code and state are required")
		return
	}

	// Validate state
	var stateExists bool
	err := h.pool.QueryRow(r.Context(),
		`SELECT EXISTS(SELECT 1 FROM auth_states WHERE state = $1 AND created_at > now() - interval '10 minutes')`,
		state).Scan(&stateExists)
	if err != nil || !stateExists {
		respondError(w, http.StatusBadRequest, "invalid or expired state")
		return
	}

	// Clean up used state
	_, _ = h.pool.Exec(r.Context(), `DELETE FROM auth_states WHERE state = $1`, state)

	// Exchange code for user info
	oidcUser, err := h.oidcProvider.Exchange(r.Context(), code)
	if err != nil {
		respondError(w, http.StatusUnauthorized, "failed to exchange code: "+err.Error())
		return
	}

	// Upsert user in the database
	userID, err := h.oidcProvider.UpsertUser(r.Context(), oidcUser)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to upsert user: "+err.Error())
		return
	}

	// Get user's default tenant and role
	var tenantID uuid.UUID
	var role string
	err = h.pool.QueryRow(r.Context(),
		`SELECT tenant_id, role FROM tenant_memberships WHERE user_id = $1 LIMIT 1`,
		userID).Scan(&tenantID, &role)
	if err != nil {
		// User has no tenant membership yet - create a personal tenant
		tenantID = uuid.New()
		_, _ = h.pool.Exec(r.Context(),
			`INSERT INTO tenants (id, name, slug, created_at, updated_at) VALUES ($1, $2, $3, now(), now())`,
			tenantID, oidcUser.Name+"'s Workspace", userID.String())
		_, _ = h.pool.Exec(r.Context(),
			`INSERT INTO tenant_memberships (user_id, tenant_id, role, created_at) VALUES ($1, $2, 'owner', now())`,
			userID, tenantID)
		role = "owner"
	}

	// Generate token pair
	tokenPair, err := h.authService.GenerateTokenPair(userID, oidcUser.Email, tenantID, role)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to generate tokens")
		return
	}

	respondJSON(w, http.StatusOK, tokenPair)
}

// Refresh exchanges a refresh token for a new token pair.
func (h *AuthHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	var req struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.RefreshToken == "" {
		respondError(w, http.StatusBadRequest, "refresh_token is required")
		return
	}

	// Validate the refresh token
	claims, err := h.authService.ValidateToken(req.RefreshToken)
	if err != nil {
		respondError(w, http.StatusUnauthorized, "invalid refresh token")
		return
	}

	// Look up user to get current tenant/role info
	var email, role string
	var tenantID uuid.UUID
	err = h.pool.QueryRow(r.Context(),
		`SELECT u.email, tm.tenant_id, tm.role
		 FROM users u
		 JOIN tenant_memberships tm ON tm.user_id = u.id
		 WHERE u.id = $1 LIMIT 1`,
		claims.UserID).Scan(&email, &tenantID, &role)
	if err != nil {
		respondError(w, http.StatusUnauthorized, "user not found")
		return
	}

	tokenPair, err := h.authService.GenerateTokenPair(claims.UserID, email, tenantID, role)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to generate tokens")
		return
	}

	respondJSON(w, http.StatusOK, tokenPair)
}

// SAMLACS handles SAML assertion consumer service callbacks.
func (h *AuthHandler) SAMLACS(w http.ResponseWriter, r *http.Request) {
	if h.samlProvider == nil {
		respondError(w, http.StatusNotImplemented, "SAML is not configured")
		return
	}

	if err := r.ParseForm(); err != nil {
		respondError(w, http.StatusBadRequest, "failed to parse form")
		return
	}

	samlResponse := r.FormValue("SAMLResponse")
	if samlResponse == "" {
		respondError(w, http.StatusBadRequest, "SAMLResponse is required")
		return
	}

	samlUser, err := h.samlProvider.HandleACS(r)
	if err != nil {
		respondError(w, http.StatusUnauthorized, "SAML authentication failed: "+err.Error())
		return
	}

	userID, err := h.samlProvider.UpsertUser(r.Context(), samlUser)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to upsert user: "+err.Error())
		return
	}

	var tenantID uuid.UUID
	var role string
	err = h.pool.QueryRow(r.Context(),
		`SELECT tenant_id, role FROM tenant_memberships WHERE user_id = $1 LIMIT 1`,
		userID).Scan(&tenantID, &role)
	if err != nil {
		tenantID = uuid.New()
		_, _ = h.pool.Exec(r.Context(),
			`INSERT INTO tenants (id, name, slug, created_at, updated_at) VALUES ($1, $2, $3, now(), now())`,
			tenantID, samlUser.Name+"'s Workspace", userID.String())
		_, _ = h.pool.Exec(r.Context(),
			`INSERT INTO tenant_memberships (user_id, tenant_id, role, created_at) VALUES ($1, $2, 'owner', now())`,
			userID, tenantID)
		role = "owner"
	}

	tokenPair, err := h.authService.GenerateTokenPair(userID, samlUser.Email, tenantID, role)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to generate tokens")
		return
	}

	respondJSON(w, http.StatusOK, tokenPair)
}

// Me returns the current user's information.
func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		respondError(w, http.StatusUnauthorized, "no claims in context")
		return
	}

	type userInfo struct {
		ID       uuid.UUID `json:"id"`
		Email    string    `json:"email"`
		Name     string    `json:"name"`
		TenantID uuid.UUID `json:"tenant_id"`
		Role     string    `json:"role"`
	}

	var info userInfo
	err := h.pool.QueryRow(r.Context(),
		`SELECT u.id, u.email, u.name FROM users u WHERE u.id = $1`,
		claims.UserID).Scan(&info.ID, &info.Email, &info.Name)
	if err != nil {
		respondError(w, http.StatusNotFound, "user not found")
		return
	}

	info.TenantID = claims.TenantID
	info.Role = claims.Role

	respondJSON(w, http.StatusOK, info)
}

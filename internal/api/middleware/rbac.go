package middleware

import (
	"net/http"

	"github.com/flowctl/flowctl/internal/auth"
)

type RBACMiddleware struct {
	engine *auth.RBACEngine
}

func NewRBACMiddleware(engine *auth.RBACEngine) *RBACMiddleware {
	return &RBACMiddleware{engine: engine}
}

func (m *RBACMiddleware) Require(resource, action string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims := GetClaims(r.Context())
			if claims == nil {
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				return
			}

			tenantID := GetTenantID(r.Context())

			allowed, err := m.engine.Check(r.Context(), auth.PermissionCheck{
				UserID:   claims.UserID,
				TenantID: tenantID,
				Resource: resource,
				Action:   action,
			})
			if err != nil || !allowed {
				http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

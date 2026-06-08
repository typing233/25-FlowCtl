package middleware

import (
	"context"
	"net/http"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

const TenantKey contextKey = "tenant_id"

type TenantMiddleware struct {
	pool *pgxpool.Pool
}

func NewTenantMiddleware(pool *pgxpool.Pool) *TenantMiddleware {
	return &TenantMiddleware{pool: pool}
}

func (m *TenantMiddleware) InjectTenant(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims := GetClaims(r.Context())
		if claims == nil {
			http.Error(w, `{"error":"no claims in context"}`, http.StatusUnauthorized)
			return
		}

		tenantID := claims.TenantID

		if headerTenant := r.Header.Get("X-Tenant-ID"); headerTenant != "" {
			parsed, err := uuid.Parse(headerTenant)
			if err == nil {
				var exists bool
				err := m.pool.QueryRow(r.Context(),
					`SELECT EXISTS(SELECT 1 FROM tenant_memberships WHERE user_id = $1 AND tenant_id = $2)`,
					claims.UserID, parsed).Scan(&exists)
				if err == nil && exists {
					tenantID = parsed
				}
			}
		}

		ctx := context.WithValue(r.Context(), TenantKey, tenantID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func GetTenantID(ctx context.Context) uuid.UUID {
	id, _ := ctx.Value(TenantKey).(uuid.UUID)
	return id
}

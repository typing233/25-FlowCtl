package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/flowctl/flowctl/internal/auth"
)

type contextKey string

const (
	ClaimsKey contextKey = "claims"
)

type AuthMiddleware struct {
	authService *auth.Service
}

func NewAuthMiddleware(authService *auth.Service) *AuthMiddleware {
	return &AuthMiddleware{authService: authService}
}

func (m *AuthMiddleware) Authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			http.Error(w, `{"error":"missing authorization header"}`, http.StatusUnauthorized)
			return
		}

		token := auth.ExtractBearerToken(authHeader)
		if token == "" {
			http.Error(w, `{"error":"invalid authorization header"}`, http.StatusUnauthorized)
			return
		}

		var claims *auth.Claims
		var err error

		if strings.HasPrefix(token, "fctl_") {
			claims, err = m.authService.ValidateAPIKey(r.Context(), token)
		} else {
			claims, err = m.authService.ValidateToken(token)
		}

		if err != nil {
			http.Error(w, `{"error":"unauthorized: `+err.Error()+`"}`, http.StatusUnauthorized)
			return
		}

		ctx := context.WithValue(r.Context(), ClaimsKey, claims)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func GetClaims(ctx context.Context) *auth.Claims {
	claims, _ := ctx.Value(ClaimsKey).(*auth.Claims)
	return claims
}

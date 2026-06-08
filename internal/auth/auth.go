package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Claims struct {
	jwt.RegisteredClaims
	UserID   uuid.UUID `json:"uid"`
	Email    string    `json:"email"`
	TenantID uuid.UUID `json:"tid"`
	Role     string    `json:"role"`
}

type TokenPair struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	ExpiresAt    time.Time `json:"expires_at"`
}

type Service struct {
	pool          *pgxpool.Pool
	jwtSecret     []byte
	jwtExpiry     time.Duration
	refreshExpiry time.Duration
}

func NewService(pool *pgxpool.Pool, jwtSecret string, jwtExpiry, refreshExpiry time.Duration) *Service {
	return &Service{
		pool:          pool,
		jwtSecret:     []byte(jwtSecret),
		jwtExpiry:     jwtExpiry,
		refreshExpiry: refreshExpiry,
	}
}

func (s *Service) GenerateTokenPair(userID uuid.UUID, email string, tenantID uuid.UUID, role string) (*TokenPair, error) {
	now := time.Now()
	expiresAt := now.Add(s.jwtExpiry)

	claims := &Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID.String(),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			Issuer:    "flowctl",
		},
		UserID:   userID,
		Email:    email,
		TenantID: tenantID,
		Role:     role,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	accessToken, err := token.SignedString(s.jwtSecret)
	if err != nil {
		return nil, fmt.Errorf("sign access token: %w", err)
	}

	refreshClaims := &jwt.RegisteredClaims{
		Subject:   userID.String(),
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(s.refreshExpiry)),
		Issuer:    "flowctl-refresh",
	}
	refreshToken := jwt.NewWithClaims(jwt.SigningMethodHS256, refreshClaims)
	refreshStr, err := refreshToken.SignedString(s.jwtSecret)
	if err != nil {
		return nil, fmt.Errorf("sign refresh token: %w", err)
	}

	return &TokenPair{
		AccessToken:  accessToken,
		RefreshToken: refreshStr,
		ExpiresAt:    expiresAt,
	}, nil
}

func (s *Service) ValidateToken(tokenStr string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(token *jwt.Token) (any, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return s.jwtSecret, nil
	})
	if err != nil {
		return nil, fmt.Errorf("parse token: %w", err)
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("invalid token claims")
	}

	return claims, nil
}

func (s *Service) ValidateAPIKey(ctx context.Context, key string) (*Claims, error) {
	hash := hashAPIKey(key)

	var userID, tenantID uuid.UUID
	var email, role string
	var expiresAt *time.Time

	err := s.pool.QueryRow(ctx,
		`SELECT ak.user_id, ak.tenant_id, u.email, tm.role, ak.expires_at
		 FROM api_keys ak
		 JOIN users u ON u.id = ak.user_id
		 JOIN tenant_memberships tm ON tm.user_id = ak.user_id AND tm.tenant_id = ak.tenant_id
		 WHERE ak.key_hash = $1`, hash).Scan(&userID, &tenantID, &email, &role, &expiresAt)
	if err != nil {
		return nil, fmt.Errorf("invalid API key")
	}

	if expiresAt != nil && expiresAt.Before(time.Now()) {
		return nil, fmt.Errorf("API key expired")
	}

	return &Claims{
		UserID:   userID,
		Email:    email,
		TenantID: tenantID,
		Role:     role,
	}, nil
}

func (s *Service) CreateAPIKey(ctx context.Context, userID, tenantID uuid.UUID, name string, scopes []string, expiresAt *time.Time) (string, error) {
	keyID := uuid.New()
	rawKey := fmt.Sprintf("fctl_%s", keyID.String())
	hash := hashAPIKey(rawKey)

	_, err := s.pool.Exec(ctx,
		`INSERT INTO api_keys (id, user_id, tenant_id, key_hash, name, scopes, expires_at, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, now())`,
		uuid.New(), userID, tenantID, hash, name, scopes, expiresAt)
	if err != nil {
		return "", fmt.Errorf("create API key: %w", err)
	}

	return rawKey, nil
}

func (s *Service) RefreshAccessToken(ctx context.Context, refreshToken string) (*TokenPair, error) {
	token, err := jwt.ParseWithClaims(refreshToken, &jwt.RegisteredClaims{}, func(token *jwt.Token) (any, error) {
		return s.jwtSecret, nil
	})
	if err != nil {
		return nil, fmt.Errorf("invalid refresh token")
	}

	claims, ok := token.Claims.(*jwt.RegisteredClaims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("invalid refresh token claims")
	}

	userID, err := uuid.Parse(claims.Subject)
	if err != nil {
		return nil, fmt.Errorf("invalid user ID in token")
	}

	var email string
	var tenantID uuid.UUID
	var role string
	err = s.pool.QueryRow(ctx,
		`SELECT u.email, tm.tenant_id, tm.role
		 FROM users u
		 JOIN tenant_memberships tm ON tm.user_id = u.id
		 WHERE u.id = $1
		 LIMIT 1`, userID).Scan(&email, &tenantID, &role)
	if err != nil {
		return nil, fmt.Errorf("user not found")
	}

	return s.GenerateTokenPair(userID, email, tenantID, role)
}

func hashAPIKey(key string) string {
	h := sha256.Sum256([]byte(key))
	return hex.EncodeToString(h[:])
}

func ExtractBearerToken(authHeader string) string {
	if strings.HasPrefix(authHeader, "Bearer ") {
		return strings.TrimPrefix(authHeader, "Bearer ")
	}
	return ""
}

package auth

import (
	"context"
	"fmt"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/oauth2"
)

type OIDCProvider struct {
	provider *oidc.Provider
	config   oauth2.Config
	verifier *oidc.IDTokenVerifier
	pool     *pgxpool.Pool
}

type OIDCConfig struct {
	IssuerURL    string
	ClientID     string
	ClientSecret string
	RedirectURL  string
}

func NewOIDCProvider(ctx context.Context, cfg OIDCConfig, pool *pgxpool.Pool) (*OIDCProvider, error) {
	provider, err := oidc.NewProvider(ctx, cfg.IssuerURL)
	if err != nil {
		return nil, fmt.Errorf("create oidc provider: %w", err)
	}

	oauthConfig := oauth2.Config{
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		RedirectURL:  cfg.RedirectURL,
		Endpoint:     provider.Endpoint(),
		Scopes:       []string{oidc.ScopeOpenID, "profile", "email"},
	}

	verifier := provider.Verifier(&oidc.Config{ClientID: cfg.ClientID})

	return &OIDCProvider{
		provider: provider,
		config:   oauthConfig,
		verifier: verifier,
		pool:     pool,
	}, nil
}

func (p *OIDCProvider) AuthURL(state string) string {
	return p.config.AuthCodeURL(state, oauth2.AccessTypeOffline)
}

func (p *OIDCProvider) Exchange(ctx context.Context, code string) (*OIDCUser, error) {
	token, err := p.config.Exchange(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("exchange code: %w", err)
	}

	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok {
		return nil, fmt.Errorf("no id_token in response")
	}

	idToken, err := p.verifier.Verify(ctx, rawIDToken)
	if err != nil {
		return nil, fmt.Errorf("verify id_token: %w", err)
	}

	var claims struct {
		Email         string `json:"email"`
		EmailVerified bool   `json:"email_verified"`
		Name          string `json:"name"`
		Subject       string `json:"sub"`
	}
	if err := idToken.Claims(&claims); err != nil {
		return nil, fmt.Errorf("extract claims: %w", err)
	}

	return &OIDCUser{
		Subject: claims.Subject,
		Email:   claims.Email,
		Name:    claims.Name,
		Issuer:  idToken.Issuer,
	}, nil
}

type OIDCUser struct {
	Subject string
	Email   string
	Name    string
	Issuer  string
}

func (p *OIDCProvider) UpsertUser(ctx context.Context, user *OIDCUser) (uuid.UUID, error) {
	var userID uuid.UUID

	err := p.pool.QueryRow(ctx,
		`SELECT id FROM users WHERE idp_subject = $1 AND idp_issuer = $2`,
		user.Subject, user.Issuer).Scan(&userID)

	if err == nil {
		return userID, nil
	}

	userID = uuid.New()
	_, err = p.pool.Exec(ctx,
		`INSERT INTO users (id, email, name, idp_subject, idp_issuer, created_at)
		 VALUES ($1, $2, $3, $4, $5, now())
		 ON CONFLICT (email) DO UPDATE SET
		   name = EXCLUDED.name,
		   idp_subject = EXCLUDED.idp_subject,
		   idp_issuer = EXCLUDED.idp_issuer
		 RETURNING id`,
		userID, user.Email, user.Name, user.Subject, user.Issuer)
	if err != nil {
		return uuid.Nil, fmt.Errorf("upsert user: %w", err)
	}

	return userID, nil
}

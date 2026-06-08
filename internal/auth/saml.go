package auth

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/url"
	"os"

	"github.com/crewjam/saml"
	"github.com/crewjam/saml/samlsp"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type SAMLProvider struct {
	sp   *samlsp.Middleware
	pool *pgxpool.Pool
}

type SAMLConfig struct {
	CertPath    string
	KeyPath     string
	EntityID    string
	MetadataURL string
	ACSURL      string
	SLOURL      string
}

func NewSAMLProvider(cfg SAMLConfig, pool *pgxpool.Pool) (*SAMLProvider, error) {
	keyData, err := os.ReadFile(cfg.KeyPath)
	if err != nil {
		return nil, fmt.Errorf("read SAML key: %w", err)
	}
	block, _ := pem.Decode(keyData)
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM block from key")
	}
	key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse private key: %w", err)
	}

	certData, err := os.ReadFile(cfg.CertPath)
	if err != nil {
		return nil, fmt.Errorf("read SAML cert: %w", err)
	}
	certBlock, _ := pem.Decode(certData)
	if certBlock == nil {
		return nil, fmt.Errorf("failed to decode PEM block from cert")
	}
	cert, err := x509.ParseCertificate(certBlock.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse certificate: %w", err)
	}

	rootURL, err := url.Parse(cfg.MetadataURL)
	if err != nil {
		return nil, fmt.Errorf("parse metadata URL: %w", err)
	}

	sp, err := samlsp.New(samlsp.Options{
		EntityID:    cfg.EntityID,
		URL:         *rootURL,
		Key:         key,
		Certificate: cert,
	})
	if err != nil {
		return nil, fmt.Errorf("create SAML SP: %w", err)
	}

	return &SAMLProvider{
		sp:   sp,
		pool: pool,
	}, nil
}

func (p *SAMLProvider) GetMiddleware() *samlsp.Middleware {
	return p.sp
}

func (p *SAMLProvider) HandleACS(r *http.Request) (*SAMLUser, error) {
	assertionInfo, err := p.sp.ServiceProvider.ParseResponse(r, []string{""})
	if err != nil {
		return nil, fmt.Errorf("parse SAML response: %w", err)
	}

	user := &SAMLUser{}
	if assertionInfo.Subject != nil && assertionInfo.Subject.NameID != nil {
		user.Subject = assertionInfo.Subject.NameID.Value
	}

	for _, stmt := range assertionInfo.AttributeStatements {
		for _, attr := range stmt.Attributes {
			val := ""
			if len(attr.Values) > 0 {
				val = attr.Values[0].Value
			}
			switch attr.Name {
			case "email", "urn:oid:0.9.2342.19200300.100.1.3",
				"http://schemas.xmlsoap.org/ws/2005/05/identity/claims/emailaddress":
				user.Email = val
			case "displayName", "urn:oid:2.16.840.1.113730.3.1.241",
				"http://schemas.xmlsoap.org/ws/2005/05/identity/claims/name":
				user.Name = val
			case "uid", "urn:oid:0.9.2342.19200300.100.1.1":
				if user.Subject == "" {
					user.Subject = val
				}
			}
		}
	}

	if user.Email == "" && user.Subject != "" {
		user.Email = user.Subject
	}
	if user.Email == "" {
		return nil, fmt.Errorf("no email found in SAML assertion")
	}
	if user.Name == "" {
		user.Name = user.Email
	}

	return user, nil
}

func (p *SAMLProvider) ExtractUser(r *http.Request) (*SAMLUser, error) {
	session := samlsp.SessionFromContext(r.Context())
	if session == nil {
		return nil, fmt.Errorf("no SAML session")
	}

	sa, ok := session.(samlsp.SessionWithAttributes)
	if !ok {
		return nil, fmt.Errorf("session has no attributes")
	}

	attrs := sa.GetAttributes()
	return &SAMLUser{
		Email:   getFirst(attrs, "email", "urn:oid:0.9.2342.19200300.100.1.3"),
		Name:    getFirst(attrs, "displayName", "urn:oid:2.16.840.1.113730.3.1.241"),
		Subject: getFirst(attrs, "uid", "urn:oid:0.9.2342.19200300.100.1.1"),
	}, nil
}

func (p *SAMLProvider) UpsertUser(ctx context.Context, user *SAMLUser) (uuid.UUID, error) {
	var userID uuid.UUID
	err := p.pool.QueryRow(ctx,
		`SELECT id FROM users WHERE email = $1`, user.Email).Scan(&userID)
	if err == nil {
		return userID, nil
	}

	userID = uuid.New()
	_, err = p.pool.Exec(ctx,
		`INSERT INTO users (id, email, name, idp_subject, idp_issuer, created_at)
		 VALUES ($1, $2, $3, $4, 'saml', now())
		 ON CONFLICT (email) DO UPDATE SET name = EXCLUDED.name`,
		userID, user.Email, user.Name, user.Subject)
	if err != nil {
		return uuid.Nil, err
	}
	return userID, nil
}

type SAMLUser struct {
	Email   string
	Name    string
	Subject string
	Groups  []string
}

func getFirst(attrs samlsp.Attributes, keys ...string) string {
	for _, key := range keys {
		if v := attrs.Get(key); v != "" {
			return v
		}
	}
	return ""
}

// compile-time interface checks
var _ = (*saml.Assertion)(nil)

package model

import (
	"time"

	"github.com/google/uuid"
)

type Tenant struct {
	ID        uuid.UUID         `json:"id" db:"id"`
	Slug      string            `json:"slug" db:"slug"`
	Name      string            `json:"name" db:"name"`
	Config    map[string]any    `json:"config" db:"config"`
	CreatedAt time.Time         `json:"created_at" db:"created_at"`
	UpdatedAt time.Time         `json:"updated_at" db:"updated_at"`
}

type User struct {
	ID           uuid.UUID `json:"id" db:"id"`
	Email        string    `json:"email" db:"email"`
	Name         string    `json:"name" db:"name"`
	IDPSubject   string    `json:"idp_subject,omitempty" db:"idp_subject"`
	IDPIssuer    string    `json:"idp_issuer,omitempty" db:"idp_issuer"`
	PasswordHash string    `json:"-" db:"password_hash"`
	CreatedAt    time.Time `json:"created_at" db:"created_at"`
}

type TenantMembership struct {
	UserID    uuid.UUID `json:"user_id" db:"user_id"`
	TenantID  uuid.UUID `json:"tenant_id" db:"tenant_id"`
	Role      string    `json:"role" db:"role"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
}

type RBACLevel string

const (
	RBACLevelSystem   RBACLevel = "system"
	RBACLevelTenant   RBACLevel = "tenant"
	RBACLevelWorkflow RBACLevel = "workflow"
	RBACLevelNode     RBACLevel = "node"
	RBACLevelOperation RBACLevel = "operation"
)

type Role struct {
	ID       uuid.UUID  `json:"id" db:"id"`
	TenantID *uuid.UUID `json:"tenant_id,omitempty" db:"tenant_id"`
	Name     string     `json:"name" db:"name"`
	Level    RBACLevel  `json:"level" db:"level"`
}

type Permission struct {
	ID         uuid.UUID      `json:"id" db:"id"`
	RoleID     uuid.UUID      `json:"role_id" db:"role_id"`
	Resource   string         `json:"resource" db:"resource"`
	Action     string         `json:"action" db:"action"`
	Conditions map[string]any `json:"conditions,omitempty" db:"conditions"`
}

type APIKey struct {
	ID        uuid.UUID  `json:"id" db:"id"`
	UserID    uuid.UUID  `json:"user_id" db:"user_id"`
	TenantID  uuid.UUID  `json:"tenant_id" db:"tenant_id"`
	KeyHash   string     `json:"-" db:"key_hash"`
	Name      string     `json:"name" db:"name"`
	Scopes    []string   `json:"scopes" db:"scopes"`
	ExpiresAt *time.Time `json:"expires_at,omitempty" db:"expires_at"`
	CreatedAt time.Time  `json:"created_at" db:"created_at"`
}

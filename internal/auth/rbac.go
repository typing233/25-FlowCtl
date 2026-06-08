package auth

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type RBACEngine struct {
	pool *pgxpool.Pool
}

func NewRBACEngine(pool *pgxpool.Pool) *RBACEngine {
	return &RBACEngine{pool: pool}
}

type PermissionCheck struct {
	UserID   uuid.UUID
	TenantID uuid.UUID
	Resource string
	Action   string
}

func (r *RBACEngine) Check(ctx context.Context, check PermissionCheck) (bool, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT p.resource, p.action
		 FROM permissions p
		 JOIN roles ro ON ro.id = p.role_id
		 LEFT JOIN tenant_memberships tm ON tm.role = ro.name AND tm.tenant_id = ro.tenant_id
		 WHERE (tm.user_id = $1 AND (tm.tenant_id = $2 OR ro.tenant_id IS NULL))
		    OR (ro.tenant_id IS NULL AND ro.name IN (
		        SELECT tm2.role FROM tenant_memberships tm2 WHERE tm2.user_id = $1
		    ))`,
		check.UserID, check.TenantID)
	if err != nil {
		return false, fmt.Errorf("query permissions: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var resource, action string
		if err := rows.Scan(&resource, &action); err != nil {
			continue
		}
		if matchesResource(resource, check.Resource) && matchesAction(action, check.Action) {
			return true, nil
		}
	}

	return false, nil
}

func (r *RBACEngine) CheckBatch(ctx context.Context, userID, tenantID uuid.UUID, checks []PermissionCheck) (map[string]bool, error) {
	results := make(map[string]bool)

	for _, check := range checks {
		check.UserID = userID
		check.TenantID = tenantID
		allowed, err := r.Check(ctx, check)
		if err != nil {
			return nil, err
		}
		key := check.Resource + ":" + check.Action
		results[key] = allowed
	}

	return results, nil
}

func (r *RBACEngine) GetUserPermissions(ctx context.Context, userID, tenantID uuid.UUID) ([]Permission, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT p.id, p.role_id, p.resource, p.action
		 FROM permissions p
		 JOIN roles ro ON ro.id = p.role_id
		 JOIN tenant_memberships tm ON tm.role = ro.name AND (tm.tenant_id = ro.tenant_id OR ro.tenant_id IS NULL)
		 WHERE tm.user_id = $1 AND (tm.tenant_id = $2 OR ro.tenant_id IS NULL)`,
		userID, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var perms []Permission
	for rows.Next() {
		var p Permission
		if err := rows.Scan(&p.ID, &p.RoleID, &p.Resource, &p.Action); err != nil {
			continue
		}
		perms = append(perms, p)
	}
	return perms, nil
}

func (r *RBACEngine) AssignRole(ctx context.Context, userID, tenantID uuid.UUID, roleName string) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO tenant_memberships (user_id, tenant_id, role, created_at)
		 VALUES ($1, $2, $3, now())
		 ON CONFLICT (user_id, tenant_id) DO UPDATE SET role = $3`,
		userID, tenantID, roleName)
	return err
}

type Permission struct {
	ID       uuid.UUID
	RoleID   uuid.UUID
	Resource string
	Action   string
}

func matchesResource(pattern, resource string) bool {
	if pattern == "*" {
		return true
	}
	if strings.HasSuffix(pattern, ":*") {
		prefix := strings.TrimSuffix(pattern, "*")
		return strings.HasPrefix(resource, prefix)
	}
	return pattern == resource
}

func matchesAction(pattern, action string) bool {
	if pattern == "*" {
		return true
	}
	return pattern == action
}

package secrets

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Vault struct {
	pool      *pgxpool.Pool
	masterKey []byte
}

func NewVault(pool *pgxpool.Pool, masterKeyHex string) (*Vault, error) {
	if masterKeyHex == "" {
		key := make([]byte, 32)
		if _, err := rand.Read(key); err != nil {
			return nil, err
		}
		masterKeyHex = hex.EncodeToString(key)
	}

	masterKey, err := hex.DecodeString(masterKeyHex)
	if err != nil {
		return nil, fmt.Errorf("invalid master key: %w", err)
	}
	if len(masterKey) != 32 {
		return nil, fmt.Errorf("master key must be 32 bytes (64 hex characters)")
	}

	return &Vault{pool: pool, masterKey: masterKey}, nil
}

func (v *Vault) Set(ctx context.Context, tenantID uuid.UUID, name, value, scope string, scopeID *uuid.UUID) error {
	encrypted, err := v.encrypt([]byte(value))
	if err != nil {
		return fmt.Errorf("encrypt: %w", err)
	}

	_, err = v.pool.Exec(ctx,
		`INSERT INTO secrets (id, tenant_id, name, encrypted_value, scope, scope_id, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, now(), now())
		 ON CONFLICT (tenant_id, name, scope, scope_id)
		 DO UPDATE SET encrypted_value = EXCLUDED.encrypted_value, updated_at = now()`,
		uuid.New(), tenantID, name, encrypted, scope, scopeID)
	return err
}

func (v *Vault) Get(ctx context.Context, tenantID uuid.UUID, name string) (string, error) {
	var encrypted []byte
	err := v.pool.QueryRow(ctx,
		`SELECT encrypted_value FROM secrets
		 WHERE tenant_id = $1 AND name = $2
		 ORDER BY scope = 'tenant' DESC
		 LIMIT 1`, tenantID, name).Scan(&encrypted)
	if err != nil {
		return "", fmt.Errorf("secret %q not found", name)
	}

	decrypted, err := v.decrypt(encrypted)
	if err != nil {
		return "", fmt.Errorf("decrypt: %w", err)
	}
	return string(decrypted), nil
}

func (v *Vault) GetForExecution(ctx context.Context, tenantID, workflowID uuid.UUID) (map[string]string, error) {
	rows, err := v.pool.Query(ctx,
		`SELECT name, encrypted_value FROM secrets
		 WHERE tenant_id = $1 AND (scope = 'tenant' OR (scope = 'workflow' AND scope_id = $2))`,
		tenantID, workflowID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	secrets := make(map[string]string)
	for rows.Next() {
		var name string
		var encrypted []byte
		if err := rows.Scan(&name, &encrypted); err != nil {
			continue
		}
		decrypted, err := v.decrypt(encrypted)
		if err != nil {
			continue
		}
		secrets[name] = string(decrypted)
	}
	return secrets, nil
}

func (v *Vault) Delete(ctx context.Context, tenantID uuid.UUID, name string) error {
	_, err := v.pool.Exec(ctx,
		"DELETE FROM secrets WHERE tenant_id = $1 AND name = $2", tenantID, name)
	return err
}

func (v *Vault) List(ctx context.Context, tenantID uuid.UUID) ([]SecretMeta, error) {
	rows, err := v.pool.Query(ctx,
		`SELECT id, name, scope, scope_id, created_at, updated_at
		 FROM secrets WHERE tenant_id = $1 ORDER BY name`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []SecretMeta
	for rows.Next() {
		var s SecretMeta
		if err := rows.Scan(&s.ID, &s.Name, &s.Scope, &s.ScopeID, &s.CreatedAt, &s.UpdatedAt); err != nil {
			continue
		}
		results = append(results, s)
	}
	return results, nil
}

type SecretMeta struct {
	ID        uuid.UUID  `json:"id"`
	Name      string     `json:"name"`
	Scope     string     `json:"scope"`
	ScopeID   *uuid.UUID `json:"scope_id,omitempty"`
	CreatedAt string     `json:"created_at"`
	UpdatedAt string     `json:"updated_at"`
}

func (v *Vault) encrypt(plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher(v.masterKey)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}

	return gcm.Seal(nonce, nonce, plaintext, nil), nil
}

func (v *Vault) decrypt(ciphertext []byte) ([]byte, error) {
	block, err := aes.NewCipher(v.masterKey)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, fmt.Errorf("ciphertext too short")
	}

	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	return gcm.Open(nil, nonce, ciphertext, nil)
}

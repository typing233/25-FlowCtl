package model

import (
	"time"

	"github.com/google/uuid"
)

type SourceFormat string

const (
	SourceFormatYAML SourceFormat = "yaml"
	SourceFormatHUML SourceFormat = "huml"
)

type Workflow struct {
	ID              uuid.UUID  `json:"id" db:"id"`
	TenantID        uuid.UUID  `json:"tenant_id" db:"tenant_id"`
	Slug            string     `json:"slug" db:"slug"`
	Name            string     `json:"name" db:"name"`
	Description     string     `json:"description" db:"description"`
	ActiveVersionID *uuid.UUID `json:"active_version_id,omitempty" db:"active_version_id"`
	CreatedAt       time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at" db:"updated_at"`
}

type WorkflowVersion struct {
	ID            uuid.UUID      `json:"id" db:"id"`
	WorkflowID    uuid.UUID      `json:"workflow_id" db:"workflow_id"`
	Version       int            `json:"version" db:"version"`
	SchemaVersion string         `json:"schema_version" db:"schema_version"`
	SourceFormat  SourceFormat   `json:"source_format" db:"source_format"`
	SourceRaw     string         `json:"source_raw" db:"source_raw"`
	Definition    map[string]any `json:"definition" db:"definition"`
	InputsSchema  map[string]any `json:"inputs_schema" db:"inputs_schema"`
	Checksum      string         `json:"checksum" db:"checksum"`
	PublishedAt   *time.Time     `json:"published_at,omitempty" db:"published_at"`
	PublishedBy   *uuid.UUID     `json:"published_by,omitempty" db:"published_by"`
	CreatedAt     time.Time      `json:"created_at" db:"created_at"`
}

type InputType string

const (
	InputTypeString  InputType = "string"
	InputTypeNumber  InputType = "number"
	InputTypeBool    InputType = "boolean"
	InputTypeObject  InputType = "object"
	InputTypeArray   InputType = "array"
)

type InputDef struct {
	Name        string    `json:"name" yaml:"name"`
	Type        InputType `json:"type" yaml:"type"`
	Required    bool      `json:"required" yaml:"required"`
	Default     any       `json:"default,omitempty" yaml:"default,omitempty"`
	Description string    `json:"description,omitempty" yaml:"description,omitempty"`
	Enum        []any     `json:"enum,omitempty" yaml:"enum,omitempty"`
}

type RetryPolicy struct {
	MaxAttempts int    `json:"max_attempts" yaml:"max_attempts"`
	Backoff     string `json:"backoff" yaml:"backoff"`
	InitialWait string `json:"initial_wait,omitempty" yaml:"initial_wait,omitempty"`
	MaxWait     string `json:"max_wait,omitempty" yaml:"max_wait,omitempty"`
}

type CompensationDef struct {
	Runner  string         `json:"runner" yaml:"runner"`
	Config  map[string]any `json:"config,omitempty" yaml:"config,omitempty"`
	Command string         `json:"command" yaml:"command"`
}

type StepDef struct {
	ID           string          `json:"id" yaml:"id"`
	Name         string          `json:"name,omitempty" yaml:"name,omitempty"`
	Runner       string          `json:"runner" yaml:"runner"`
	DependsOn    []string        `json:"depends_on,omitempty" yaml:"depends_on,omitempty"`
	When         string          `json:"when,omitempty" yaml:"when,omitempty"`
	Config       map[string]any  `json:"config" yaml:"config"`
	Command      string          `json:"command,omitempty" yaml:"command,omitempty"`
	Image        string          `json:"image,omitempty" yaml:"image,omitempty"`
	Host         string          `json:"host,omitempty" yaml:"host,omitempty"`
	Timeout      string          `json:"timeout,omitempty" yaml:"timeout,omitempty"`
	Retry        *RetryPolicy    `json:"retry,omitempty" yaml:"retry,omitempty"`
	Compensation *CompensationDef `json:"compensation,omitempty" yaml:"compensation,omitempty"`
	Inputs       map[string]any  `json:"inputs,omitempty" yaml:"inputs,omitempty"`
	Env          map[string]string `json:"env,omitempty" yaml:"env,omitempty"`
}

type ApprovalDef struct {
	ID            string   `json:"id" yaml:"id"`
	DependsOn     []string `json:"depends_on,omitempty" yaml:"depends_on,omitempty"`
	When          string   `json:"when,omitempty" yaml:"when,omitempty"`
	RequiredRoles []string `json:"required_roles" yaml:"required_roles"`
	Message       string   `json:"message,omitempty" yaml:"message,omitempty"`
}

type WorkflowDefinition struct {
	Name        string        `json:"name" yaml:"name"`
	Version     string        `json:"version" yaml:"version"`
	Description string        `json:"description,omitempty" yaml:"description,omitempty"`
	Inputs      []InputDef    `json:"inputs" yaml:"inputs"`
	Steps       []StepDef     `json:"steps" yaml:"steps"`
	Approvals   []ApprovalDef `json:"approvals,omitempty" yaml:"approvals,omitempty"`
}

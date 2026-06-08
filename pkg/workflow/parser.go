package workflow

import (
	"fmt"

	"gopkg.in/yaml.v3"

	"github.com/flowctl/flowctl/internal/model"
)

type yamlWorkflow struct {
	Name        string           `yaml:"name"`
	Version     string           `yaml:"version"`
	Description string           `yaml:"description,omitempty"`
	Inputs      []yamlInput      `yaml:"inputs"`
	Steps       []yamlStep       `yaml:"steps"`
	Approvals   []yamlApproval   `yaml:"approvals,omitempty"`
}

type yamlInput struct {
	Name        string `yaml:"name"`
	Type        string `yaml:"type"`
	Required    bool   `yaml:"required"`
	Default     any    `yaml:"default,omitempty"`
	Description string `yaml:"description,omitempty"`
	Enum        []any  `yaml:"enum,omitempty"`
}

type yamlStep struct {
	ID           string            `yaml:"id"`
	Name         string            `yaml:"name,omitempty"`
	Runner       string            `yaml:"runner"`
	DependsOn    []string          `yaml:"depends_on,omitempty"`
	When         string            `yaml:"when,omitempty"`
	Command      string            `yaml:"command"`
	Image        string            `yaml:"image,omitempty"`
	Host         string            `yaml:"host,omitempty"`
	Timeout      string            `yaml:"timeout,omitempty"`
	Retry        *yamlRetry        `yaml:"retry,omitempty"`
	Compensation *yamlCompensation `yaml:"compensation,omitempty"`
	Env          map[string]string `yaml:"env,omitempty"`
	Config       map[string]any    `yaml:"config,omitempty"`
}

type yamlRetry struct {
	MaxAttempts int    `yaml:"max_attempts"`
	Backoff     string `yaml:"backoff"`
	InitialWait string `yaml:"initial_wait,omitempty"`
	MaxWait     string `yaml:"max_wait,omitempty"`
}

type yamlApproval struct {
	ID            string   `yaml:"id"`
	DependsOn     []string `yaml:"depends_on,omitempty"`
	When          string   `yaml:"when,omitempty"`
	RequiredRoles []string `yaml:"required_roles"`
	Message       string   `yaml:"message,omitempty"`
}

type yamlCompensation struct {
	Runner  string         `yaml:"runner,omitempty"`
	Command string         `yaml:"command"`
	Config  map[string]any `yaml:"config,omitempty"`
}

func ParseYAML(source []byte) (*model.WorkflowDefinition, error) {
	var raw yamlWorkflow
	if err := yaml.Unmarshal(source, &raw); err != nil {
		return nil, fmt.Errorf("yaml parse error: %w", err)
	}

	def := &model.WorkflowDefinition{
		Name:        raw.Name,
		Version:     raw.Version,
		Description: raw.Description,
	}

	for _, input := range raw.Inputs {
		def.Inputs = append(def.Inputs, model.InputDef{
			Name:        input.Name,
			Type:        model.InputType(input.Type),
			Required:    input.Required,
			Default:     input.Default,
			Description: input.Description,
			Enum:        input.Enum,
		})
	}

	for _, step := range raw.Steps {
		s := model.StepDef{
			ID:        step.ID,
			Name:      step.Name,
			Runner:    step.Runner,
			DependsOn: step.DependsOn,
			When:      step.When,
			Command:   step.Command,
			Image:     step.Image,
			Host:      step.Host,
			Timeout:   step.Timeout,
			Env:       step.Env,
			Config:    step.Config,
		}
		if s.Config == nil {
			s.Config = make(map[string]any)
		}
		if step.Retry != nil {
			s.Retry = &model.RetryPolicy{
				MaxAttempts: step.Retry.MaxAttempts,
				Backoff:     step.Retry.Backoff,
				InitialWait: step.Retry.InitialWait,
				MaxWait:     step.Retry.MaxWait,
			}
		}
		if step.Compensation != nil {
			s.Compensation = &model.CompensationDef{
				Runner:  step.Compensation.Runner,
				Command: step.Compensation.Command,
				Config:  step.Compensation.Config,
			}
		}
		def.Steps = append(def.Steps, s)
	}

	for _, approval := range raw.Approvals {
		def.Approvals = append(def.Approvals, model.ApprovalDef{
			ID:            approval.ID,
			DependsOn:     approval.DependsOn,
			When:          approval.When,
			RequiredRoles: approval.RequiredRoles,
			Message:       approval.Message,
		})
	}

	return def, nil
}

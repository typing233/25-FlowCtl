package validator

import (
	"fmt"

	"github.com/flowctl/flowctl/internal/engine"
	"github.com/flowctl/flowctl/internal/model"
)

type ValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

type ValidationResult struct {
	Valid  bool              `json:"valid"`
	Errors []ValidationError `json:"errors,omitempty"`
}

func ValidateWorkflow(def *model.WorkflowDefinition) *ValidationResult {
	result := &ValidationResult{Valid: true}

	if def.Name == "" {
		result.addError("name", "workflow name is required")
	}

	validateInputs(def.Inputs, result)
	validateSteps(def.Steps, result)
	validateApprovals(def.Approvals, result)
	validateDAG(def, result)
	validateExpressions(def, result)

	return result
}

func validateInputs(inputs []model.InputDef, result *ValidationResult) {
	seen := make(map[string]bool)
	for _, input := range inputs {
		if input.Name == "" {
			result.addError("inputs", "input name is required")
			continue
		}
		if seen[input.Name] {
			result.addError("inputs."+input.Name, "duplicate input name")
		}
		seen[input.Name] = true

		switch input.Type {
		case model.InputTypeString, model.InputTypeNumber, model.InputTypeBool, model.InputTypeObject, model.InputTypeArray:
		case "":
			result.addError("inputs."+input.Name+".type", "type is required")
		default:
			result.addError("inputs."+input.Name+".type", fmt.Sprintf("invalid type %q", input.Type))
		}
	}
}

func validateSteps(steps []model.StepDef, result *ValidationResult) {
	seen := make(map[string]bool)
	for _, step := range steps {
		if step.ID == "" {
			result.addError("steps", "step id is required")
			continue
		}
		if seen[step.ID] {
			result.addError("steps."+step.ID, "duplicate step id")
		}
		seen[step.ID] = true

		switch step.Runner {
		case "docker", "local", "ssh":
		case "":
			result.addError("steps."+step.ID+".runner", "runner is required")
		default:
			result.addError("steps."+step.ID+".runner", fmt.Sprintf("unknown runner %q", step.Runner))
		}

		if step.Command == "" {
			result.addError("steps."+step.ID+".command", "command is required")
		}

		if step.Runner == "docker" && step.Image == "" {
			result.addError("steps."+step.ID+".image", "image is required for docker runner")
		}

		if step.Runner == "ssh" && step.Host == "" {
			result.addError("steps."+step.ID+".host", "host is required for ssh runner")
		}

		if step.Retry != nil {
			if step.Retry.MaxAttempts < 1 {
				result.addError("steps."+step.ID+".retry.max_attempts", "must be >= 1")
			}
			switch step.Retry.Backoff {
			case "exponential", "linear", "fixed", "":
			default:
				result.addError("steps."+step.ID+".retry.backoff", fmt.Sprintf("invalid backoff %q", step.Retry.Backoff))
			}
		}
	}
}

func validateApprovals(approvals []model.ApprovalDef, result *ValidationResult) {
	seen := make(map[string]bool)
	for _, approval := range approvals {
		if approval.ID == "" {
			result.addError("approvals", "approval id is required")
			continue
		}
		if seen[approval.ID] {
			result.addError("approvals."+approval.ID, "duplicate approval id")
		}
		seen[approval.ID] = true

		if len(approval.RequiredRoles) == 0 {
			result.addError("approvals."+approval.ID+".required_roles", "at least one role is required")
		}
	}
}

func validateDAG(def *model.WorkflowDefinition, result *ValidationResult) {
	dag, err := engine.BuildDAG(def)
	if err != nil {
		result.addError("dag", err.Error())
		return
	}

	if err := dag.DetectCycles(); err != nil {
		result.addError("dag", err.Error())
	}
}

func validateExpressions(def *model.WorkflowDefinition, result *ValidationResult) {
	knownInputs := make(map[string]bool)
	for _, input := range def.Inputs {
		knownInputs[input.Name] = true
	}

	knownSteps := make(map[string]bool)
	for _, step := range def.Steps {
		knownSteps[step.ID] = true
	}
	for _, approval := range def.Approvals {
		knownSteps[approval.ID] = true
	}

	for _, step := range def.Steps {
		for _, dep := range step.DependsOn {
			if !knownSteps[dep] {
				result.addError("steps."+step.ID+".depends_on", fmt.Sprintf("unknown dependency %q", dep))
			}
		}
	}

	for _, approval := range def.Approvals {
		for _, dep := range approval.DependsOn {
			if !knownSteps[dep] {
				result.addError("approvals."+approval.ID+".depends_on", fmt.Sprintf("unknown dependency %q", dep))
			}
		}
	}
}

func (r *ValidationResult) addError(field, message string) {
	r.Valid = false
	r.Errors = append(r.Errors, ValidationError{Field: field, Message: message})
}

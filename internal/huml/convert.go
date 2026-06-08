package huml

import (
	"fmt"

	"github.com/flowctl/flowctl/internal/model"
)

func Convert(node *WorkflowNode) (*model.WorkflowDefinition, error) {
	def := &model.WorkflowDefinition{
		Name: node.Name,
	}

	for _, child := range node.Body {
		switch n := child.(type) {
		case *AssignNode:
			switch n.Key {
			case "version":
				def.Version = nodeToString(n.Value)
			case "description":
				def.Description = nodeToString(n.Value)
			}
		case *BlockNode:
			switch n.Type {
			case "input":
				input, err := convertInput(n)
				if err != nil {
					return nil, err
				}
				def.Inputs = append(def.Inputs, *input)
			case "step":
				step, err := convertStep(n)
				if err != nil {
					return nil, err
				}
				def.Steps = append(def.Steps, *step)
			case "approval":
				approval, err := convertApproval(n)
				if err != nil {
					return nil, err
				}
				def.Approvals = append(def.Approvals, *approval)
			}
		}
	}

	return def, nil
}

func convertInput(block *BlockNode) (*model.InputDef, error) {
	input := &model.InputDef{
		Name: block.Name,
	}

	for _, child := range block.Body {
		assign, ok := child.(*AssignNode)
		if !ok {
			continue
		}
		switch assign.Key {
		case "type":
			input.Type = model.InputType(nodeToString(assign.Value))
		case "required":
			input.Required = nodeToBool(assign.Value)
		case "default":
			input.Default = nodeToAny(assign.Value)
		case "description":
			input.Description = nodeToString(assign.Value)
		case "enum":
			input.Enum = nodeToSlice(assign.Value)
		}
	}

	return input, nil
}

func convertStep(block *BlockNode) (*model.StepDef, error) {
	step := &model.StepDef{
		ID:     block.Name,
		Config: make(map[string]any),
		Env:    make(map[string]string),
	}

	for _, child := range block.Body {
		switch n := child.(type) {
		case *AssignNode:
			switch n.Key {
			case "runner":
				step.Runner = nodeToString(n.Value)
			case "image":
				step.Image = nodeToString(n.Value)
			case "command":
				step.Command = nodeToString(n.Value)
			case "host":
				step.Host = nodeToString(n.Value)
			case "timeout":
				step.Timeout = nodeToString(n.Value)
			case "depends_on":
				step.DependsOn = nodeToStringSlice(n.Value)
			case "when":
				step.When = nodeToString(n.Value)
			case "name":
				step.Name = nodeToString(n.Value)
			default:
				step.Config[n.Key] = nodeToAny(n.Value)
			}
		case *BlockNode:
			switch n.Type {
			case "retry":
				retry, err := convertRetry(n)
				if err != nil {
					return nil, err
				}
				step.Retry = retry
			case "compensation":
				comp, err := convertCompensation(n)
				if err != nil {
					return nil, err
				}
				step.Compensation = comp
			}
		}
	}

	return step, nil
}

func convertApproval(block *BlockNode) (*model.ApprovalDef, error) {
	approval := &model.ApprovalDef{
		ID: block.Name,
	}

	for _, child := range block.Body {
		assign, ok := child.(*AssignNode)
		if !ok {
			continue
		}
		switch assign.Key {
		case "depends_on":
			approval.DependsOn = nodeToStringSlice(assign.Value)
		case "when":
			approval.When = nodeToString(assign.Value)
		case "required_roles":
			approval.RequiredRoles = nodeToStringSlice(assign.Value)
		case "message":
			approval.Message = nodeToString(assign.Value)
		}
	}

	return approval, nil
}

func convertRetry(block *BlockNode) (*model.RetryPolicy, error) {
	retry := &model.RetryPolicy{}
	for _, child := range block.Body {
		assign, ok := child.(*AssignNode)
		if !ok {
			continue
		}
		switch assign.Key {
		case "max_attempts":
			retry.MaxAttempts = nodeToInt(assign.Value)
		case "backoff":
			retry.Backoff = nodeToString(assign.Value)
		case "initial_wait":
			retry.InitialWait = nodeToString(assign.Value)
		case "max_wait":
			retry.MaxWait = nodeToString(assign.Value)
		}
	}
	return retry, nil
}

func convertCompensation(block *BlockNode) (*model.CompensationDef, error) {
	comp := &model.CompensationDef{
		Config: make(map[string]any),
	}
	for _, child := range block.Body {
		assign, ok := child.(*AssignNode)
		if !ok {
			continue
		}
		switch assign.Key {
		case "runner":
			comp.Runner = nodeToString(assign.Value)
		case "command":
			comp.Command = nodeToString(assign.Value)
		default:
			comp.Config[assign.Key] = nodeToAny(assign.Value)
		}
	}
	return comp, nil
}

// Helper converters

func nodeToString(n Node) string {
	switch v := n.(type) {
	case *StringNode:
		return v.Value
	case *ExpressionNode:
		return "${" + v.Raw + "}"
	case *IdentifierNode:
		return v.Value
	case *NumberNode:
		return v.Value
	default:
		return fmt.Sprintf("%v", n)
	}
}

func nodeToBool(n Node) bool {
	if v, ok := n.(*BoolNode); ok {
		return v.Value
	}
	return false
}

func nodeToInt(n Node) int {
	if v, ok := n.(*NumberNode); ok {
		result := 0
		for _, c := range v.Value {
			if c >= '0' && c <= '9' {
				result = result*10 + int(c-'0')
			}
		}
		return result
	}
	return 0
}

func nodeToAny(n Node) any {
	switch v := n.(type) {
	case *StringNode:
		return v.Value
	case *NumberNode:
		return v.Value
	case *BoolNode:
		return v.Value
	case *NullNode:
		return nil
	case *ListNode:
		return nodeToSlice(n)
	case *ExpressionNode:
		return "${" + v.Raw + "}"
	default:
		return nil
	}
}

func nodeToSlice(n Node) []any {
	list, ok := n.(*ListNode)
	if !ok {
		return nil
	}
	var result []any
	for _, item := range list.Items {
		result = append(result, nodeToAny(item))
	}
	return result
}

func nodeToStringSlice(n Node) []string {
	list, ok := n.(*ListNode)
	if !ok {
		return nil
	}
	var result []string
	for _, item := range list.Items {
		result = append(result, nodeToString(item))
	}
	return result
}

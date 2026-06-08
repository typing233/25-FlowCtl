package huml

import (
	"fmt"
	"strings"
)

type EvalContext struct {
	Inputs  map[string]any
	Steps   map[string]map[string]any
	Secrets map[string]string
	Env     map[string]string
}

func NewEvalContext() *EvalContext {
	return &EvalContext{
		Inputs:  make(map[string]any),
		Steps:   make(map[string]map[string]any),
		Secrets: make(map[string]string),
		Env:     make(map[string]string),
	}
}

func Evaluate(expr Expr, ctx *EvalContext) (any, error) {
	switch e := expr.(type) {
	case *LiteralExpr:
		return e.Value, nil

	case *RefExpr:
		return resolveRef(e.Path, ctx)

	case *BinaryExpr:
		return evalBinary(e, ctx)

	case *UnaryExpr:
		return evalUnary(e, ctx)

	case *ConditionalExpr:
		cond, err := Evaluate(e.Cond, ctx)
		if err != nil {
			return nil, err
		}
		if toBool(cond) {
			return Evaluate(e.Then, ctx)
		}
		return Evaluate(e.Else, ctx)

	case *FuncCallExpr:
		return evalFunc(e, ctx)

	case *IndexExpr:
		obj, err := Evaluate(e.Object, ctx)
		if err != nil {
			return nil, err
		}
		idx, err := Evaluate(e.Index, ctx)
		if err != nil {
			return nil, err
		}
		return indexAccess(obj, idx)

	default:
		return nil, fmt.Errorf("unsupported expression type: %T", expr)
	}
}

func EvaluateString(raw string, ctx *EvalContext) (string, error) {
	if !strings.Contains(raw, "${") {
		return raw, nil
	}

	var result strings.Builder
	i := 0
	runes := []rune(raw)

	for i < len(runes) {
		if i+1 < len(runes) && runes[i] == '$' && runes[i+1] == '{' {
			i += 2
			depth := 1
			start := i
			for i < len(runes) && depth > 0 {
				if runes[i] == '{' {
					depth++
				} else if runes[i] == '}' {
					depth--
				}
				if depth > 0 {
					i++
				}
			}
			exprStr := string(runes[start:i])
			i++ // skip closing '}'

			expr, err := ParseExpression(exprStr)
			if err != nil {
				return "", fmt.Errorf("parse expression %q: %w", exprStr, err)
			}
			val, err := Evaluate(expr, ctx)
			if err != nil {
				return "", fmt.Errorf("evaluate expression %q: %w", exprStr, err)
			}
			result.WriteString(fmt.Sprintf("%v", val))
		} else {
			result.WriteRune(runes[i])
			i++
		}
	}

	return result.String(), nil
}

func resolveRef(path []string, ctx *EvalContext) (any, error) {
	if len(path) == 0 {
		return nil, fmt.Errorf("empty reference path")
	}

	switch path[0] {
	case "inputs":
		if len(path) < 2 {
			return ctx.Inputs, nil
		}
		val, ok := ctx.Inputs[path[1]]
		if !ok {
			return nil, fmt.Errorf("input %q not found", path[1])
		}
		return drillDown(val, path[2:])

	case "steps":
		if len(path) < 2 {
			return ctx.Steps, nil
		}
		stepData, ok := ctx.Steps[path[1]]
		if !ok {
			return nil, fmt.Errorf("step %q not found", path[1])
		}
		if len(path) < 3 {
			return stepData, nil
		}
		val, ok := stepData[path[2]]
		if !ok {
			return nil, fmt.Errorf("step %q field %q not found", path[1], path[2])
		}
		return drillDown(val, path[3:])

	case "secrets":
		if len(path) < 2 {
			return nil, fmt.Errorf("secret name required")
		}
		val, ok := ctx.Secrets[path[1]]
		if !ok {
			return nil, fmt.Errorf("secret %q not found", path[1])
		}
		return val, nil

	case "env":
		if len(path) < 2 {
			return ctx.Env, nil
		}
		return ctx.Env[path[1]], nil

	default:
		return nil, fmt.Errorf("unknown reference root %q", path[0])
	}
}

func drillDown(val any, path []string) (any, error) {
	for _, key := range path {
		m, ok := val.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("cannot access %q on non-object", key)
		}
		val, ok = m[key]
		if !ok {
			return nil, fmt.Errorf("field %q not found", key)
		}
	}
	return val, nil
}

func evalBinary(e *BinaryExpr, ctx *EvalContext) (any, error) {
	left, err := Evaluate(e.Left, ctx)
	if err != nil {
		return nil, err
	}
	right, err := Evaluate(e.Right, ctx)
	if err != nil {
		return nil, err
	}

	switch e.Op {
	case "==":
		return fmt.Sprintf("%v", left) == fmt.Sprintf("%v", right), nil
	case "!=":
		return fmt.Sprintf("%v", left) != fmt.Sprintf("%v", right), nil
	case "&&":
		return toBool(left) && toBool(right), nil
	case "||":
		return toBool(left) || toBool(right), nil
	case "<":
		return toFloat(left) < toFloat(right), nil
	case ">":
		return toFloat(left) > toFloat(right), nil
	case "<=":
		return toFloat(left) <= toFloat(right), nil
	case ">=":
		return toFloat(left) >= toFloat(right), nil
	case "+":
		lf, lok := toNumeric(left)
		rf, rok := toNumeric(right)
		if lok && rok {
			return lf + rf, nil
		}
		return fmt.Sprintf("%v%v", left, right), nil
	case "-":
		return toFloat(left) - toFloat(right), nil
	case "*":
		return toFloat(left) * toFloat(right), nil
	case "/":
		r := toFloat(right)
		if r == 0 {
			return nil, fmt.Errorf("division by zero")
		}
		return toFloat(left) / r, nil
	case "%":
		r := int64(toFloat(right))
		if r == 0 {
			return nil, fmt.Errorf("modulo by zero")
		}
		return int64(toFloat(left)) % r, nil
	default:
		return nil, fmt.Errorf("unknown operator %q", e.Op)
	}
}

func evalUnary(e *UnaryExpr, ctx *EvalContext) (any, error) {
	val, err := Evaluate(e.Operand, ctx)
	if err != nil {
		return nil, err
	}
	switch e.Op {
	case "!":
		return !toBool(val), nil
	case "-":
		return -toFloat(val), nil
	default:
		return nil, fmt.Errorf("unknown unary operator %q", e.Op)
	}
}

func evalFunc(e *FuncCallExpr, ctx *EvalContext) (any, error) {
	args := make([]any, len(e.Args))
	for i, arg := range e.Args {
		val, err := Evaluate(arg, ctx)
		if err != nil {
			return nil, err
		}
		args[i] = val
	}

	switch e.Name {
	case "len":
		if len(args) != 1 {
			return nil, fmt.Errorf("len() expects 1 argument")
		}
		return length(args[0])
	case "toJSON":
		if len(args) != 1 {
			return nil, fmt.Errorf("toJSON() expects 1 argument")
		}
		return fmt.Sprintf("%v", args[0]), nil
	case "upper":
		if len(args) != 1 {
			return nil, fmt.Errorf("upper() expects 1 argument")
		}
		return strings.ToUpper(fmt.Sprintf("%v", args[0])), nil
	case "lower":
		if len(args) != 1 {
			return nil, fmt.Errorf("lower() expects 1 argument")
		}
		return strings.ToLower(fmt.Sprintf("%v", args[0])), nil
	case "contains":
		if len(args) != 2 {
			return nil, fmt.Errorf("contains() expects 2 arguments")
		}
		return strings.Contains(fmt.Sprintf("%v", args[0]), fmt.Sprintf("%v", args[1])), nil
	case "default":
		if len(args) != 2 {
			return nil, fmt.Errorf("default() expects 2 arguments")
		}
		if args[0] == nil || fmt.Sprintf("%v", args[0]) == "" {
			return args[1], nil
		}
		return args[0], nil
	default:
		return nil, fmt.Errorf("unknown function %q", e.Name)
	}
}

func indexAccess(obj any, idx any) (any, error) {
	switch o := obj.(type) {
	case map[string]any:
		key := fmt.Sprintf("%v", idx)
		return o[key], nil
	case []any:
		i := int(toFloat(idx))
		if i < 0 || i >= len(o) {
			return nil, fmt.Errorf("index %d out of range", i)
		}
		return o[i], nil
	default:
		return nil, fmt.Errorf("cannot index %T", obj)
	}
}

func toBool(v any) bool {
	switch val := v.(type) {
	case bool:
		return val
	case nil:
		return false
	case string:
		return val != ""
	case float64:
		return val != 0
	case int:
		return val != 0
	case int64:
		return val != 0
	default:
		return true
	}
}

func toFloat(v any) float64 {
	switch val := v.(type) {
	case float64:
		return val
	case int:
		return float64(val)
	case int64:
		return float64(val)
	case string:
		f := 0.0
		fmt.Sscanf(val, "%f", &f)
		return f
	case bool:
		if val {
			return 1
		}
		return 0
	default:
		return 0
	}
}

func toNumeric(v any) (float64, bool) {
	switch val := v.(type) {
	case float64:
		return val, true
	case int:
		return float64(val), true
	case int64:
		return float64(val), true
	default:
		return 0, false
	}
}

func length(v any) (int, error) {
	switch val := v.(type) {
	case string:
		return len(val), nil
	case []any:
		return len(val), nil
	case map[string]any:
		return len(val), nil
	default:
		return 0, fmt.Errorf("len() not supported for %T", v)
	}
}

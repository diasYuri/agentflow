package workflow

import (
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"strings"

	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/vm"
)

var templateExpr = regexp.MustCompile(`\$\{([^}]+)\}`)

type NodeView struct {
	Status   string
	Output   any
	Outputs  []any
	Stdout   string
	Stderr   string
	ExitCode *int
	Error    string
	Path     []string
}

type EvalContext struct {
	Inputs  map[string]any
	Vars    map[string]any
	Secrets map[string]any
	Nodes   map[string]any
	Item    any
	Index   *int
	Total   *int
	Run     map[string]any
}

func RenderTemplate(tmpl string, ctx EvalContext) (string, error) {
	var firstErr error
	out := templateExpr.ReplaceAllStringFunc(tmpl, func(match string) string {
		if firstErr != nil {
			return ""
		}
		inner := strings.TrimSpace(match[2 : len(match)-1])
		value, err := EvalValue(inner, ctx)
		if err != nil {
			firstErr = fmt.Errorf("template expression %q: %w", inner, err)
			return ""
		}
		return stringify(value)
	})
	if firstErr != nil {
		return "", firstErr
	}
	return out, nil
}

func EvalTemplateValue(input string, ctx EvalContext) (any, error) {
	trimmed := strings.TrimSpace(input)
	if matches := templateExpr.FindStringSubmatch(trimmed); len(matches) == 2 && matches[0] == trimmed {
		expression := strings.TrimSpace(matches[1])
		value, err := EvalValue(expression, ctx)
		if err != nil {
			return nil, fmt.Errorf("template expression %q: %w", expression, err)
		}
		return value, nil
	}
	return RenderTemplate(input, ctx)
}

func EvalBool(input string, ctx EvalContext) (bool, error) {
	if strings.TrimSpace(input) == "" {
		return true, nil
	}
	expression := templateExpr.ReplaceAllString(input, `($1)`)
	value, err := EvalValue(expression, ctx)
	if err != nil {
		return false, fmt.Errorf("expression %q: %w", input, err)
	}
	boolValue, ok := value.(bool)
	if !ok {
		return false, fmt.Errorf("expression %q returned %T, want bool", input, value)
	}
	return boolValue, nil
}

func EvalValue(expression string, ctx EvalContext) (any, error) {
	program, err := compile(expression, ctx)
	if err != nil {
		return nil, err
	}
	value, err := vm.Run(program, env(ctx))
	if err != nil {
		return nil, err
	}
	return value, nil
}

func compile(expression string, ctx EvalContext) (*vm.Program, error) {
	return expr.Compile(expression, expr.Env(env(ctx)), expr.AllowUndefinedVariables())
}

func env(ctx EvalContext) map[string]any {
	env := map[string]any{
		"inputs":  ctx.Inputs,
		"vars":    ctx.Vars,
		"secrets": ctx.Secrets,
		"nodes":   ctx.Nodes,
		"item":    ctx.Item,
		"index":   ctx.Index,
		"total":   ctx.Total,
		"run":     ctx.Run,
	}
	env["exists"] = func(v any) bool { return v != nil }
	env["success"] = func(id string) bool {
		node, ok := ctx.Nodes[id].(map[string]any)
		return ok && node["status"] == "success"
	}
	env["failed"] = func(id string) bool {
		node, ok := ctx.Nodes[id].(map[string]any)
		return ok && node["status"] == "failed"
	}
	env["contains"] = contains
	env["len"] = length
	env["default"] = defaultValue
	env["json"] = jsonValue
	return env
}

func defaultValue(value any, fallback any) any {
	if value == nil {
		return fallback
	}
	if s, ok := value.(string); ok && s == "" {
		return fallback
	}
	return value
}

func jsonValue(value any) (any, error) {
	s, ok := value.(string)
	if !ok {
		return nil, fmt.Errorf("json() requires a string argument, got %T", value)
	}
	var result any
	if err := json.Unmarshal([]byte(s), &result); err != nil {
		return nil, fmt.Errorf("json() parse error: %w", err)
	}
	return result, nil
}

func contains(container any, needle any) bool {
	switch typed := container.(type) {
	case string:
		return strings.Contains(typed, stringify(needle))
	case []any:
		for _, item := range typed {
			if reflect.DeepEqual(item, needle) {
				return true
			}
		}
	}
	value := reflect.ValueOf(container)
	if value.IsValid() && value.Kind() == reflect.Slice {
		for i := 0; i < value.Len(); i++ {
			if reflect.DeepEqual(value.Index(i).Interface(), needle) {
				return true
			}
		}
	}
	return false
}

func length(v any) int {
	if v == nil {
		return 0
	}
	switch typed := v.(type) {
	case string:
		return len(typed)
	case []any:
		return len(typed)
	case map[string]any:
		return len(typed)
	}
	value := reflect.ValueOf(v)
	if value.IsValid() {
		switch value.Kind() {
		case reflect.Array, reflect.Chan, reflect.Map, reflect.Slice, reflect.String:
			return value.Len()
		}
	}
	return 0
}

func stringify(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return typed
	case fmt.Stringer:
		return typed.String()
	default:
		data, err := json.Marshal(typed)
		if err == nil {
			return string(data)
		}
		return fmt.Sprint(typed)
	}
}

func ToAnySlice(value any) ([]any, error) {
	if value == nil {
		return nil, fmt.Errorf("value is nil, want array")
	}
	if items, ok := value.([]any); ok {
		return items, nil
	}
	rv := reflect.ValueOf(value)
	if rv.Kind() != reflect.Slice && rv.Kind() != reflect.Array {
		return nil, fmt.Errorf("value has type %T, want array", value)
	}
	items := make([]any, rv.Len())
	for i := 0; i < rv.Len(); i++ {
		items[i] = rv.Index(i).Interface()
	}
	return items, nil
}

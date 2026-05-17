package workflow

import "testing"

func TestEvalTemplateValueReturnsTypedValue(t *testing.T) {
	ctx := EvalContext{Inputs: map[string]any{"files": []any{"a.go", "b.go"}}}
	value, err := EvalTemplateValue("${inputs.files}", ctx)
	if err != nil {
		t.Fatal(err)
	}
	items, err := ToAnySlice(value)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
}

func TestEvalBoolSupportsNodeOutputReferences(t *testing.T) {
	exitCode := 1
	ctx := EvalContext{Nodes: map[string]any{
		"test": map[string]any{"status": "failed", "exit_code": &exitCode},
	}}
	ok, err := EvalBool("${nodes.test.exit_code} != 0 && failed('test')", ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected expression to be true")
	}
}

func TestEvalValueDefaultHelper(t *testing.T) {
	ctx := EvalContext{Inputs: map[string]any{"name": "world"}}
	v, err := EvalValue("default(inputs.name, 'fallback')", ctx)
	if err != nil {
		t.Fatal(err)
	}
	if v != "world" {
		t.Fatalf("expected world, got %v", v)
	}

	ctx = EvalContext{Inputs: map[string]any{"name": nil}}
	v, err = EvalValue("default(inputs.name, 'fallback')", ctx)
	if err != nil {
		t.Fatal(err)
	}
	if v != "fallback" {
		t.Fatalf("expected fallback, got %v", v)
	}

	ctx = EvalContext{Inputs: map[string]any{"name": ""}}
	v, err = EvalValue("default(inputs.name, 'fallback')", ctx)
	if err != nil {
		t.Fatal(err)
	}
	if v != "fallback" {
		t.Fatalf("expected fallback for empty string, got %v", v)
	}
}

func TestEvalValueMatchesOperator(t *testing.T) {
	ctx := EvalContext{Inputs: map[string]any{"email": "test@example.com"}}
	v, err := EvalValue(`inputs.email matches "^[a-z]+@example\\.com$"`, ctx)
	if err != nil {
		t.Fatal(err)
	}
	if v != true {
		t.Fatalf("expected true, got %v", v)
	}

	ctx = EvalContext{Inputs: map[string]any{"email": "invalid"}}
	v, err = EvalValue(`inputs.email matches "^[a-z]+@example\\.com$"`, ctx)
	if err != nil {
		t.Fatal(err)
	}
	if v != false {
		t.Fatalf("expected false, got %v", v)
	}
}

func TestEvalValueJsonHelper(t *testing.T) {
	ctx := EvalContext{Inputs: map[string]any{"raw": `{"count": 42}`}}
	v, err := EvalValue("json(inputs.raw).count", ctx)
	if err != nil {
		t.Fatal(err)
	}
	if v != float64(42) {
		t.Fatalf("expected 42, got %v", v)
	}
}

func TestEvalValueJsonHelperRequiresString(t *testing.T) {
	ctx := EvalContext{Inputs: map[string]any{"raw": 42}}
	_, err := EvalValue("json(inputs.raw)", ctx)
	if err == nil {
		t.Fatal("expected error for non-string json input")
	}
}

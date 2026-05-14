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

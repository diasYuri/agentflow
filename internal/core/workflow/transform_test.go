package workflow

import "testing"

func TestApplyTransformFlatMapFlattensNestedArrays(t *testing.T) {
	out, err := ApplyTransform("flat_map", []any{
		[]any{"a", "b"},
		[]any{"c"},
		"d",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := out.([]any)
	if !ok {
		t.Fatalf("expected []any, got %T", out)
	}
	if len(got) != 4 || got[0] != "a" || got[1] != "b" || got[2] != "c" || got[3] != "d" {
		t.Fatalf("unexpected result: %#v", got)
	}
}

func TestApplyTransformFlatMapUsesPath(t *testing.T) {
	out, err := ApplyTransform("flat_map", []any{
		map[string]any{"items": []any{"a", "b"}},
		map[string]any{"items": []any{"c"}},
	}, map[string]any{"path": "items"})
	if err != nil {
		t.Fatal(err)
	}
	got, ok := out.([]any)
	if !ok {
		t.Fatalf("expected []any, got %T", out)
	}
	if len(got) != 3 || got[0] != "a" || got[1] != "b" || got[2] != "c" {
		t.Fatalf("unexpected result: %#v", got)
	}
}

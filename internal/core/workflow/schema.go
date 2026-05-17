package workflow

import (
	"fmt"
	"reflect"
	"strings"
)

// ValidateSchema validates a value against a conservative JSON Schema subset.
// Supported fields: type, required, properties, items, enum.
func ValidateSchema(value any, schema map[string]any, path string) error {
	if len(schema) == 0 {
		return nil
	}
	if value == nil {
		return nil
	}

	if enum, ok := schema["enum"].([]any); ok {
		found := false
		for _, allowed := range enum {
			if reflect.DeepEqual(value, allowed) {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("%s: value %v not in enum %v", path, value, enum)
		}
	}

	schemaType, _ := schema["type"].(string)
	if schemaType != "" {
		if err := validateSchemaType(value, schemaType, path); err != nil {
			return err
		}
	}

	switch schemaType {
	case "object":
		if err := validateObjectSchema(value, schema, path); err != nil {
			return err
		}
	case "array":
		if err := validateArraySchema(value, schema, path); err != nil {
			return err
		}
	}

	return nil
}

func validateSchemaType(value any, want string, path string) error {
	if value == nil {
		return nil
	}
	ok := false
	switch want {
	case "string":
		_, ok = value.(string)
	case "integer":
		ok = isInteger(value)
	case "number":
		ok = isNumber(value)
	case "boolean":
		_, ok = value.(bool)
	case "array":
		ok = hasKind(value, reflect.Array, reflect.Slice)
	case "object":
		ok = hasKind(value, reflect.Map, reflect.Struct)
	default:
		return fmt.Errorf("%s: unsupported schema type %q", path, want)
	}
	if !ok {
		return fmt.Errorf("%s: got %T, want %s", path, value, want)
	}
	return nil
}

func validateObjectSchema(value any, schema map[string]any, path string) error {
	m, ok := value.(map[string]any)
	if !ok {
		return nil
	}

	if required, ok := schema["required"].([]any); ok {
		for _, item := range required {
			key, ok := item.(string)
			if !ok {
				return fmt.Errorf("%s: required must contain strings", path)
			}
			if _, exists := m[key]; !exists {
				return fmt.Errorf("%s: missing required property %q", path, key)
			}
		}
	}

	if props, ok := schema["properties"].(map[string]any); ok {
		for key, propSchema := range props {
			propMap, ok := propSchema.(map[string]any)
			if !ok {
				return fmt.Errorf("%s: properties.%s must be an object", path, key)
			}
			if propValue, exists := m[key]; exists {
				if err := ValidateSchema(propValue, propMap, path+"."+key); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

func validateArraySchema(value any, schema map[string]any, path string) error {
	items, err := ToAnySlice(value)
	if err != nil {
		return nil
	}

	if itemSchema, ok := schema["items"].(map[string]any); ok {
		for i, item := range items {
			if err := ValidateSchema(item, itemSchema, fmt.Sprintf("%s[%d]", path, i)); err != nil {
				return err
			}
		}
	}

	return nil
}

// ValidateSchemaDecl validates a schema declaration itself (not a value against it).
func ValidateSchemaDecl(schema map[string]any, path string) error {
	if len(schema) == 0 {
		return nil
	}

	schemaType, _ := schema["type"].(string)
	if schemaType != "" {
		switch schemaType {
		case "string", "integer", "number", "boolean", "array", "object":
		default:
			return fmt.Errorf("%s: unsupported schema type %q", path, schemaType)
		}
	}

	if required, ok := schema["required"].([]any); ok {
		if schemaType != "" && schemaType != "object" {
			return fmt.Errorf("%s: required is only valid for object type", path)
		}
		for _, item := range required {
			if _, ok := item.(string); !ok {
				return fmt.Errorf("%s: required must be an array of strings", path)
			}
		}
	}

	if props, ok := schema["properties"].(map[string]any); ok {
		if schemaType != "" && schemaType != "object" {
			return fmt.Errorf("%s: properties is only valid for object type", path)
		}
		for key, val := range props {
			propMap, ok := val.(map[string]any)
			if !ok {
				return fmt.Errorf("%s: properties.%s must be a schema object", path, key)
			}
			if err := ValidateSchemaDecl(propMap, path+".properties."+key); err != nil {
				return err
			}
		}
	}

	if items, ok := schema["items"].(map[string]any); ok {
		if schemaType != "" && schemaType != "array" {
			return fmt.Errorf("%s: items is only valid for array type", path)
		}
		if err := ValidateSchemaDecl(items, path+".items"); err != nil {
			return err
		}
	}

	if enum, ok := schema["enum"].([]any); ok {
		if len(enum) == 0 {
			return fmt.Errorf("%s: enum must not be empty", path)
		}
	}

	return nil
}

// CoerceAndValidateInputValue validates a value against either a schema or a legacy type.
func CoerceAndValidateInputValue(value any, inputType string, schema map[string]any, name string) error {
	if value == nil {
		return nil
	}
	if len(schema) > 0 {
		if inputType != "" {
			if err := validateSchemaType(value, inputType, "inputs."+name); err == nil {
				if err := ValidateSchema(value, schema, "inputs."+name); err != nil {
					return err
				}
				return nil
			}
			return fmt.Errorf("inputs.%s: type %q conflicts with schema", name, inputType)
		}
		return ValidateSchema(value, schema, "inputs."+name)
	}
	if inputType != "" {
		return validateInputValue(inputType, value)
	}
	return nil
}

func isValidOutputScopeRef(ref string) bool {
	// Allow references to inputs.*, vars.*, nodes.*.output, nodes.*.outputs, run.*
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return false
	}
	parts := strings.Split(ref, ".")
	if len(parts) < 2 {
		return false
	}
	switch parts[0] {
	case "inputs", "vars", "run":
		return true
	case "nodes":
		if len(parts) >= 3 {
			switch parts[2] {
			case "output", "outputs", "status", "stdout", "stderr", "exit_code", "error":
				return true
			}
		}
	}
	return false
}

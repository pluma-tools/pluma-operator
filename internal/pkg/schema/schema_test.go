package schema

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"helm.sh/helm/v3/pkg/chart"
)

func TestFilter_FilterValues(t *testing.T) {
	tests := []struct {
		name     string
		schema   *JSONSchema
		input    map[string]interface{}
		expected map[string]interface{}
	}{
		{
			name: "basic field filtering",
			schema: &JSONSchema{
				Type: "object",
				Properties: map[string]*JSONSchema{
					"autoscaling": {
						Type: "object",
						Properties: map[string]*JSONSchema{
							"enabled":     {Type: "boolean"},
							"minReplicas": {Type: "integer"},
						},
					},
					"resources": {
						Type: "object",
						Properties: map[string]*JSONSchema{
							"limits":   {Type: "object"},
							"requests": {Type: "object"},
						},
					},
				},
				AdditionalProperties: false,
			},
			input: map[string]interface{}{
				"autoscaling": map[string]interface{}{
					"enabled":     true,
					"minReplicas": 1,
				},
				"resources": map[string]interface{}{
					"limits": map[string]interface{}{
						"cpu":    "1",
						"memory": "900Mi",
					},
					"requests": map[string]interface{}{
						"cpu":    "50m",
						"memory": "50Mi",
					},
				},
				// These should be filtered out
				"meshConfig": map[string]interface{}{
					"defaultConfig": map[string]interface{}{
						"extraStatTags": []string{"destination_mesh_id"},
					},
				},
				"pilot": map[string]interface{}{
					"autoscaleEnabled": true,
				},
			},
			expected: map[string]interface{}{
				"autoscaling": map[string]interface{}{
					"enabled":     true,
					"minReplicas": 1,
				},
				"resources": map[string]interface{}{
					"limits": map[string]interface{}{
						"cpu":    "1",
						"memory": "900Mi",
					},
					"requests": map[string]interface{}{
						"cpu":    "50m",
						"memory": "50Mi",
					},
				},
			},
		},
		{
			name: "allow additional properties",
			schema: &JSONSchema{
				Type: "object",
				Properties: map[string]*JSONSchema{
					"autoscaling": {Type: "object"},
				},
				AdditionalProperties: true,
			},
			input: map[string]interface{}{
				"autoscaling": map[string]interface{}{
					"enabled": true,
				},
				"customField": "customValue",
			},
			expected: map[string]interface{}{
				"autoscaling": map[string]interface{}{
					"enabled": true,
				},
				"customField": "customValue",
			},
		},
		{
			name: "nested object filtering",
			schema: &JSONSchema{
				Type: "object",
				Properties: map[string]*JSONSchema{
					"global": {
						Type: "object",
						Properties: map[string]*JSONSchema{
							"hub":            {Type: "string"},
							"istioNamespace": {Type: "string"},
						},
					},
				},
			},
			input: map[string]interface{}{
				"global": map[string]interface{}{
					"hub":            "release-ci.daocloud.io/mspider",
					"istioNamespace": "istio-system",
					"meshID":         "jxj31-mspider-tg", // Should be filtered out
				},
				"pilot": map[string]interface{}{ // Should be filtered out
					"autoscaleEnabled": true,
				},
			},
			expected: map[string]interface{}{
				"global": map[string]interface{}{
					"hub":            "release-ci.daocloud.io/mspider",
					"istioNamespace": "istio-system",
				},
			},
		},
		{
			name: "array filtering",
			schema: &JSONSchema{
				Type: "object",
				Properties: map[string]*JSONSchema{
					"tags": {
						Type:  "array",
						Items: &JSONSchema{Type: "string"},
					},
				},
			},
			input: map[string]interface{}{
				"tags":  []interface{}{"tag1", "tag2", "tag3"},
				"other": "value", // Should be filtered out
			},
			expected: map[string]interface{}{
				"tags": []interface{}{"tag1", "tag2", "tag3"},
			},
		},
		{
			name:     "nil schema returns original values",
			schema:   nil,
			input:    map[string]interface{}{"key": "value"},
			expected: map[string]interface{}{"key": "value"},
		},
		{
			name: "empty schema properties",
			schema: &JSONSchema{
				Type:       "object",
				Properties: map[string]*JSONSchema{},
			},
			input:    map[string]interface{}{"key": "value"},
			expected: map[string]interface{}{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filter := NewFilter(tt.schema)
			result := filter.FilterValues(tt.input)

			if !mapsEqual(result, tt.expected) {
				t.Errorf("FilterValues() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestFilter_filterObject(t *testing.T) {
	tests := []struct {
		name     string
		schema   *JSONSchema
		input    map[string]interface{}
		expected map[string]interface{}
	}{
		{
			name: "filter out undefined properties",
			schema: &JSONSchema{
				Type: "object",
				Properties: map[string]*JSONSchema{
					"allowed": {Type: "string"},
				},
				AdditionalProperties: false,
			},
			input: map[string]interface{}{
				"allowed":   "value",
				"forbidden": "value",
			},
			expected: map[string]interface{}{
				"allowed": "value",
			},
		},
		{
			name: "nil schema properties",
			schema: &JSONSchema{
				Type:       "object",
				Properties: nil,
			},
			input:    map[string]interface{}{"key": "value"},
			expected: map[string]interface{}{"key": "value"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filter := &Filter{schema: tt.schema}
			result := filter.filterObject(tt.input, tt.schema)

			if !mapsEqual(result, tt.expected) {
				t.Errorf("filterObject() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestFilter_filterValue(t *testing.T) {
	tests := []struct {
		name     string
		schema   *JSONSchema
		input    interface{}
		expected interface{}
	}{
		{
			name: "object filtering",
			schema: &JSONSchema{
				Type: "object",
				Properties: map[string]*JSONSchema{
					"nested": {Type: "string"},
				},
			},
			input: map[string]interface{}{
				"nested": "value",
				"other":  "value",
			},
			expected: map[string]interface{}{
				"nested": "value",
			},
		},
		{
			name: "array filtering",
			schema: &JSONSchema{
				Type:  "array",
				Items: &JSONSchema{Type: "string"},
			},
			input:    []interface{}{"item1", "item2"},
			expected: []interface{}{"item1", "item2"},
		},
		{
			name:     "primitive types pass through",
			schema:   &JSONSchema{Type: "string"},
			input:    "test",
			expected: "test",
		},
		{
			name:     "nil schema returns original value",
			schema:   nil,
			input:    "test",
			expected: "test",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filter := &Filter{schema: tt.schema}
			result := filter.filterValue(tt.input, tt.schema)

			if !valuesEqual(result, tt.expected) {
				t.Errorf("filterValue() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestFilter_filterArray(t *testing.T) {
	tests := []struct {
		name     string
		schema   *JSONSchema
		input    []interface{}
		expected []interface{}
	}{
		{
			name: "array with items schema",
			schema: &JSONSchema{
				Type: "array",
				Items: &JSONSchema{
					Type: "object",
					Properties: map[string]*JSONSchema{
						"name": {Type: "string"},
					},
				},
			},
			input: []interface{}{
				map[string]interface{}{
					"name": "item1",
					"id":   "1", // Should be filtered out
				},
				map[string]interface{}{
					"name": "item2",
					"id":   "2", // Should be filtered out
				},
			},
			expected: []interface{}{
				map[string]interface{}{
					"name": "item1",
				},
				map[string]interface{}{
					"name": "item2",
				},
			},
		},
		{
			name: "nil items schema",
			schema: &JSONSchema{
				Type:  "array",
				Items: nil,
			},
			input:    []interface{}{"item1", "item2"},
			expected: []interface{}{"item1", "item2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filter := &Filter{schema: tt.schema}
			result := filter.filterArray(tt.input, tt.schema)

			if !slicesEqual(result, tt.expected) {
				t.Errorf("filterArray() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestLoadSchemaFromChart(t *testing.T) {
	tests := []struct {
		name        string
		chart       *chart.Chart
		expectError bool
	}{
		{
			name:        "chart without schema",
			chart:       &chart.Chart{Raw: []*chart.File{}},
			expectError: true,
		},
		{
			name: "chart with schema",
			chart: &chart.Chart{
				Raw: []*chart.File{
					{
						Name: "values.schema.json",
						Data: []byte(`{"type": "object", "properties": {"test": {"type": "string"}}}`),
					},
				},
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := LoadSchemaFromChart(tt.chart)
			if (err != nil) != tt.expectError {
				t.Errorf("LoadSchemaFromChart() error = %v, expectError %v", err, tt.expectError)
			}
		})
	}
}

func TestResolveSchemaReferences(t *testing.T) {
	tests := []struct {
		name     string
		schema   *JSONSchema
		expected *JSONSchema
	}{
		{
			name: "resolve $ref to $defs/values",
			schema: &JSONSchema{
				Ref: "#/$defs/values",
				Defs: map[string]*JSONSchema{
					"values": {
						Type: "object",
						Properties: map[string]*JSONSchema{
							"autoscaling": {
								Type: "object",
								Properties: map[string]*JSONSchema{
									"enabled": {Type: "boolean"},
								},
							},
						},
					},
				},
			},
			expected: &JSONSchema{
				Type: "object",
				Properties: map[string]*JSONSchema{
					"autoscaling": {
						Type: "object",
						Properties: map[string]*JSONSchema{
							"enabled": {Type: "boolean"},
						},
					},
				},
			},
		},
		{
			name: "schema without $ref",
			schema: &JSONSchema{
				Type: "object",
				Properties: map[string]*JSONSchema{
					"test": {Type: "string"},
				},
			},
			expected: &JSONSchema{
				Type: "object",
				Properties: map[string]*JSONSchema{
					"test": {Type: "string"},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ResolveSchemaReferences(tt.schema)
			if err != nil {
				t.Errorf("ResolveSchemaReferences() error = %v", err)
				return
			}

			if !schemasEqual(result, tt.expected) {
				t.Errorf("resolveSchemaReferences() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestFilterWithResolvedSchema(t *testing.T) {
	// Test with a schema that has $ref like the real Istio schema
	schema := &JSONSchema{
		Ref: "#/$defs/values",
		Defs: map[string]*JSONSchema{
			"values": {
				Type: "object",
				Properties: map[string]*JSONSchema{
					"autoscaling": {
						Type: "object",
						Properties: map[string]*JSONSchema{
							"enabled":     {Type: "boolean"},
							"minReplicas": {Type: "integer"},
						},
					},
					"global": {
						Type: "object",
						Properties: map[string]*JSONSchema{
							"hub": {Type: "string"},
						},
					},
				},
				AdditionalProperties: false,
			},
		},
	}

	// Resolve the schema
	resolvedSchema, err := ResolveSchemaReferences(schema)
	if err != nil {
		t.Fatalf("Failed to resolve schema: %v", err)
	}

	// Test filtering
	filter := NewFilter(resolvedSchema)
	input := map[string]interface{}{
		"autoscaling": map[string]interface{}{
			"enabled":     true,
			"minReplicas": 1,
		},
		"global": map[string]interface{}{
			"hub": "test-hub",
		},
		"meshConfig": map[string]interface{}{
			"defaultConfig": map[string]interface{}{
				"extraStatTags": []string{"destination_mesh_id"},
			},
		},
	}

	expected := map[string]interface{}{
		"autoscaling": map[string]interface{}{
			"enabled":     true,
			"minReplicas": 1,
		},
		"global": map[string]interface{}{
			"hub": "test-hub",
		},
	}

	result := filter.FilterValues(input)
	if !mapsEqual(result, expected) {
		t.Errorf("FilterValues() with resolved schema = %v, want %v", result, expected)
	}
}

func TestFilterWithTypeArray(t *testing.T) {
	// Test with a schema that has type arrays like ["object", "null"]
	schema := &JSONSchema{
		Type: "object",
		Properties: map[string]*JSONSchema{
			"securityContext": {
				Type: []interface{}{"object", "null"},
				Properties: map[string]*JSONSchema{
					"runAsUser": {Type: "integer"},
				},
			},
			"containerSecurityContext": {
				Type: []interface{}{"object", "null"},
				Properties: map[string]*JSONSchema{
					"runAsNonRoot": {Type: "boolean"},
				},
			},
			"minReadySeconds": {
				Type: []interface{}{"null", "integer"},
			},
		},
		AdditionalProperties: false,
	}

	filter := NewFilter(schema)
	input := map[string]interface{}{
		"securityContext": map[string]interface{}{
			"runAsUser": 1000,
		},
		"containerSecurityContext": map[string]interface{}{
			"runAsNonRoot": true,
		},
		"minReadySeconds": 30,
		"unknownField":    "should be filtered out",
	}

	expected := map[string]interface{}{
		"securityContext": map[string]interface{}{
			"runAsUser": 1000,
		},
		"containerSecurityContext": map[string]interface{}{
			"runAsNonRoot": true,
		},
		"minReadySeconds": 30,
	}

	result := filter.FilterValues(input)
	if !mapsEqual(result, expected) {
		t.Errorf("FilterValues() with type arrays = %v, want %v", result, expected)
	}
}

func TestFilterWithRealIstioSchema(t *testing.T) {
	// Test with a schema structure similar to the real Istio schema you provided
	istioSchemaJSON := `{
		"$schema": "http://json-schema.org/schema#",
		"$defs": {
			"values": {
				"type": "object",
				"additionalProperties": false,
				"properties": {
					"autoscaling": {
						"type": "object",
						"properties": {
							"enabled": {
								"type": "boolean"
							},
							"maxReplicas": {
								"type": "integer"
							},
							"minReplicas": {
								"type": "integer"
							}
						}
					},
					"global": {
						"type": "object",
						"properties": {
							"hub": {
								"type": "string"
							},
							"istioNamespace": {
								"type": "string"
							}
						}
					},
					"pilot": {
						"type": "object"
					}
				}
			}
		},
		"$ref": "#/$defs/values"
	}`

	var schema JSONSchema
	if err := json.Unmarshal([]byte(istioSchemaJSON), &schema); err != nil {
		t.Fatalf("Failed to unmarshal schema: %v", err)
	}

	// Resolve the schema
	resolvedSchema, err := ResolveSchemaReferences(&schema)
	if err != nil {
		t.Fatalf("Failed to resolve schema: %v", err)
	}

	// Verify that the schema was resolved correctly
	if resolvedSchema.Type != "object" {
		t.Errorf("Expected resolved schema type to be 'object', got '%s'", resolvedSchema.Type)
	}

	if resolvedSchema.Properties == nil {
		t.Error("Expected resolved schema to have properties")
	}

	// Check that specific properties exist
	if _, exists := resolvedSchema.Properties["autoscaling"]; !exists {
		t.Error("Expected 'autoscaling' property in resolved schema")
	}

	if _, exists := resolvedSchema.Properties["global"]; !exists {
		t.Error("Expected 'global' property in resolved schema")
	}

	// Test filtering with the resolved schema
	filter := NewFilter(resolvedSchema)
	input := map[string]interface{}{
		"autoscaling": map[string]interface{}{
			"enabled":     true,
			"minReplicas": 1,
			"maxReplicas": 10,
		},
		"global": map[string]interface{}{
			"hub":            "test-hub",
			"istioNamespace": "istio-system",
		},
		"pilot": map[string]interface{}{
			"autoscaleEnabled": true,
		},
		// This should be filtered out
		"meshConfig": map[string]interface{}{
			"defaultConfig": map[string]interface{}{
				"extraStatTags": []string{"destination_mesh_id"},
			},
		},
	}

	result := filter.FilterValues(input)

	// Verify that only schema-defined properties are kept
	if len(result) != 3 {
		t.Errorf("Expected 3 properties in result, got %d: %v", len(result), result)
	}

	// Verify specific properties
	if _, exists := result["autoscaling"]; !exists {
		t.Error("Expected 'autoscaling' in result")
	}

	if _, exists := result["global"]; !exists {
		t.Error("Expected 'global' in result")
	}

	if _, exists := result["pilot"]; !exists {
		t.Error("Expected 'pilot' in result")
	}

	// Verify that meshConfig was filtered out
	if _, exists := result["meshConfig"]; exists {
		t.Error("Expected 'meshConfig' to be filtered out")
	}
}

// Helper functions for deep comparison

func mapsEqual(a, b map[string]interface{}) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if !valuesEqual(v, b[k]) {
			return false
		}
	}
	return true
}

func valuesEqual(a, b interface{}) bool {
	switch va := a.(type) {
	case map[string]interface{}:
		if vb, ok := b.(map[string]interface{}); ok {
			return mapsEqual(va, vb)
		}
		return false
	case []interface{}:
		if vb, ok := b.([]interface{}); ok {
			return slicesEqual(va, vb)
		}
		return false
	default:
		return a == b
	}
}

func slicesEqual(a, b []interface{}) bool {
	if len(a) != len(b) {
		return false
	}
	for i, v := range a {
		if !valuesEqual(v, b[i]) {
			return false
		}
	}
	return true
}

func schemasEqual(a, b *JSONSchema) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}

	if !typesEqual(a.Type, b.Type) || a.Ref != b.Ref {
		return false
	}

	// Compare properties
	if len(a.Properties) != len(b.Properties) {
		return false
	}
	for k, v := range a.Properties {
		if !schemasEqual(v, b.Properties[k]) {
			return false
		}
	}

	// Compare items
	if !schemasEqual(a.Items, b.Items) {
		return false
	}

	// Compare additionalProperties (simplified comparison)
	if a.AdditionalProperties != b.AdditionalProperties {
		return false
	}

	return true
}

func typesEqual(a, b interface{}) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}

	// Convert both to strings for comparison
	aStr := typeToString(a)
	bStr := typeToString(b)
	return aStr == bStr
}

func typeToString(t interface{}) string {
	switch v := t.(type) {
	case string:
		return v
	case []interface{}:
		// Convert array to string representation
		var strs []string
		for _, item := range v {
			if str, ok := item.(string); ok {
				strs = append(strs, str)
			}
		}
		return fmt.Sprintf("[%s]", strings.Join(strs, ","))
	default:
		return fmt.Sprintf("%v", v)
	}
}

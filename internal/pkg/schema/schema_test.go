package schema

import (
	"testing"
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
		chartPath   string
		expectError bool
	}{
		{
			name:        "non-existent chart path",
			chartPath:   "/non/existent/path",
			expectError: true,
		},
		{
			name:        "empty chart path",
			chartPath:   "",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := LoadSchemaFromChart(tt.chartPath)
			if (err != nil) != tt.expectError {
				t.Errorf("LoadSchemaFromChart() error = %v, expectError %v", err, tt.expectError)
			}
		})
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

package schema

import (
	"encoding/json"
	"fmt"

	"helm.sh/helm/v3/pkg/chart/loader"
)

// JSONSchema represents a simplified JSON Schema structure for field filtering
type JSONSchema struct {
	Type                 string                 `json:"type,omitempty"`
	Properties           map[string]*JSONSchema `json:"properties,omitempty"`
	AdditionalProperties interface{}            `json:"additionalProperties,omitempty"`
	Items                *JSONSchema            `json:"items,omitempty"`
}

// Filter handles JSON schema field filtering
type Filter struct {
	schema *JSONSchema
}

// NewFilter creates a new schema filter
func NewFilter(schema *JSONSchema) *Filter {
	return &Filter{schema: schema}
}

// LoadSchemaFromChart loads values.schema.json from a Helm chart
func LoadSchemaFromChart(chartPath string) (*JSONSchema, error) {
	// Load the chart
	chart, err := loader.Load(chartPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load chart: %w", err)
	}

	// Look for values.schema.json in the chart files
	for _, file := range chart.Files {
		if file.Name == "values.schema.json" {
			var schema JSONSchema
			if err := json.Unmarshal(file.Data, &schema); err != nil {
				return nil, fmt.Errorf("failed to parse values.schema.json: %w", err)
			}
			return &schema, nil
		}
	}

	return nil, fmt.Errorf("values.schema.json not found in chart")
}

// FilterValues filters values based on the JSON schema (only field filtering)
func (f *Filter) FilterValues(values map[string]interface{}) map[string]interface{} {
	if f.schema == nil {
		return values
	}

	return f.filterObject(values, f.schema)
}

// filterObject recursively filters an object based on schema
func (f *Filter) filterObject(obj map[string]interface{}, schema *JSONSchema) map[string]interface{} {
	if schema == nil || schema.Properties == nil {
		return obj
	}

	filtered := make(map[string]interface{})

	for key, value := range obj {
		// Check if the property is defined in the schema
		if propSchema, exists := schema.Properties[key]; exists {
			// Recursively filter the value based on its schema
			filteredValue := f.filterValue(value, propSchema)
			if filteredValue != nil {
				filtered[key] = filteredValue
			}
		} else {
			// Property not in schema, check additionalProperties
			if schema.AdditionalProperties != nil {
				if allowAdditional, ok := schema.AdditionalProperties.(bool); ok && allowAdditional {
					filtered[key] = value
				} else if additionalSchema, ok := schema.AdditionalProperties.(map[string]interface{}); ok {
					// Additional properties schema
					additionalJSONSchema := &JSONSchema{}
					if data, err := json.Marshal(additionalSchema); err == nil {
						json.Unmarshal(data, additionalJSONSchema)
						filteredValue := f.filterValue(value, additionalJSONSchema)
						if filteredValue != nil {
							filtered[key] = filteredValue
						}
					}
				}
			}
		}
	}

	return filtered
}

// filterValue filters a value based on its schema
func (f *Filter) filterValue(value interface{}, schema *JSONSchema) interface{} {
	if schema == nil {
		return value
	}

	switch schema.Type {
	case "object":
		if obj, ok := value.(map[string]interface{}); ok {
			return f.filterObject(obj, schema)
		}
	case "array":
		if arr, ok := value.([]interface{}); ok {
			return f.filterArray(arr, schema)
		}
	case "string", "number", "integer", "boolean":
		// For primitive types, just return the value
		return value
	}

	return value
}

// filterArray filters an array based on schema
func (f *Filter) filterArray(arr []interface{}, schema *JSONSchema) []interface{} {
	if schema.Items == nil {
		return arr
	}

	filtered := make([]interface{}, 0, len(arr))
	for _, item := range arr {
		filteredItem := f.filterValue(item, schema.Items)
		if filteredItem != nil {
			filtered = append(filtered, filteredItem)
		}
	}

	return filtered
}

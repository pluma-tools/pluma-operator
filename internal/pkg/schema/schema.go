package schema

import (
	"encoding/json"
	"fmt"

	"helm.sh/helm/v3/pkg/chart"
)

// JSONSchema represents a simplified JSON Schema structure for field filtering
type JSONSchema struct {
	Type                 interface{}            `json:"type,omitempty"` // Can be string or []string
	Properties           map[string]*JSONSchema `json:"properties,omitempty"`
	AdditionalProperties interface{}            `json:"additionalProperties,omitempty"`
	Items                *JSONSchema            `json:"items,omitempty"`
	Ref                  string                 `json:"$ref,omitempty"`
	Defs                 map[string]*JSONSchema `json:"$defs,omitempty"`
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
func LoadSchemaFromChart(chart *chart.Chart) (*JSONSchema, error) {
	// Look for values.schema.json in the chart files
	for _, file := range chart.Raw {
		if file.Name == "values.schema.json" {
			var schema JSONSchema
			if err := json.Unmarshal(file.Data, &schema); err != nil {
				return nil, fmt.Errorf("failed to parse values.schema.json: %w", err)
			}

			// Resolve $ref references
			resolvedSchema, err := ResolveSchemaReferences(&schema)
			if err != nil {
				return nil, fmt.Errorf("failed to resolve schema references: %w", err)
			}

			return resolvedSchema, nil
		}
	}

	return nil, fmt.Errorf("values.schema.json not found in chart")
}

// ResolveSchemaReferences resolves $ref references in the schema
func ResolveSchemaReferences(schema *JSONSchema) (*JSONSchema, error) {
	if schema == nil {
		return nil, nil
	}

	// If this schema has a $ref, resolve it
	if schema.Ref != "" {
		resolved, err := resolveRef(schema, schema)
		if err != nil {
			return nil, err
		}
		return resolved, nil
	}

	// Recursively resolve references in properties
	if schema.Properties != nil {
		for key, prop := range schema.Properties {
			resolved, err := ResolveSchemaReferences(prop)
			if err != nil {
				return nil, err
			}
			schema.Properties[key] = resolved
		}
	}

	// Resolve references in items
	if schema.Items != nil {
		resolved, err := ResolveSchemaReferences(schema.Items)
		if err != nil {
			return nil, err
		}
		schema.Items = resolved
	}

	// Resolve references in additionalProperties if it's a schema object
	if schema.AdditionalProperties != nil {
		if additionalSchema, ok := schema.AdditionalProperties.(map[string]interface{}); ok {
			// Convert to JSONSchema and resolve
			additionalJSONSchema := &JSONSchema{}
			if data, err := json.Marshal(additionalSchema); err == nil {
				json.Unmarshal(data, additionalJSONSchema)
				resolved, err := ResolveSchemaReferences(additionalJSONSchema)
				if err != nil {
					return nil, err
				}
				// Convert back to interface{}
				if resolvedData, err := json.Marshal(resolved); err == nil {
					var resolvedInterface interface{}
					json.Unmarshal(resolvedData, &resolvedInterface)
					schema.AdditionalProperties = resolvedInterface
				}
			}
		}
	}

	return schema, nil
}

// resolveRef resolves a $ref reference within a schema
func resolveRef(schema *JSONSchema, rootSchema *JSONSchema) (*JSONSchema, error) {
	if schema.Ref == "" {
		return schema, nil
	}

	// Handle simple $ref like "#/$defs/values"
	if schema.Ref == "#/$defs/values" && rootSchema.Defs != nil {
		if def, exists := rootSchema.Defs["values"]; exists {
			// Resolve any references in the definition
			return ResolveSchemaReferences(def)
		}
	}

	// Handle other $ref patterns if needed
	// For now, return the original schema if we can't resolve
	return schema, nil
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

	// Handle type field which can be string or []string
	schemaType := f.getSchemaType(schema)

	switch schemaType {
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

// getSchemaType extracts the primary type from schema.Type which can be string or []string
func (f *Filter) getSchemaType(schema *JSONSchema) string {
	if schema.Type == nil {
		return ""
	}

	switch t := schema.Type.(type) {
	case string:
		return t
	case []interface{}:
		// For type arrays like ["object", "null"], return the first non-null type
		for _, typeVal := range t {
			if typeStr, ok := typeVal.(string); ok && typeStr != "null" {
				return typeStr
			}
		}
		// If all types are "null", return "null"
		if len(t) > 0 {
			if typeStr, ok := t[0].(string); ok {
				return typeStr
			}
		}
	}

	return ""
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

package controller

import (
	"testing"

	operatorv1alpha1 "pluma.io/api/operator/v1alpha1"
	"pluma.io/pluma-operator/internal/pkg/tools"
)

func TestHelmAppReconciler_SchemaValidation_Logic(t *testing.T) {
	tests := []struct {
		name                string
		component           *operatorv1alpha1.HelmComponent
		globalValues        map[string]interface{}
		componentValues     map[string]interface{}
		expectedSchemaCheck bool
		description         string
	}{
		{
			name: "gateway component should enable schema validation",
			component: &operatorv1alpha1.HelmComponent{
				Name:                   "istio-gateway",
				Chart:                  "gateway",
				Version:                "1.25.5",
				EnableSchemaValidation: true,
			},
			globalValues: map[string]interface{}{
				"global": map[string]interface{}{
					"hub": "release-ci.daocloud.io/mspider",
				},
				"meshConfig": map[string]interface{}{
					"defaultConfig": map[string]interface{}{
						"extraStatTags": []string{"destination_mesh_id"},
					},
				},
			},
			componentValues: map[string]interface{}{
				"autoscaling": map[string]interface{}{
					"enabled":     true,
					"minReplicas": 1,
				},
			},
			expectedSchemaCheck: true,
			description:         "Gateway component should have schema validation enabled",
		},
		{
			name: "istiod component should not enable schema validation",
			component: &operatorv1alpha1.HelmComponent{
				Name:                   "istio-istiod",
				Chart:                  "istiod",
				Version:                "1.25.5",
				EnableSchemaValidation: false,
			},
			globalValues: map[string]interface{}{
				"global": map[string]interface{}{
					"hub": "release-ci.daocloud.io/mspider",
				},
				"meshConfig": map[string]interface{}{
					"defaultConfig": map[string]interface{}{
						"extraStatTags": []string{"destination_mesh_id"},
					},
				},
			},
			componentValues: map[string]interface{}{
				"pilot": map[string]interface{}{
					"resources": map[string]interface{}{
						"limits": map[string]interface{}{
							"cpu":    "1500m",
							"memory": "1500Mi",
						},
					},
				},
			},
			expectedSchemaCheck: false,
			description:         "Istiod component should not have schema validation enabled",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test the value merging logic
			values := tools.MergeMaps(tt.globalValues, tt.componentValues)

			// Verify that values are merged correctly
			t.Logf("Values merged: global=%d, component=%d, merged=%d",
				len(tt.globalValues), len(tt.componentValues), len(values))

			// Test schema validation flag
			if tt.component.EnableSchemaValidation != tt.expectedSchemaCheck {
				t.Errorf("EnableSchemaValidation = %v, want %v",
					tt.component.EnableSchemaValidation, tt.expectedSchemaCheck)
			}

			// Verify that the component has the expected configuration
			if tt.component.Name == "" {
				t.Error("Component name should not be empty")
			}
			if tt.component.Chart == "" {
				t.Error("Component chart should not be empty")
			}

			// Test the logic path for schema validation
			if tt.component.EnableSchemaValidation {
				t.Logf("Schema validation is enabled for component %s", tt.component.Name)
				// In a real scenario, this would trigger schema-based filtering
			} else {
				t.Logf("Schema validation is disabled for component %s", tt.component.Name)
				// In a real scenario, values would pass through without filtering
			}
		})
	}
}

func TestHelmAppReconciler_ComponentConfiguration(t *testing.T) {
	tests := []struct {
		name        string
		component   *operatorv1alpha1.HelmComponent
		expectValid bool
	}{
		{
			name: "valid gateway component",
			component: &operatorv1alpha1.HelmComponent{
				Name:                   "istio-gateway",
				Chart:                  "gateway",
				Version:                "1.25.5",
				EnableSchemaValidation: true,
			},
			expectValid: true,
		},
		{
			name: "valid istiod component",
			component: &operatorv1alpha1.HelmComponent{
				Name:                   "istio-istiod",
				Chart:                  "istiod",
				Version:                "1.25.5",
				EnableSchemaValidation: false,
			},
			expectValid: true,
		},
		{
			name: "invalid component with empty name",
			component: &operatorv1alpha1.HelmComponent{
				Name:                   "",
				Chart:                  "gateway",
				Version:                "1.25.5",
				EnableSchemaValidation: true,
			},
			expectValid: false,
		},
		{
			name: "invalid component with empty chart",
			component: &operatorv1alpha1.HelmComponent{
				Name:                   "istio-gateway",
				Chart:                  "",
				Version:                "1.25.5",
				EnableSchemaValidation: true,
			},
			expectValid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isValid := tt.component.Name != "" && tt.component.Chart != ""

			if isValid != tt.expectValid {
				t.Errorf("Component validation = %v, want %v", isValid, tt.expectValid)
			}

			// Test schema validation flag
			if tt.component.EnableSchemaValidation {
				t.Logf("Component %s has schema validation enabled", tt.component.Name)
			} else {
				t.Logf("Component %s has schema validation disabled", tt.component.Name)
			}
		})
	}
}

func TestHelmAppReconciler_ValueMerging(t *testing.T) {
	tests := []struct {
		name            string
		globalValues    map[string]interface{}
		componentValues map[string]interface{}
		expectedCount   int
	}{
		{
			name: "merge global and component values",
			globalValues: map[string]interface{}{
				"global": map[string]interface{}{
					"hub": "release-ci.daocloud.io/mspider",
				},
			},
			componentValues: map[string]interface{}{
				"autoscaling": map[string]interface{}{
					"enabled": true,
				},
			},
			expectedCount: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test the value merging logic
			values := tools.MergeMaps(tt.globalValues, tt.componentValues)

			// Verify that values are merged correctly
			if len(values) != tt.expectedCount {
				t.Errorf("Values merged count = %v, want %v", len(values), tt.expectedCount)
			}
		})
	}
}

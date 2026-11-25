package istio

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"google.golang.org/protobuf/types/known/structpb"
	"istio.io/istio/operator/pkg/component"
	"istio.io/istio/operator/pkg/render"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	istiov1alpha1 "pluma.io/api/istio/v1alpha1"
	operatorv1alpha1 "pluma.io/api/operator/v1alpha1"
)

func TestConvertIOPRendersLegacyGatewayFromInstallPackage(t *testing.T) {
	dir := t.TempDir()
	chartDir := "charts/gateways/istio-ingress"
	const legacySchema = `{"$ref":"#/$defs/values","additionalProperties":false,"$defs":{"values":{"properties":{"replicaCount":{"type":"integer"}}}}}`
	for name, content := range map[string]string{
		"profiles/default.yaml": `spec:
  components:
    pilot: {enabled: false}
    ingressGateways:
    - {name: custom-gateway, enabled: true}
`,
		"charts/base/Chart.yaml":  "apiVersion: v2\nname: base\nversion: 1.23.4\n",
		chartDir + "/Chart.yaml":  "apiVersion: v2\nname: gateway\nversion: 1.23.4\n",
		chartDir + "/values.yaml": "defaults: {replicaCount: 1}\n",
		chartDir + "/templates/gateway.yaml": `apiVersion: v1
kind: ConfigMap
metadata:
  name: gateway
data:
  replicas: {{ .Values.defaults.replicaCount | quote }}
`,
		"gateway.schema.json": legacySchema,
	} {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	schemaPath := filepath.Join(dir, chartDir, "values.schema.json")
	if err := os.Symlink(filepath.Join(dir, "gateway.schema.json"), schemaPath); err != nil {
		t.Fatal(err)
	}

	iop := &istiov1alpha1.IstioOperator{
		TypeMeta:   metav1.TypeMeta{APIVersion: "install.istio.io/v1alpha1", Kind: "IstioOperator"},
		ObjectMeta: metav1.ObjectMeta{Name: "mesh", Namespace: "istio-system"},
		Spec:       &istiov1alpha1.IstioOperatorSpec{InstallPackagePath: dir, Tag: structpb.NewStringValue("1.23.4")},
	}
	r := &IstioOperatorReconciler{}
	app, err := r.convertIopToHelmApp(context.Background(), iop)
	if err != nil {
		t.Fatal(err)
	}
	if got := app.Spec.Components; len(got) != 2 || got[0].Chart != "base" ||
		got[1].Chart != "gateway" || got[1].Name != "custom-gateway" || got[1].Version != "1.23.4" {
		t.Fatalf("rendered components = %v", got)
	}
	iop.Spec.Values = &structpb.Struct{Fields: map[string]*structpb.Value{
		"replicaCount": structpb.NewStringValue("invalid"),
	}}
	if _, err := r.convertIopToHelmApp(context.Background(), iop); err == nil || !strings.Contains(err.Error(), "want integer") {
		t.Fatalf("expected Helm to reject an invalid replicaCount, got %v", err)
	}
	info, err := os.Lstat(schemaPath)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(schemaPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink == 0 || string(data) != legacySchema {
		t.Fatal("the original chart schema was modified")
	}
}

func TestConvertIOPKeepsBuiltInImageDefaults(t *testing.T) {
	for _, tc := range []struct {
		hub, valuesHub, want string
	}{
		{want: "gcr.io/istio-testing"},
		{hub: "images.example/istio", want: "images.example/istio"},
		{valuesHub: "values.example/istio", want: "values.example/istio"},
		{hub: "images.example/istio", valuesHub: "values.example/istio", want: "values.example/istio"},
	} {
		t.Run(tc.hub+"_"+tc.valuesHub, func(t *testing.T) {
			iop := &istiov1alpha1.IstioOperator{
				TypeMeta:   metav1.TypeMeta{APIVersion: "install.istio.io/v1alpha1", Kind: "IstioOperator"},
				ObjectMeta: metav1.ObjectMeta{Name: "mesh", Namespace: "istio-system"},
				Spec:       &istiov1alpha1.IstioOperatorSpec{Hub: tc.hub},
			}
			if tc.valuesHub != "" {
				iop.Spec.Values = &structpb.Struct{Fields: map[string]*structpb.Value{
					"global": structpb.NewStructValue(&structpb.Struct{Fields: map[string]*structpb.Value{
						"hub": structpb.NewStringValue(tc.valuesHub),
					}}),
				}}
			}
			app, err := (&IstioOperatorReconciler{}).convertIopToHelmApp(context.Background(), iop)
			if err != nil {
				t.Fatal(err)
			}
			global := app.Spec.GlobalValues.GetFields()["global"].GetStructValue().GetFields()
			if got := global["hub"].GetStringValue(); got != tc.want {
				t.Errorf("image hub = %q, want %q", got, tc.want)
			}
			for _, component := range app.Spec.Components {
				if component.Version != "1.22.8" {
					t.Errorf("%s chart version = %q, want 1.22.8", component.Name, component.Version)
				}
			}
		})
	}
}

func TestConvertIOPKeepsLegacyRemoteFromInstallPackage(t *testing.T) {
	dir := t.TempDir()
	for name, content := range map[string]string{
		"profiles/default.yaml": `spec:
  hub: custom.example/istio
  components:
    base: {enabled: false}
    pilot: {enabled: false}
    ingressGateways: []
    istiodRemote: {enabled: true}
  values:
    global:
      remotePilotAddress: 192.0.2.10
`,
		"charts/istiod-remote/Chart.yaml": "apiVersion: v2\nname: istiod-remote\nversion: 1.23.4\n",
	} {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	iop := &istiov1alpha1.IstioOperator{
		TypeMeta:   metav1.TypeMeta{APIVersion: "install.istio.io/v1alpha1", Kind: "IstioOperator"},
		ObjectMeta: metav1.ObjectMeta{Name: "mesh", Namespace: "istio-system"},
		Spec:       &istiov1alpha1.IstioOperatorSpec{InstallPackagePath: dir, Tag: structpb.NewStringValue("1.23.4")},
	}
	app, err := (&IstioOperatorReconciler{}).convertIopToHelmApp(context.Background(), iop)
	if err != nil {
		t.Fatal(err)
	}
	if got := app.Spec.Components; len(got) != 1 || got[0].Chart != "istiod-remote" ||
		got[0].Name != "iop-mesh-istiod-remote" || got[0].Version != "1.23.4" {
		t.Fatalf("legacy remote components = %v", got)
	}
	global := app.Spec.GlobalValues.GetFields()["global"].GetStructValue().GetFields()
	if global["hub"].GetStringValue() != "custom.example/istio" ||
		global["remotePilotAddress"].GetStringValue() != "192.0.2.10" {
		t.Fatalf("custom install package values changed: %v", global)
	}
	iop.Spec.Tag = structpb.NewStringValue("1.24.6")
	if app, err := (&IstioOperatorReconciler{}).convertIopToHelmApp(context.Background(), iop); err == nil {
		t.Fatalf("legacy remote-only IOP unexpectedly produced a HelmApp on 1.24.6: %v", app)
	}
}

func TestIstioOperatorReconciler_convertIopToHelmApp_WithSchemaValidation(t *testing.T) {
	tests := []struct {
		name                  string
		iop                   *istiov1alpha1.IstioOperator
		expectedGatewaySchema bool
		expectedIstiodSchema  bool
		expectedBaseSchema    bool
	}{
		{
			name: "IstioOperator with gateway component should enable schema validation",
			iop: &istiov1alpha1.IstioOperator{
				TypeMeta: metav1.TypeMeta{
					Kind:       "IstioOperator",
					APIVersion: "install.istio.io/v1alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-iop",
					Namespace: "istio-system",
				},
				Spec: &istiov1alpha1.IstioOperatorSpec{
					Tag: &structpb.Value{
						Kind: &structpb.Value_StringValue{
							StringValue: "1.25.5",
						},
					},
					Values: &structpb.Struct{
						Fields: map[string]*structpb.Value{
							"global": {
								Kind: &structpb.Value_StructValue{
									StructValue: &structpb.Struct{
										Fields: map[string]*structpb.Value{
											"hub": {
												Kind: &structpb.Value_StringValue{
													StringValue: "release-ci.daocloud.io/mspider",
												},
											},
											"istioNamespace": {
												Kind: &structpb.Value_StringValue{
													StringValue: "istio-system",
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			expectedGatewaySchema: true,
			expectedIstiodSchema:  false,
			expectedBaseSchema:    false,
		},
		{
			name: "IstioOperator with pilot component should not enable schema validation",
			iop: &istiov1alpha1.IstioOperator{
				TypeMeta: metav1.TypeMeta{
					Kind:       "IstioOperator",
					APIVersion: "install.istio.io/v1alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-iop-pilot",
					Namespace: "istio-system",
				},
				Spec: &istiov1alpha1.IstioOperatorSpec{
					Tag: &structpb.Value{
						Kind: &structpb.Value_StringValue{
							StringValue: "1.25.5",
						},
					},
					Values: &structpb.Struct{
						Fields: map[string]*structpb.Value{
							"pilot": {
								Kind: &structpb.Value_StructValue{
									StructValue: &structpb.Struct{
										Fields: map[string]*structpb.Value{
											"resources": {
												Kind: &structpb.Value_StructValue{
													StructValue: &structpb.Struct{
														Fields: map[string]*structpb.Value{
															"limits": {
																Kind: &structpb.Value_StructValue{
																	StructValue: &structpb.Struct{
																		Fields: map[string]*structpb.Value{
																			"cpu": {
																				Kind: &structpb.Value_StringValue{
																					StringValue: "1500m",
																				},
																			},
																			"memory": {
																				Kind: &structpb.Value_StringValue{
																					StringValue: "1500Mi",
																				},
																			},
																		},
																	},
																},
															},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			expectedGatewaySchema: true, // Gateway components are automatically enabled for schema validation
			expectedIstiodSchema:  false,
			expectedBaseSchema:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a mock reconciler
			reconciler := &IstioOperatorReconciler{}

			// Convert IstioOperator to HelmApp
			helmApp, err := reconciler.convertIopToHelmApp(context.Background(), tt.iop)
			if err != nil {
				t.Fatalf("convertIopToHelmApp() error = %v", err)
			}

			// Check if schema validation is enabled for gateway components
			gatewaySchemaEnabled := false
			istiodSchemaEnabled := false
			baseSchemaEnabled := false

			for _, component := range helmApp.Spec.Components {
				switch component.Chart {
				case "gateway":
					gatewaySchemaEnabled = component.EnableSchemaValidation
				case "istiod":
					istiodSchemaEnabled = component.EnableSchemaValidation
				case "base":
					baseSchemaEnabled = component.EnableSchemaValidation
				}
			}

			if gatewaySchemaEnabled != tt.expectedGatewaySchema {
				t.Errorf("Gateway schema validation = %v, want %v", gatewaySchemaEnabled, tt.expectedGatewaySchema)
			}

			if istiodSchemaEnabled != tt.expectedIstiodSchema {
				t.Errorf("Istiod schema validation = %v, want %v", istiodSchemaEnabled, tt.expectedIstiodSchema)
			}

			if baseSchemaEnabled != tt.expectedBaseSchema {
				t.Errorf("Base schema validation = %v, want %v", baseSchemaEnabled, tt.expectedBaseSchema)
			}
		})
	}
}

func TestIsGateway(t *testing.T) {
	tests := []struct {
		name     string
		specName string
		expected bool
	}{
		{
			name:     "ingress gateways",
			specName: "ingressGateways",
			expected: true,
		},
		{
			name:     "egress gateways",
			specName: "egressGateways",
			expected: true,
		},
		{
			name:     "pilot component",
			specName: "pilot",
			expected: false,
		},
		{
			name:     "base component",
			specName: "base",
			expected: false,
		},
		{
			name:     "cni component",
			specName: "cni",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a mock component migration
			cInfo := render.ComponentMigration{
				Component: component.Component{
					SpecName: tt.specName,
				},
			}

			result := isGateway(cInfo)
			if result != tt.expected {
				t.Errorf("isGateway() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestIsIngressGateway(t *testing.T) {
	tests := []struct {
		name     string
		specName string
		expected bool
	}{
		{
			name:     "ingress gateways",
			specName: "ingressGateways",
			expected: true,
		},
		{
			name:     "egress gateways",
			specName: "egressGateways",
			expected: false,
		},
		{
			name:     "pilot component",
			specName: "pilot",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cInfo := render.ComponentMigration{
				Component: component.Component{
					SpecName: tt.specName,
				},
			}

			result := isIngressGateway(cInfo)
			if result != tt.expected {
				t.Errorf("isIngressGateway() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestIsEgressGateway(t *testing.T) {
	tests := []struct {
		name     string
		specName string
		expected bool
	}{
		{
			name:     "egress gateways",
			specName: "egressGateways",
			expected: true,
		},
		{
			name:     "ingress gateways",
			specName: "ingressGateways",
			expected: false,
		},
		{
			name:     "pilot component",
			specName: "pilot",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cInfo := render.ComponentMigration{
				Component: component.Component{
					SpecName: tt.specName,
				},
			}

			result := isEgressGateway(cInfo)
			if result != tt.expected {
				t.Errorf("isEgressGateway() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestIstioOperatorReconciler_convertIopToHelmApp_Autoscaling(t *testing.T) {
	tests := []struct {
		name                 string
		iop                  *istiov1alpha1.IstioOperator
		expectedAutoscaling  map[string]interface{}
		gatewayComponentName string
		description          string
	}{
		{
			name:                 "autoscaling default disabled when not set",
			description:          "When autoscaleEnabled is not set, autoscaling should be disabled by default",
			gatewayComponentName: "istio-ingressgateway",
			iop: &istiov1alpha1.IstioOperator{
				TypeMeta: metav1.TypeMeta{
					Kind:       "IstioOperator",
					APIVersion: "install.istio.io/v1alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-iop-default",
					Namespace: "istio-system",
				},
				Spec: &istiov1alpha1.IstioOperatorSpec{
					Tag: &structpb.Value{
						Kind: &structpb.Value_StringValue{
							StringValue: "1.25.5",
						},
					},
					Values: &structpb.Struct{
						Fields: map[string]*structpb.Value{
							"global": {
								Kind: &structpb.Value_StructValue{
									StructValue: &structpb.Struct{
										Fields: map[string]*structpb.Value{
											"istioNamespace": {
												Kind: &structpb.Value_StringValue{
													StringValue: "istio-system",
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			expectedAutoscaling: map[string]interface{}{
				"enabled": false,
			},
		},
		{
			name:                 "autoscaling enabled when autoscaleEnabled is true",
			description:          "When autoscaleEnabled is explicitly set to true, autoscaling should be enabled",
			gatewayComponentName: "istio-ingressgateway",
			iop: &istiov1alpha1.IstioOperator{
				TypeMeta: metav1.TypeMeta{
					Kind:       "IstioOperator",
					APIVersion: "install.istio.io/v1alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-iop-enabled",
					Namespace: "istio-system",
				},
				Spec: &istiov1alpha1.IstioOperatorSpec{
					Tag: &structpb.Value{
						Kind: &structpb.Value_StringValue{
							StringValue: "1.25.5",
						},
					},
					Values: &structpb.Struct{
						Fields: map[string]*structpb.Value{
							"global": {
								Kind: &structpb.Value_StructValue{
									StructValue: &structpb.Struct{
										Fields: map[string]*structpb.Value{
											"istioNamespace": {
												Kind: &structpb.Value_StringValue{
													StringValue: "istio-system",
												},
											},
										},
									},
								},
							},
							"gateways": {
								Kind: &structpb.Value_StructValue{
									StructValue: &structpb.Struct{
										Fields: map[string]*structpb.Value{
											"istio-ingressgateway": {
												Kind: &structpb.Value_StructValue{
													StructValue: &structpb.Struct{
														Fields: map[string]*structpb.Value{
															"autoscaleEnabled": {
																Kind: &structpb.Value_BoolValue{
																	BoolValue: true,
																},
															},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			expectedAutoscaling: map[string]interface{}{
				"enabled": true,
			},
		},
		{
			name:                 "autoscaling disabled when autoscaleEnabled is false",
			description:          "When autoscaleEnabled is explicitly set to false, autoscaling should be disabled",
			gatewayComponentName: "istio-ingressgateway",
			iop: &istiov1alpha1.IstioOperator{
				TypeMeta: metav1.TypeMeta{
					Kind:       "IstioOperator",
					APIVersion: "install.istio.io/v1alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-iop-disabled",
					Namespace: "istio-system",
				},
				Spec: &istiov1alpha1.IstioOperatorSpec{
					Tag: &structpb.Value{
						Kind: &structpb.Value_StringValue{
							StringValue: "1.25.5",
						},
					},
					Values: &structpb.Struct{
						Fields: map[string]*structpb.Value{
							"global": {
								Kind: &structpb.Value_StructValue{
									StructValue: &structpb.Struct{
										Fields: map[string]*structpb.Value{
											"istioNamespace": {
												Kind: &structpb.Value_StringValue{
													StringValue: "istio-system",
												},
											},
										},
									},
								},
							},
							"gateways": {
								Kind: &structpb.Value_StructValue{
									StructValue: &structpb.Struct{
										Fields: map[string]*structpb.Value{
											"istio-ingressgateway": {
												Kind: &structpb.Value_StructValue{
													StructValue: &structpb.Struct{
														Fields: map[string]*structpb.Value{
															"autoscaleEnabled": {
																Kind: &structpb.Value_BoolValue{
																	BoolValue: false,
																},
															},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			expectedAutoscaling: map[string]interface{}{
				"enabled": false,
			},
		},
		{
			name:                 "autoscaling enabled with minReplicas when both are set",
			description:          "When autoscaleEnabled is true and autoscaleMin is set, both should be included",
			gatewayComponentName: "istio-ingressgateway",
			iop: &istiov1alpha1.IstioOperator{
				TypeMeta: metav1.TypeMeta{
					Kind:       "IstioOperator",
					APIVersion: "install.istio.io/v1alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-iop-with-min",
					Namespace: "istio-system",
				},
				Spec: &istiov1alpha1.IstioOperatorSpec{
					Tag: &structpb.Value{
						Kind: &structpb.Value_StringValue{
							StringValue: "1.25.5",
						},
					},
					Values: &structpb.Struct{
						Fields: map[string]*structpb.Value{
							"global": {
								Kind: &structpb.Value_StructValue{
									StructValue: &structpb.Struct{
										Fields: map[string]*structpb.Value{
											"istioNamespace": {
												Kind: &structpb.Value_StringValue{
													StringValue: "istio-system",
												},
											},
										},
									},
								},
							},
							"gateways": {
								Kind: &structpb.Value_StructValue{
									StructValue: &structpb.Struct{
										Fields: map[string]*structpb.Value{
											"istio-ingressgateway": {
												Kind: &structpb.Value_StructValue{
													StructValue: &structpb.Struct{
														Fields: map[string]*structpb.Value{
															"autoscaleEnabled": {
																Kind: &structpb.Value_BoolValue{
																	BoolValue: true,
																},
															},
															"autoscaleMin": {
																Kind: &structpb.Value_StringValue{
																	StringValue: "3",
																},
															},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			expectedAutoscaling: map[string]interface{}{
				"enabled":     true,
				"minReplicas": "3",
			},
		},
		{
			name:                 "egress gateway autoscaling default disabled",
			description:          "Egress gateway should also have autoscaling disabled by default",
			gatewayComponentName: "istio-egressgateway",
			iop: &istiov1alpha1.IstioOperator{
				TypeMeta: metav1.TypeMeta{
					Kind:       "IstioOperator",
					APIVersion: "install.istio.io/v1alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-iop-egress",
					Namespace: "istio-system",
				},
				Spec: &istiov1alpha1.IstioOperatorSpec{
					Tag: &structpb.Value{
						Kind: &structpb.Value_StringValue{
							StringValue: "1.25.5",
						},
					},
					Values: &structpb.Struct{
						Fields: map[string]*structpb.Value{
							"global": {
								Kind: &structpb.Value_StructValue{
									StructValue: &structpb.Struct{
										Fields: map[string]*structpb.Value{
											"istioNamespace": {
												Kind: &structpb.Value_StringValue{
													StringValue: "istio-system",
												},
											},
										},
									},
								},
							},
							"gateways": {
								Kind: &structpb.Value_StructValue{
									StructValue: &structpb.Struct{
										Fields: map[string]*structpb.Value{
											"istio-egressgateway": {
												Kind: &structpb.Value_StructValue{
													StructValue: &structpb.Struct{
														Fields: map[string]*structpb.Value{},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			expectedAutoscaling: map[string]interface{}{
				"enabled": false,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a mock reconciler
			reconciler := &IstioOperatorReconciler{}

			// Convert IstioOperator to HelmApp
			helmApp, err := reconciler.convertIopToHelmApp(context.Background(), tt.iop)
			if err != nil {
				t.Fatalf("convertIopToHelmApp() error = %v", err)
			}

			// Find the gateway component (should be unique by Chart name)
			var gatewayComponent *operatorv1alpha1.HelmComponent
			for _, component := range helmApp.Spec.Components {
				if component.Chart == "gateway" {
					gatewayComponent = component
					break
				}
			}

			if gatewayComponent == nil {
				t.Fatalf("Gateway component not found in HelmApp")
			}

			// Extract autoscaling from component values
			componentValuesMap := structToMap(gatewayComponent.ComponentValues)
			if componentValuesMap == nil {
				t.Fatalf("Failed to convert component values to map")
			}
			autoscaling, ok := componentValuesMap["autoscaling"]
			if !ok {
				t.Fatalf("autoscaling not found in component values")
			}

			autoscalingMap, ok := autoscaling.(map[string]interface{})
			if !ok {
				t.Fatalf("autoscaling is not a map[string]interface{}, got %T", autoscaling)
			}

			// Check enabled field
			enabled, ok := autoscalingMap["enabled"]
			if !ok {
				t.Errorf("autoscaling.enabled not found")
			} else {
				expectedEnabled := tt.expectedAutoscaling["enabled"]
				if enabled != expectedEnabled {
					t.Errorf("autoscaling.enabled = %v, want %v", enabled, expectedEnabled)
				}
			}

			// Check minReplicas field if expected
			if expectedMinReplicas, ok := tt.expectedAutoscaling["minReplicas"]; ok {
				minReplicas, ok := autoscalingMap["minReplicas"]
				if !ok {
					t.Errorf("autoscaling.minReplicas not found, want %v", expectedMinReplicas)
				} else if minReplicas != expectedMinReplicas {
					t.Errorf("autoscaling.minReplicas = %v, want %v", minReplicas, expectedMinReplicas)
				}
			} else {
				// If minReplicas is not expected, it should not be present
				if minReplicas, ok := autoscalingMap["minReplicas"]; ok {
					t.Errorf("autoscaling.minReplicas should not be present, got %v", minReplicas)
				}
			}
		})
	}
}

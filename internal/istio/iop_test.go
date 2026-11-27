package istio

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
	"istio.io/istio/operator/pkg/component"
	"istio.io/istio/operator/pkg/render"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	istiov1alpha1 "pluma.io/api/istio/v1alpha1"
	operatorv1alpha1 "pluma.io/api/operator/v1alpha1"
	"pluma.io/pluma-operator/internal/pkg/constants"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/yaml"
)

func TestHelmAppUpdateCompatibility(t *testing.T) {
	for _, managed := range []bool{true, false} {
		t.Run(map[bool]string{true: "managed", false: "unmanaged"}[managed], func(t *testing.T) {
			scheme := runtime.NewScheme()
			if err := operatorv1alpha1.AddToScheme(scheme); err != nil {
				t.Fatal(err)
			}
			labels := map[string]string{}
			if managed {
				labels[constants.ManagedLabel] = constants.ManagedLabelValue
			}
			existing := &operatorv1alpha1.HelmApp{
				ObjectMeta: metav1.ObjectMeta{Name: "migration", Namespace: "istio-system", Labels: labels},
				Spec:       &operatorv1alpha1.HelmAppSpec{Repo: &operatorv1alpha1.HelmRepo{Url: "https://old.example.com"}},
				Status:     &operatorv1alpha1.HelmAppStatus{Phase: operatorv1alpha1.Phase_SUCCEEDED},
			}
			kube := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(existing).WithObjects(existing).Build()
			r := &IstioOperatorReconciler{Client: kube, Scheme: scheme}
			desired := existing.DeepCopy()
			desired.Spec.Repo.Url = "https://new.example.com"
			if err := r.createOrUpdateHelmApp(context.Background(), desired); err != nil {
				t.Fatal(err)
			}
			got := &operatorv1alpha1.HelmApp{}
			key := client.ObjectKeyFromObject(existing)
			if err := kube.Get(context.Background(), key, got); err != nil {
				t.Fatal(err)
			}
			wantSpec := existing.Spec
			if managed {
				wantSpec = desired.Spec
			}
			if !proto.Equal(got.Spec, wantSpec) || !proto.Equal(got.Status, existing.Status) {
				t.Fatalf("spec or status changed unexpectedly: %v", got)
			}
			version := got.ResourceVersion
			if err := r.createOrUpdateHelmApp(context.Background(), desired); err != nil {
				t.Fatal(err)
			}
			if err := kube.Get(context.Background(), key, got); err != nil {
				t.Fatal(err)
			}
			if got.ResourceVersion != version {
				t.Error("replaying the same configuration should not update the resource")
			}
		})
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

// Compare the complete migration output with snapshots captured before dependency upgrades.
func TestMigrationCompatibility(t *testing.T) {
	input, err := os.ReadFile("../../demo/iop.yaml")
	if err != nil {
		t.Fatal(err)
	}
	for _, profile := range []string{"default", "remote", "ambient"} {
		t.Run(profile, func(t *testing.T) {
			iop := &istiov1alpha1.IstioOperator{}
			if err := yaml.Unmarshal(input, iop); err != nil {
				t.Fatal(err)
			}
			iop.Spec.Profile = profile
			result, err := (&IstioOperatorReconciler{}).convertIopToHelmApp(context.Background(), iop)
			if err != nil {
				t.Fatal(err)
			}
			got, err := json.MarshalIndent(result, "", "  ")
			if err != nil {
				t.Fatal(err)
			}
			got = append(got, '\n')
			path := filepath.Join("testdata", "compatibility", profile+".json")
			if os.Getenv("UPDATE_COMPATIBILITY") == "1" {
				if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, got, 0644); err != nil {
					t.Fatal(err)
				}
			}
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != string(want) {
				t.Errorf("migration output changed from the pre-upgrade snapshot %s", path)
			}
		})
	}
}

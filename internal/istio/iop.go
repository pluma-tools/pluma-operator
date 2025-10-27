package istio

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"time"

	"istio.io/istio/operator/pkg/apis"
	"istio.io/istio/operator/pkg/render"
	"k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/json"
	"pluma.io/pluma-operator/config"
	"pluma.io/pluma-operator/internal/pkg/constants"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/yaml"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"google.golang.org/protobuf/types/known/structpb"
	structpb2 "google.golang.org/protobuf/types/known/structpb"
	istiov1alpha1 "pluma.io/api/istio/v1alpha1"

	operatorv1alpha1 "pluma.io/api/operator/v1alpha1"
)

// IstioOperatorReconciler reconciles a IstioOperator object
type IstioOperatorReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Config config.Config
}

// SetupWithManager sets up the controller with the Manager.
func (r *IstioOperatorReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&istiov1alpha1.IstioOperator{}).
		Complete(r)
}

func (r *IstioOperatorReconciler) reconcileDelete(ctx context.Context, iop *istiov1alpha1.IstioOperator) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	// Attempt to remove the HelmApp
	hApp := &operatorv1alpha1.HelmApp{}
	err := r.Get(ctx, client.ObjectKey{Namespace: iop.GetNamespace(), Name: iop.GetName()}, hApp)
	if err != nil {
		if !errors.IsNotFound(err) {
			return ctrl.Result{}, fmt.Errorf("failed to get HelmApp: %w", err)
		}
		// HelmApp not found, proceed with finalizer removal
	} else if hApp.GetName() == iop.GetName() && hApp.Labels != nil && hApp.Labels[constants.ManagedLabel] == constants.ManagedLabelValue {
		// HelmApp found, attempt to delete it
		if err := r.Delete(ctx, hApp); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to delete HelmApp: %w", err)
		}
		log.Info("HelmApp deleted successfully", "HelmApp", hApp.Name)
	}

	// Remove the finalizer from the IstioOperator
	controllerutil.RemoveFinalizer(iop, constants.IOPFinalizer)
	if err := r.Update(ctx, iop); err != nil {
		return ctrl.Result{}, err
	}

	log.Info("Finalizer removed successfully", "IstioOperator", iop.Name)

	return ctrl.Result{}, nil
}

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *IstioOperatorReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	// Fetch the IstioOperator instance
	iop := &istiov1alpha1.IstioOperator{}
	if err := r.Get(ctx, req.NamespacedName, iop); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Check if object is being deleted
	if !iop.ObjectMeta.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, iop)
	}

	// Add finalizer if it doesn't exist
	if !controllerutil.ContainsFinalizer(iop, constants.IOPFinalizer) {
		controllerutil.AddFinalizer(iop, constants.IOPFinalizer)
		if err := r.Update(ctx, iop); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Convert IstioOperator to HelmApp
	helmApp, err := r.convertIopToHelmApp(ctx, iop)
	if err != nil {
		log.Error(err, "Failed to convert IstioOperator to HelmApp")
		return ctrl.Result{}, err
	}

	// Create or update the HelmApp
	if err := r.createOrUpdateHelmApp(ctx, helmApp); err != nil {
		log.Error(err, "Failed to create or update HelmApp")
		return ctrl.Result{}, err
	}

	status := r.calculateOverallPhase(ctx, iop)
	iop.Status = &istiov1alpha1.InstallStatus{
		Status: status,
	}
	if err := r.Status().Update(ctx, iop); err != nil {
		return ctrl.Result{RequeueAfter: serverFailedAfter}, fmt.Errorf("failed to update iop status: %w", err)
	}

	switch status {
	case istiov1alpha1.InstallStatus_ERROR:
		return ctrl.Result{RequeueAfter: failedAfter}, nil
	case istiov1alpha1.InstallStatus_RECONCILING, istiov1alpha1.InstallStatus_NONE:
		return ctrl.Result{RequeueAfter: reconcileAfter}, nil
	default:
		return ctrl.Result{}, nil
	}
}

const (
	failedAfter       = 90 * time.Second
	serverFailedAfter = 60 * time.Second
	reconcileAfter    = 20 * time.Second
)

func (r *IstioOperatorReconciler) calculateOverallPhase(ctx context.Context, iop *istiov1alpha1.IstioOperator) istiov1alpha1.InstallStatus_Status {
	if iop == nil {
		return istiov1alpha1.InstallStatus_NONE
	}
	helmApp := &operatorv1alpha1.HelmApp{}
	err := r.Get(ctx, client.ObjectKey{Namespace: iop.GetNamespace(), Name: iop.GetName()}, helmApp)
	if err != nil {
		if errors.IsNotFound(err) {
			return istiov1alpha1.InstallStatus_RECONCILING
		}
		return istiov1alpha1.InstallStatus_NONE
	}

	phase := operatorv1alpha1.Phase_UNKNOWN
	if helmApp.Status != nil {
		phase = helmApp.Status.GetPhase()
	}
	switch phase {
	case operatorv1alpha1.Phase_UNKNOWN:
		return istiov1alpha1.InstallStatus_NONE
	case operatorv1alpha1.Phase_SUCCEEDED:
		return istiov1alpha1.InstallStatus_HEALTHY
	case operatorv1alpha1.Phase_FAILED:
		return istiov1alpha1.InstallStatus_ERROR
	default:
		return istiov1alpha1.InstallStatus_RECONCILING
	}
}

func structToMap(in any) map[string]interface{} {
	var res map[string]interface{}
	inStr, err := json.Marshal(in)
	if err != nil {
		_ = fmt.Errorf("failed to marshal input to JSON: %w", err)
		return nil
	}

	if err := json.Unmarshal(inStr, &res); err != nil {
		_ = fmt.Errorf("failed to unmarshal JSON to map: %w", err)
		return nil
	}
	return res
}

func (r *IstioOperatorReconciler) convertIopToHelmApp(ctx context.Context, in *istiov1alpha1.IstioOperator) (*operatorv1alpha1.HelmApp, error) {
	log := log.FromContext(ctx)

	if in == nil || in.Spec == nil {
		return nil, fmt.Errorf("iop is required")
	}

	tempFile, err := os.CreateTemp("", "iop-*.yaml")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp file: %w", err)
	}
	defer func() {
		if err := tempFile.Close(); err != nil {
			log.Error(err, "Failed to close temp file")
		}
		if err := os.Remove(tempFile.Name()); err != nil {
			log.Error(err, "Failed to remove temp file")
		}
	}()

	data, err := yaml.Marshal(in)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal IstioOperator to YAML: %w", err)
	}

	if _, err := tempFile.Write(data); err != nil {
		return nil, fmt.Errorf("failed to write to temp file: %w", err)
	}

	mRes, err := render.Migrate([]string{tempFile.Name()}, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to migrate IstioOperator: %w", err)
	}

	buildName := func(p string) string {
		return fmt.Sprintf("iop-%s-%s", in.GetName(), p)
	}

	version := "1.22.8"
	if tag := in.Spec.GetTag().GetStringValue(); tag != "" {
		version = tag
	}

	components := make([]*operatorv1alpha1.HelmComponent, 0, len(mRes.Components))
	var globalValues *structpb.Struct
	gwIndex := 0
	for _, cInfo := range mRes.Components {
		if globalValues == nil {
			vals, _ := cInfo.Values.GetPathMap("spec.values")
			values := structToMap(vals)
			componentValuesStruct, err := structpb.NewStruct(values)
			if err != nil {
				log.Error(err, "Failed to convert component values to structpb.Struct", "values", values)
			} else {
				globalValues = componentValuesStruct
			}
		}

		componentValues := make(map[string]interface{})
		if isGateway(cInfo) {
			gwComp := &operatorv1alpha1.HelmComponent{
				Name:                   cInfo.ComponentSpec.Name,
				Chart:                  "gateway",
				Version:                version,
				EnableSchemaValidation: true, // Enable schema validation for gateway components
			}

			labels := map[string]string{}

			// Extract gateway configuration
			componentsKey := fmt.Sprintf("spec.components.%s", cInfo.Component.SpecName)
			if componentsGateway, ok := cInfo.Values.GetPath(componentsKey); ok {
				// Convert to GatewayComponentSpec using JSON marshal/unmarshal
				jsonBytes, err := json.Marshal(componentsGateway)
				if err != nil {
					log.Error(err, "Failed to marshal gateway", componentsKey)
					continue
				}

				var gws []*apis.GatewayComponentSpec
				if err := json.Unmarshal(jsonBytes, &gws); err != nil {
					log.Error(err, "Failed to unmarshal to GatewayComponentSpec")
					continue
				}

				if len(gws) > gwIndex {
					k8sValuesMap := structToMap(gws[gwIndex].Kubernetes)
					for k, v := range k8sValuesMap {
						if k == "env" {
							continue
						}
						componentValues[k] = v
					}

					// env processing
					envValues := map[string]string{}
					if gws[gwIndex].Kubernetes != nil {
						for _, e := range gws[gwIndex].Kubernetes.Env {
							envValues[e.Name] = e.Value
						}
						componentValues["env"] = structToMap(envValues)
					}

					if gws[gwIndex].Label != nil {
						labels = gws[gwIndex].Label
					}
					gwIndex++
				}
			}

			// spec.values.gateways.istio-ingressgateway.autoscaleEnabled
			// spec.values.gateways.istio-egressgateway.autoscaleEnabled
			autoscaleEnabledKey := fmt.Sprintf("spec.values.%s.autoscaleEnabled", cInfo.Component.ToHelmValuesTreeRoot)
			minReplicasKey := fmt.Sprintf("spec.values.%s.autoscaleMin", cInfo.Component.ToHelmValuesTreeRoot)
			// Configure autoscaling settings
			if enabled := cInfo.Values.GetPathBool(autoscaleEnabledKey); enabled {
				autoscaling := map[string]interface{}{
					"enabled": enabled,
				}

				if minReplicas := cInfo.Values.GetPathString(minReplicasKey); minReplicas != "" {
					autoscaling["minReplicas"] = minReplicas
				}

				componentValues["autoscaling"] = autoscaling
			}

			// compatible iop  gateway template
			appValue := "istio-ingressgateway"
			istioValue := "ingressgateway"
			if isEgressGateway(cInfo) {
				appValue = "istio-egressgateway"
				istioValue = "egressgateway"
			}
			if _, ok := labels["app"]; !ok {
				labels["app"] = appValue
			}
			if _, ok := labels["istio"]; !ok {
				labels["istio"] = istioValue
			}
			componentValues["labels"] = structToMap(labels)

			// Convert values to struct
			if componentValuesStruct, err := structpb2.NewStruct(componentValues); err != nil {
				log.Error(err, "Failed to convert gateway component values to struct")
			} else {
				gwComp.ComponentValues = componentValuesStruct
				components = append(components, gwComp)
			}
			continue
		}

		componentK8SKey := fmt.Sprintf("spec.components.%s.k8s", cInfo.Component.SpecName)
		if componentsGateway, ok := cInfo.Values.GetPath(componentK8SKey); ok {
			// Convert to GatewayComponentSpec using JSON marshal/unmarshal
			jsonBytes, err := json.Marshal(componentsGateway)
			if err != nil {
				log.Error(err, "Failed to marshal", componentK8SKey)
				continue
			}

			var k8s apis.KubernetesResources
			if err := json.Unmarshal(jsonBytes, &k8s); err != nil {
				log.Error(err, "Failed to unmarshal to GatewayComponentSpec")
				continue
			}

			k8sValuesMap := structToMap(&k8s)
			iopC := getComponent(cInfo.Component.SpecName)
			if iopC.HelmBaseRootKey != "" {
				baseValues := map[string]interface{}{}
				for k, v := range k8sValuesMap {
					baseValues[k] = v
				}
				componentValues[iopC.HelmBaseRootKey] = baseValues
			} else {
				for k, v := range k8sValuesMap {
					componentValues[k] = v
				}
			}
		}

		name := cInfo.Component.ReleaseName
		if name == "" {
			log.Error(fmt.Errorf("invalid component name"), "Component name is empty")
			continue
		}

		helmComp := &operatorv1alpha1.HelmComponent{
			Name:    buildName(name),
			Chart:   name,
			Version: version,
		}
		if len(componentValues) > 0 {
			componentValuesStruct, err := structpb2.NewStruct(componentValues)
			if err != nil {
				log.Error(err, "Failed to convert gateway component values to struct")
			}
			helmComp.ComponentValues = componentValuesStruct
		}

		components = append(components, helmComp)
	}

	if len(components) == 0 {
		return nil, fmt.Errorf("no valid components found")
	}

	repo := "https://istio-release.storage.googleapis.com/charts"
	if v := in.GetAnnotations()[constants.IOPSourceRepoLabel]; v != "" {
		repo = v
	}
	happ := &operatorv1alpha1.HelmApp{
		ObjectMeta: v1.ObjectMeta{
			Name:      in.GetName(),
			Namespace: in.GetNamespace(),
			Labels: map[string]string{
				constants.ManagedLabel:           constants.ManagedLabelValue,
				constants.AllowForceUpgradeLabel: "true",
				constants.SourceFromIOP:          in.GetName(),
			},
		},
		Spec: &operatorv1alpha1.HelmAppSpec{
			Components:   components,
			GlobalValues: globalValues,
			Repo: &operatorv1alpha1.HelmRepo{
				Name: "istio",
				Url:  repo,
			},
		},
	}

	if labels := in.GetLabels(); labels != nil {
		if v, ok := labels[constants.AllowForceUpgradeLabel]; ok {
			happ.Labels[constants.AllowForceUpgradeLabel] = v
		}
	}

	return happ, nil
}

func (r *IstioOperatorReconciler) createOrUpdateHelmApp(ctx context.Context, helmApp *operatorv1alpha1.HelmApp) error {
	log := log.FromContext(ctx)

	// Check if the HelmApp already exists
	existingHelmApp := &operatorv1alpha1.HelmApp{}
	err := r.Get(ctx, client.ObjectKey{Namespace: helmApp.Namespace, Name: helmApp.Name}, existingHelmApp)
	if err != nil {
		if errors.IsNotFound(err) {
			// HelmApp doesn't exist, create it
			log.Info("Creating new HelmApp", "namespace", helmApp.Namespace, "name", helmApp.Name)
			if err := r.Create(ctx, helmApp); err != nil {
				return fmt.Errorf("failed to create HelmApp: %w", err)
			}
			return nil
		}
		// Error reading the object - requeue the request
		return fmt.Errorf("failed to get HelmApp: %w", err)
	}

	managed := false
	if existingHelmApp.Labels != nil && existingHelmApp.Labels[constants.ManagedLabel] == constants.ManagedLabelValue {
		managed = true
	}
	// HelmApp exists, check if update is needed
	if managed && (!reflect.DeepEqual(existingHelmApp.Labels, helmApp.Labels) ||
		!reflect.DeepEqual(existingHelmApp.Spec, helmApp.Spec)) {
		log.Info("Updating existing HelmApp", "namespace", helmApp.Namespace, "name", helmApp.Name)
		existingHelmApp.Labels = helmApp.Labels
		existingHelmApp.Spec = helmApp.Spec
		if err := r.Update(ctx, existingHelmApp); err != nil {
			return fmt.Errorf("failed to update HelmApp: %w", err)
		}
	} else {
		log.Info("No changes detected, skipping update", "namespace", helmApp.Namespace, "name", helmApp.Name)
	}

	return nil
}

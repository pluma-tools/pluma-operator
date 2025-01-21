# Pluma Operator

A Comprehensive Helm and Istio Operator

The Pluma Operator is a sophisticated Kubernetes operator designed to enhance component management through the use of Helm charts. It ensures continuous lifecycle management for components and facilitates the transformation of Istio Custom Resource Definitions (CRDs) into HelmApp resources, simplifying Istio installations.

## Key Features

1. **Helm Integration**: Leverages Helm charts for consistent and efficient component deployment.
2. **Lifecycle Management**: Ensures regular maintenance and updates for deployed components.
3. **Istio Support**: Transforms Istio CRDs into HelmApp resources, supporting suite-based Istio installations.
4. **Kubernetes Native**: Fully integrates with Kubernetes environments for seamless operations.

## Installation

To install the Pluma Operator, run the following commands:

```bash
export VERSION=v0.1.0
helm repo add pluma-charts https://pluma-tools.github.io/charts
helm repo update
helm upgrade --install pluma-operator pluma-charts/pluma-operator --version=${VERSION} --create-namespace --namespace pluma-system
```

## Getting Started

### Install Istio

Use IstioOperator CRD

#### Istio Mesh Demo

```yaml
apiVersion: install.istio.io/v1alpha1
kind: IstioOperator
metadata:
  name: demo-mesh
  namespace: istio-system
spec:
  components:
    ingressGateways:
      - enabled: true
        k8s:
          resources:
            limits:
              cpu: 1000m
              memory: 900Mi
            requests:
              cpu: 50m
              memory: 50Mi
        name: istio-ingressgateway
    pilot:
      k8s:
        resources:
          limits:
            cpu: 1500m
            memory: 1500Mi
          requests:
            cpu: 200m
            memory: 200Mi
  namespace: istio-system
  profile: default
  tag: 1.23.4
  values:
    global:
      istioNamespace: istio-system
      meshID: demo-mesh
```

#### Istio Gateway Demo

```yaml
apiVersion: install.istio.io/v1alpha1
kind: IstioOperator
metadata:
  name: test-gw
  namespace: default
spec:
  components:
    ingressGateways:
      - enabled: true
        k8s:
          replicaCount: 1
          resources:
            limits:
              cpu: 600m
              memory: 200Mi
            requests:
              cpu: 200m
              memory: 200Mi
          service:
            ports:
              - name: http-0
                port: 80
                protocol: TCP
                targetPort: 8080
            type: NodePort
        label:
          test-gw: test-gw
        name: test-gw
        namespace: default
  profile: empty
  tag: 1.23.4
  values:
    gateways:
      istio-ingressgateway:
        autoscaleEnabled: false
        injectionTemplate: gateway
```

## Common helm application

```yaml
apiVersion: operator.pluma.io/v1alpha1
kind: HelmApp
metadata:
  name: helm-demo
  namespace: default
spec:
  components:
    - chart: gateway
      componentValues:
        resources:
          limits:
            cpu: 600m
            memory: 200Mi
          requests:
            cpu: 200m
            memory: 200Mi
      name: demo
      version: 1.23.4
  globalValues:
    global:
      meshID: demo-mesh
  repo:
    name: istio
    url: https://istio-release.storage.googleapis.com/charts    
```

## HelmApp CRD

### Status
```yaml
status:
  components:
  - name: demo
    resources:
    - apiVersion: v1
      kind: ServiceAccount
      name: demo
      namespace: default
    - apiVersion: rbac.authorization.k8s.io/v1
      kind: Role
      name: demo
      namespace: default
    - apiVersion: rbac.authorization.k8s.io/v1
      kind: RoleBinding
      name: demo
      namespace: default
    - apiVersion: v1
      kind: Service
      name: demo
      namespace: default
    - apiVersion: apps/v1
      kind: Deployment
      name: demo
      namespace: default
    - apiVersion: autoscaling/v2
      kind: HorizontalPodAutoscaler
      name: demo
      namespace: default
    resourcesTotal: 6
    status: deployed
    version: "1"
  phase: SUCCEEDED
```

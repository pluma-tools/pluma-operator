# Pluma Operator
[![LICENSE](https://img.shields.io/github/license/pluma-tools/pluma-operator.svg?style=flat-square)](/LICENSE)

A Comprehensive Helm and Istio lightweight Operator

:rocket: We Welcome Your Feedback! :heart:

## Background

With the [official deprecation of the Istio In-Cluster Operator](https://istio.io/latest/blog/2024/in-cluster-operator-deprecation-announcement/), the need for a more lightweight and maintainable installation solution has become increasingly important. **Pluma Operator** focuses on leveraging standard Helm to simplify and streamline Istio installation and management, offering users a more efficient and straightforward experience.

## Key Features

- **Istio Support**: Fully compatible with **IstioOperator CRD**, ensuring alignment with community standards
- **Lightweight Design**: Utilizes Helm internally to enable automated installation and management, reducing complexity and maintenance overhead
- **Lifecycle Management**: Ensures regular maintenance and updates for deployed components.
- **Kubernetes Native**: Fully integrates with Kubernetes environments for seamless operations.

## Installation

To install the Pluma Operator, run the following commands:

```bash
export VERSION=v0.1.0
helm repo add pluma-charts https://pluma-tools.github.io/charts
helm repo update
helm upgrade --install pluma-operator pluma-charts/pluma-operator --version=${VERSION} --create-namespace --namespace pluma-system
```

### 版本维护

PR 合并后，有仓库写权限的成员可在 PR 评论 `/cherry-pick release-0.1`，自动创建以目标发布分支为 base 的 cherry-pick PR。目标分支须已存在；冲突时工作流失败，需手动处理。多提交 PR 以 squash 或 rebase 合并时，GitHub 返回的单个合并提交无法安全区分这两种情况，需手动 cherry-pick。工作流使用仓库已有的 `APP_ID` 和 `APP_PRIVATE_KEY`，对应 GitHub App 须安装在本仓库并具有 Contents 和 Pull requests 写权限。

在 GitHub Actions 的 **Create release** 中运行工作流，填写发布分支（如 `release-0.1`）、新标签（如 `v0.1.6`），可选填写用于比较的上一个标签。工作流从指定分支提交创建 GitHub Release，由 GitHub 根据标签差异生成发行说明，并启动现有镜像和 Chart 发布工作流。标签必须与分支的主次版本一致且尚不存在；指定上一个标签时，它必须是发布提交的祖先。发布流程的完成状态请查看 **Release Latest** 工作流。

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

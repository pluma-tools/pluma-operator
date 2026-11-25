#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")/.."
for tool in docker kind kubectl helm jq go; do
  command -v "$tool" >/dev/null || { echo "Missing tool: $tool" >&2; exit 1; }
done

work_dir=$(mktemp -d)
operator_pid=
forward_pid=
cluster=
cleanup() {
  local status=$?
  trap - EXIT
  if [[ -n "$forward_pid" ]]; then
    kill "$forward_pid" 2>/dev/null || true
    wait "$forward_pid" 2>/dev/null || true
  fi
  if [[ -n "$operator_pid" ]]; then
    kill "$operator_pid" 2>/dev/null || true
    wait "$operator_pid" 2>/dev/null || true
  fi
  if [[ -n "$cluster" ]]; then
    if (( status != 0 )); then
      kubectl -n istio-system get istiooperators,helmapps,pods -o wide >&2 || true
      kubectl -n istio-system get events --sort-by=.lastTimestamp >&2 || true
      if [[ ${TEST_AMBIENT:-0} == 1 ]]; then
        kubectl -n ambient-test get pods -o wide >&2 || true
        kubectl -n ambient-outside get pods -o wide >&2 || true
      fi
      tail -100 "$work_dir/operator.log" >&2 2>/dev/null || true
      kind export logs "$work_dir/kind-logs" --name "$cluster" || true
    fi
    kind delete cluster --name "$cluster" || true
  fi
  if (( status == 0 )); then
    rm -rf "$work_dir"
  else
    echo "Failure artifacts: $work_dir" >&2
  fi
  exit "$status"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

go build -o "$work_dir/operator" ./cmd
if [[ ${TEST_LIFECYCLE:-0} == 1 ]]; then
  for tool in git curl; do
    command -v "$tool" >/dev/null || { echo "Missing tool: $tool" >&2; exit 1; }
  done
  baseline_ref=${BASELINE_REF:-v0.1.5}
  mkdir "$work_dir/baseline"
  git archive "$baseline_ref" | tar -x -C "$work_dir/baseline"
  echo "Building baseline Operator at $baseline_ref"
  (cd "$work_dir/baseline" && go build -o "$work_dir/operator-baseline" ./cmd)
fi
export KUBECONFIG="$work_dir/kubeconfig"
export HELM_CACHE_HOME="$work_dir/helm/cache"
export HELM_CONFIG_HOME="$work_dir/helm/config"
export HELM_DATA_HOME="$work_dir/helm/data"

install_istio() {
  local version=$1 node_image=$2
  local timeout=${INSTALL_TIMEOUT:-600}
  local namespace=istio-system
  cluster="pluma-install-${version//./-}-$$"
  rm -f "$KUBECONFIG"
  echo "Testing Istio $version on $node_image"
  kind create cluster --name "$cluster" --image "$node_image" --kubeconfig "$KUBECONFIG" --wait 3m
  helm template pluma-install manifests/pluma-operator --set global.enabledIstioOperatorCRD=true \
    --show-only templates/iop.istio.io.yaml \
    --show-only templates/operator.pluma.io_helmapps.yaml | kubectl apply -f -
  kubectl wait --for=condition=Established --timeout=60s \
    crd/istiooperators.install.istio.io crd/helmapps.operator.pluma.io
  if [[ ${PRELOAD_IMAGES:-0} == 1 ]]; then
    local image
    local images=("docker.io/istio/pilot:$version" "docker.io/istio/proxyv2:$version")
    if [[ ${TEST_AMBIENT:-0} == 1 && "$version" == 1.30.8 ]]; then
      images+=("docker.io/istio/install-cni:$version" "docker.io/istio/ztunnel:$version")
    fi
    for image in "${images[@]}"; do
      docker image inspect "$image" >/dev/null 2>&1 || docker pull "$image"
      kind load docker-image "$image" --name "$cluster"
    done
  fi
  kubectl create namespace "$namespace"
  if [[ ${TEST_AMBIENT:-0} == 1 && "$version" == 1.30.8 ]]; then
    # shellcheck source=scripts/test-kind-ambient.sh
    source scripts/test-kind-ambient.sh
    check_ambient "$work_dir" "$namespace" "$version" "$timeout" "$cluster"
    stop_operator
    kind delete cluster --name "$cluster"
    cluster=
    return
  fi
  cat >"$work_dir/iop.yaml" <<YAML
apiVersion: install.istio.io/v1alpha1
kind: IstioOperator
metadata:
  name: install-smoke
  annotations:
    pluma.io/source-repo: https://blob.istio.io/istio-release/charts
spec:
  profile: default
  hub: docker.io/istio
  tag: "$version"
  values:
    imagePullPolicy: IfNotPresent
    global:
      imagePullPolicy: IfNotPresent
      meshID: pluma-kind
  components:
    ingressGateways:
      - name: istio-ingressgateway
        enabled: true
        label:
          test.pluma.io/gateway: installation
        k8s:
          replicaCount: 1
          service:
            type: ClusterIP
            ports:
              - name: http
                port: 8080
                targetPort: 8080
YAML

  if [[ ${TEST_LIFECYCLE:-0} == 1 && "$version" == 1.23.4 ]]; then
    start_operator "$work_dir/operator-baseline"
    kubectl -n "$namespace" apply -f "$work_dir/iop.yaml"
    check_install
    check_operator_upgrade
  else
    start_operator "$work_dir/operator"
    kubectl -n "$namespace" apply -f "$work_dir/iop.yaml"
    check_install
  fi

  stop_operator
  kind delete cluster --name "$cluster"
  cluster=
}

start_operator() {
  "$1" --metrics-bind-address=0 --health-probe-bind-address=0 >"$work_dir/operator.log" 2>&1 &
  operator_pid=$!
}

stop_operator() {
  kill "$operator_pid"
  wait "$operator_pid" || true
  operator_pid=
}

wait_for_iop_health() {
  local deadline=$((SECONDS + timeout))
  until [[ $(kubectl -n "$namespace" get istiooperator "$1" -o jsonpath='{.status.status}') == HEALTHY ]]; do
    if (( SECONDS >= deadline )) || ! kill -0 "$operator_pid" 2>/dev/null; then
      echo "IOP failed to become HEALTHY for Istio $version" >&2
      return 1
    fi
    sleep 3
  done
}

check_install() {
  kubectl -n "$namespace" get istiooperator install-smoke -o json |
    jq -e --arg version "$version" '
      .spec.tag == $version
    ' >/dev/null

  wait_for_iop_health install-smoke
  helm list -n "$namespace" -o json | jq -e --arg version "$version" '
    (map(.name) | sort) == ["iop-install-smoke-base", "iop-install-smoke-istiod", "istio-ingressgateway"] and
    all(.[]; .status == "deployed") and
    (map(.chart) | sort) == (["base-", "gateway-", "istiod-"] | map(. + $version))
  ' >/dev/null
  kubectl -n "$namespace" rollout status deployment/istiod --timeout="${timeout}s"
  kubectl -n "$namespace" rollout status deployment/istio-ingressgateway --timeout="${timeout}s"
  kubectl -n "$namespace" get pods -l 'app in (istiod,istio-ingressgateway)' -o json |
    jq -e --arg version "$version" '
      any(.items[]; .metadata.labels.app == "istiod" and
        any(.spec.containers[]; .name == "discovery" and .image == ("docker.io/istio/pilot:" + $version))) and
      any(.items[]; .metadata.labels.app == "istio-ingressgateway" and
        any(.spec.containers[]; .name == "istio-proxy" and .image == ("docker.io/istio/proxyv2:" + $version)))
    ' >/dev/null
  kubectl -n "$namespace" get service istio-ingressgateway -o json |
    jq -e 'any(.spec.ports[]; .port == 8080)' >/dev/null
  kubectl get crd virtualservices.networking.istio.io >/dev/null
  kubectl get mutatingwebhookconfiguration istio-sidecar-injector >/dev/null
  echo "Istio $version installation passed"
}

check_traffic() {
  local deadline=$((SECONDS + 90))
  local address=
  kubectl -n "$namespace" port-forward svc/istio-ingressgateway :8080 >"$work_dir/port-forward.log" 2>&1 &
  forward_pid=$!
  while (( SECONDS < deadline )); do
    address=$(sed -n 's/^Forwarding from \(127\.0\.0\.1:[0-9]*\) -> .*/\1/p' "$work_dir/port-forward.log")
    if [[ -n "$address" ]] && [[ $(curl --noproxy '*' -fsS --max-time 3 "http://$address/" 2>/dev/null) == pluma-kind-ok ]]; then
      kill "$forward_pid"
      wait "$forward_pid" || true
      forward_pid=
      return 0
    fi
    kill -0 "$forward_pid" 2>/dev/null || break
    sleep 2
  done
  cat "$work_dir/port-forward.log" >&2
  echo "Gateway traffic probe failed" >&2
  return 1
}

snapshot_install() {
  local output_dir=$1 release
  mkdir -p "$output_dir"
  helm list -n "$namespace" -o json |
    jq -S 'sort_by(.name) | map({name, namespace, chart, status, revision})' >"$output_dir/releases.json"
  kubectl -n "$namespace" get istiooperator install-smoke -o json |
    jq -S '{uid: .metadata.uid, spec}' >"$output_dir/iop.json"
  kubectl -n "$namespace" get helmapp install-smoke -o json |
    jq -S .spec >"$output_dir/helmapp.json"
  kubectl -n "$namespace" get deployment istiod istio-ingressgateway -o json |
    jq -S '[.items[] | {name: .metadata.name, uid: .metadata.uid}] | sort_by(.name)' >"$output_dir/deployments.json"
  for release in iop-install-smoke-base iop-install-smoke-istiod istio-ingressgateway; do
    helm get values "$release" -n "$namespace" --all -o json |
      jq -S . >"$output_dir/$release.json"
    helm get manifest "$release" -n "$namespace" >"$output_dir/$release.yaml"
  done
}

check_operator_upgrade() {
  local echo_image=docker.io/library/busybox:1.37.0
  docker image inspect "$echo_image" >/dev/null 2>&1 || docker pull "$echo_image"
  kind load docker-image "$echo_image" --name "$cluster"
  kubectl -n "$namespace" create deployment traffic-echo --image="$echo_image" -- \
    sh -c 'mkdir -p /www; printf pluma-kind-ok > /www/index.html; exec httpd -f -p 8080 -h /www'
  kubectl -n "$namespace" expose deployment traffic-echo --port=8080
  kubectl -n "$namespace" rollout status deployment/traffic-echo --timeout=120s
  kubectl -n "$namespace" apply -f - <<'YAML'
apiVersion: networking.istio.io/v1
kind: Gateway
metadata:
  name: install-smoke
spec:
  selector:
    istio: ingressgateway
  servers:
    - port:
        number: 8080
        name: http
        protocol: HTTP
      hosts: ["*"]
---
apiVersion: networking.istio.io/v1
kind: VirtualService
metadata:
  name: install-smoke
spec:
  hosts: ["*"]
  gateways: [install-smoke]
  http:
    - route:
        - destination:
            host: traffic-echo
            port:
              number: 8080
YAML
  check_traffic
  snapshot_install "$work_dir/before"
  stop_operator
  mv "$work_dir/operator.log" "$work_dir/operator-baseline.log"
  # A no-op reconcile can leave resourceVersion unchanged. Wait for both
  # controllers to handle the existing objects before comparing snapshots.
  local deadline
  start_operator "$work_dir/operator"
  deadline=$((SECONDS + timeout))
  until grep -Eq 'No changes detected, skipping update|Updating existing HelmApp' "$work_dir/operator.log" &&
    grep -Eq '(No changes detected, skipping upgrade|Upgraded release|Installed release).*"component": "istio-ingressgateway"' "$work_dir/operator.log"; do
    if (( SECONDS >= deadline )) || ! kill -0 "$operator_pid" 2>/dev/null; then
      echo "Candidate Operator did not reconcile the existing HelmApp" >&2
      return 1
    fi
    sleep 2
  done
  check_install
  check_traffic
  snapshot_install "$work_dir/after"
  diff -ru "$work_dir/before" "$work_dir/after"
  echo "Istio 1.23.4 Operator-only upgrade passed"
}

if (( $# == 0 )); then
  set -- 1.23.4 1.30.8 1.32.0
fi
for version in "$@"; do
  case "$version" in
    1.23.*) node_image=${KIND_NODE_IMAGE_123:-kindest/node:v1.30.4} ;;
    1.30.*|1.32.*) node_image=${KIND_NODE_IMAGE_MODERN:-kindest/node:v1.33.4} ;;
    *) echo "No Kind node image configured for Istio $version" >&2; exit 2 ;;
  esac
  install_istio "$version" "$node_image"
done

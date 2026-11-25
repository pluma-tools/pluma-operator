#!/usr/bin/env bash
# Sourced by test-kind-install.sh after the cluster and Operator CRDs are ready.

check_ambient() {
  local work_dir=$1 namespace=$2 version=$3 timeout=$4 cluster=$5
  command -v curl >/dev/null || { echo "Missing tool: curl" >&2; return 1; }
  start_operator "$work_dir/operator"
  kubectl -n "$namespace" apply -f - <<YAML
apiVersion: install.istio.io/v1alpha1
kind: IstioOperator
metadata:
  name: ambient-smoke
spec:
  hub: docker.io/istio
  tag: "$version"
  components:
    cni: {enabled: true}
    ztunnel: {enabled: true}
    ingressGateways:
      - {name: istio-ingressgateway, enabled: false}
  meshConfig:
    defaultConfig:
      proxyMetadata:
        ISTIO_META_ENABLE_HBONE: "true"
  values:
    hub: docker.io/istio
    imagePullPolicy: IfNotPresent
    global:
      imagePullPolicy: IfNotPresent
    pilot:
      env:
        PILOT_ENABLE_AMBIENT: "true"
        PILOT_ENABLE_HBONE: "true"
        CA_TRUSTED_NODE_ACCOUNTS: istio-system/ztunnel,kube-system/ztunnel
    cni:
      ambient:
        enabled: true
YAML
  wait_for_iop_health ambient-smoke
  helm list -n "$namespace" -o json | jq -e --arg version "$version" '
    all(.[]; .status == "deployed") and
    (map(.chart) | sort) == (["base-", "cni-", "istiod-", "ztunnel-"] | map(. + $version))
  ' >/dev/null
  kubectl -n "$namespace" rollout status deployment/istiod --timeout="${timeout}s"
  local daemonset
  for daemonset in istio-cni-node ztunnel; do
    kubectl -n "$namespace" rollout status "daemonset/$daemonset" --timeout="${timeout}s"
    kubectl -n "$namespace" get "daemonset/$daemonset" -o json | jq -e '
      .status.desiredNumberScheduled > 0 and .status.numberReady == .status.desiredNumberScheduled
    ' >/dev/null
  done
  echo "Ambient CNI and ztunnel are ready"

  local image=docker.io/library/busybox:1.37.0
  docker image inspect "$image" >/dev/null 2>&1 || docker pull "$image"
  kind load docker-image "$image" --name "$cluster"
  kubectl create namespace ambient-test
  kubectl label namespace ambient-test istio.io/dataplane-mode=ambient
  kubectl create namespace ambient-outside
  kubectl -n ambient-test create deployment client --image="$image" -- sleep 3600
  kubectl -n ambient-outside create deployment client --image="$image" -- sleep 3600
  kubectl -n ambient-test create deployment echo --image="$image" -- \
    sh -c 'mkdir -p /www; printf ambient-ok > /www/index.html; exec httpd -f -p 8080 -h /www'
  kubectl -n ambient-test expose deployment echo --port=8080
  kubectl -n ambient-test rollout status deployment/client --timeout=120s
  kubectl -n ambient-test rollout status deployment/echo --timeout=120s
  kubectl -n ambient-outside rollout status deployment/client --timeout=120s

  local pods client server deadline address
  pods=$(kubectl -n ambient-test get pods -o json)
  jq -e '.items | length == 2 and all(.[]; all(.spec.containers[]; .name != "istio-proxy"))' <<<"$pods" >/dev/null
  client=$(jq -er '.items[] | select(.metadata.labels.app == "client") | .metadata.name' <<<"$pods")
  server=$(jq -er '.items[] | select(.metadata.labels.app == "echo") | .metadata.name' <<<"$pods")
  kubectl -n "$namespace" port-forward daemonset/ztunnel :15000 >"$work_dir/ztunnel-forward.log" 2>&1 &
  forward_pid=$!
  deadline=$((SECONDS + 90))
  while :; do
    address=$(sed -n 's/^Forwarding from \(127\.0\.0\.1:[0-9]*\) -> .*/\1/p' "$work_dir/ztunnel-forward.log")
    if [[ -n "$address" ]] && curl --noproxy '*' -fsS --max-time 3 "http://$address/config_dump" >"$work_dir/ztunnel.json" &&
      jq -e --arg client "$client" --arg server "$server" '
        [.workloads[] | select(.namespace == "ambient-test" and (.protocol | ascii_upcase) == "HBONE") | .name] |
        index($client) != null and index($server) != null
      ' "$work_dir/ztunnel.json" >/dev/null; then
      break
    fi
    if (( SECONDS >= deadline )) || ! kill -0 "$forward_pid" 2>/dev/null; then
      echo "ztunnel did not report both workloads as HBONE" >&2
      cat "$work_dir/ztunnel-forward.log" >&2
      return 1
    fi
    sleep 2
  done
  kill "$forward_pid"
  wait "$forward_pid" || true
  forward_pid=
  echo "ztunnel discovered both sidecarless workloads with HBONE"

  local url=http://echo.ambient-test.svc.cluster.local:8080/
  [[ $(ambient_request ambient-test "$url") == ambient-ok ]]
  [[ $(ambient_request ambient-outside "$url") == ambient-ok ]]
  kubectl -n ambient-test apply -f - <<'YAML'
apiVersion: security.istio.io/v1
kind: PeerAuthentication
metadata:
  name: strict
spec:
  mtls:
    mode: STRICT
YAML
  deadline=$((SECONDS + 90))
  until [[ $(ambient_request ambient-outside "$url") == rejected ]]; do
    if (( SECONDS >= deadline )); then
      echo "STRICT mTLS did not reject the client outside the mesh" >&2
      return 1
    fi
    sleep 2
  done
  local attempt
  for ((attempt = 1; attempt <= 5; attempt++)); do
    [[ $(ambient_request ambient-test "$url") == ambient-ok ]]
    [[ $(ambient_request ambient-outside "$url") == rejected ]]
  done
  echo "Ambient STRICT mTLS passed: in-mesh HTTP 5/5, outside client rejected 5/5"
}

ambient_request() {
  # The positional parameter is expanded by the shell in the Pod.
  # shellcheck disable=SC2016
  kubectl -n "$1" exec deployment/client -c busybox -- sh -c '
    if body=$(wget -qO- -T 3 "$1"); then
      printf "%s" "$body"
    else
      printf rejected
    fi
  ' sh "$2"
}

# Istio 兼容回归

升级 Go 和依赖后，使用相同的 IstioOperator 输入应保留原 chart 版本、release 名称、values 和业务流量。未指定 `spec.tag` 时沿用原 chart 默认版本 1.22.8，显式的镜像仓库和 `pluma.io/source-repo` 继续按 IOP 生效。

| mspider 模式 | 回归时核对的参数 |
| --- | --- |
| 专有网格 | chart 仓库、tag、hub、revision、mesh ID、cluster name 和网关配置 |
| 托管控制面 | `profile: minimal`、`pilot.env.KUBECONFIG`、Secret volumes、volume mounts 和 webhook 管理开关 |
| 托管成员 | `global.remotePilotAddress` 和 cluster name。mspider 从 Istio 1.27 起还传入 `istiodRemote.enabled` 和 `enabledLocalInjectorIstiod` |
| Ambient | mspider 展开后的 CNI、ztunnel、禁用 ingress、HBONE 和 Pilot 环境变量 |

Istio 1.24 之前的自定义安装包如果提供独立的 `istiod-remote` chart，沿用原 release 名称。mspider 的托管成员沿用 `istiod` 和现有 remote values。

## 本地验证

```bash
go test ./...
TEST_LIFECYCLE=1 ./scripts/test-kind-install.sh 1.23.4
TEST_AMBIENT=1 KIND_NODE_IMAGE_MODERN=registry.istio.io/testing/kind-node:v1.35.0 \
  ./scripts/test-kind-install.sh 1.30.8
```

Kind 用例需要 Docker、Kind、kubectl、Helm、jq 和 Go。升级用例还需要 curl，默认用 `v0.1.5` 部署网格，再只替换 Operator。脚本对比 HelmApp spec、Helm revisions、values、manifests 和 Deployment UID，并复测入口流量。可以用 `BASELINE_REF` 指定其他基线版本。

Ambient 用例检查 CNI 和 ztunnel 就绪，并验证 STRICT mTLS 下的网格内外流量。设置 `PRELOAD_IMAGES=1` 可由宿主机拉取并预载镜像。托管场景还需用可达的托管 API 和成员集群验证 Pilot 的实际连接和跨集群流量。

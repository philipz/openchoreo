# OpenChoreo OTLP Collector Manifests

Kustomize base for deploying a node-local OpenTelemetry Collector that tails Kubernetes container logs, enriches them with the standard OpenChoreo labels, and forwards everything to the ClickStack ingress gateway.

## Usage

```bash
# Deploy to the default namespace (openchoreo-observability-plane)
kubectl apply -k config/observability/collectors/otel/base
```

Customize the namespace or exporter endpoint by editing `kustomization.yaml` or patching the `collector-configmap.yaml`.

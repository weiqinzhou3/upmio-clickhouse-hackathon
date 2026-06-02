# ClickHouse Monitoring Runtime Validation

## Purpose

Validate whether UPMIO can create monitoring integration resources for ClickHouse and whether ClickHouse exposes a usable metrics endpoint.

## Commands executed

```bash
kubectl get crd | grep -Ei 'servicemonitor|podmonitor|prometheus|prometheusrule'
kubectl get podmonitor -A || true
kubectl get servicemonitor -A || true
kubectl apply -f monitoring.coreos.com_podmonitors.yaml
kubectl apply -f monitoring.coreos.com_servicemonitors.yaml
kubectl apply -f monitoring.coreos.com_prometheusrules.yaml
kubectl annotate unitset clickhouse-runtime -n upm-clickhouse-runtime runtime-validation/reconcile-ts="$(date +%s)" --overwrite
kubectl get podmonitor clickhouse-runtime-exporter-podmon -n upm-clickhouse-runtime -o yaml
kubectl exec -n upm-clickhouse-runtime clickhouse-runtime-0 -c clickhouse -- curl -sS --max-time 5 http://127.0.0.1:9363/metrics
kubectl exec -n upm-clickhouse-runtime clickhouse-runtime-0 -c clickhouse -- curl -sS --max-time 5 http://127.0.0.1:8123/metrics
```

Prometheus Operator CRDs installed only:

```text
Prometheus Operator CRD version: v0.80.0
Reason: unit-operator/go.mod depends on github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring v0.80.0
```

Full Prometheus Server/Grafana were not installed in this phase because image availability was a known environment risk and PodMonitor behavior could be validated with CRDs only.

## Runtime evidence

Before CRD installation:

```text
error: the server doesn't have a resource type "podmonitor"
error: the server doesn't have a resource type "servicemonitor"
```

Installed CRDs:

```text
podmonitors.monitoring.coreos.com
servicemonitors.monitoring.coreos.com
prometheusrules.monitoring.coreos.com
```

PodMonitor created by UnitSet:

```yaml
apiVersion: monitoring.coreos.com/v1
kind: PodMonitor
metadata:
  name: clickhouse-runtime-exporter-podmon
  namespace: upm-clickhouse-runtime
  ownerReferences:
  - kind: UnitSet
    name: clickhouse-runtime
spec:
  namespaceSelector:
    matchNames:
    - upm-clickhouse-runtime
  podMetricsEndpoints:
  - port: metrics
  selector:
    matchLabels:
      unit-operator/unitset.name: clickhouse-runtime
```

Service port evidence:

```text
name: metrics
port: 9363
targetPort: 9363
```

Metrics endpoint test:

```text
curl http://127.0.0.1:9363/metrics -> Failed to connect to 127.0.0.1 port 9363: Connection refused
curl http://127.0.0.1:8123/metrics -> There is no handle /metrics.
```

Config probe:

```text
grep 'prometheus|9363|metrics' /var/lib/clickhouse/conf/config.xml -> no matches
```

Source evidence:

```text
unit-operator/api/v1alpha2/unitset_types.go:95-123 defines spec.podMonitor.
unit-operator/pkg/controller/unitset/unitset_podmonitor.go:21-35 skips creation when PodMonitor CRD is absent.
unit-operator/pkg/controller/unitset/unitset_podmonitor.go:49-65 creates PodMonitor when CRD exists.
```

## Key findings

UPMIO provides PodMonitor integration, not a full monitoring stack.

UPMIO did not install:

- Prometheus Server.
- Grafana.
- dashboards.
- alert rules for this ClickHouse runtime.

`UnitSet.spec.podMonitor` works after PodMonitor CRD exists. However, ClickHouse package did not configure a Prometheus metrics endpoint in `config.xml`, so Prometheus would discover a target that is not scrapeable.

No `ServiceMonitor` was created for ClickHouse; the observed integration is PodMonitor only.

## Open questions

- Should UPMIO package templates enable ClickHouse `<prometheus>` configuration?
- Should the future manager own dashboards and alerts, or only create monitor resources and leave Prometheus/Grafana external?
- Should the hackathon environment install full kube-prometheus-stack once image import is solved?

## Conclusion

Status: PARTIAL.

PodMonitor integration is validated. Actual ClickHouse metrics scraping is not validated because the runtime endpoint is not listening.

## Impact on future ClickHouse Spec

The Spec should declare Prometheus/Grafana as external dependencies unless the product explicitly owns installation. The ClickHouse package or manager must enable a metrics endpoint before dashboards or alerts can be reliable.

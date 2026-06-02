# Phase 05 Monitoring Capability Analysis

## Purpose

Verify monitoring-related capabilities in public UPMIO repositories and the prepared Kubernetes cluster. This document separates integration points from full monitoring stack installation.

## Commands Executed

```bash
grep -R "prometheus\|Prometheus\|ServiceMonitor\|PodMonitor\|Grafana\|dashboard\|metrics\|alert" -n unit-operator compose-operator upm-packages | head -500
find unit-operator compose-operator upm-packages -type f | grep -Ei "monitor|prometheus|grafana|dashboard|alert|servicemonitor|podmonitor|metrics"
kubectl get crd | grep -Ei "servicemonitor|podmonitor|prometheus|grafana|alert" || true
kubectl get pods -A | grep -Ei "prometheus|grafana|monitoring" || true
```

## Key Findings

1. Prometheus Server installation: not found in UPMIO public repos.
2. Grafana installation: not found in UPMIO public repos.
3. ServiceMonitor: present for operator/controller metrics in `compose-operator/config/prometheus/monitor.yaml`; similar kubebuilder layout exists in operator bundles.
4. PodMonitor: supported by `UnitSet.spec.podMonitor` and created by unit-operator only when the Prometheus Operator PodMonitor CRD exists.
5. Database metrics endpoints: present in multiple packages. MySQL has a `mysqld-exporter` sidecar; Redis, MongoDB, Elasticsearch, etc. also have package metrics snippets.
6. Grafana dashboards: no concrete dashboard JSON/files found in public repos.
7. Alert rules: no concrete PrometheusRule files found in public repos.
8. MySQL metrics integration: present through exporter sidecar on port named `metrics`.
9. ClickHouse metrics integration: package has a port named `metrics`, but no verified ClickHouse exporter/dashboard/alert template was found in public ClickHouse package files.
10. Recommended external dependency: Prometheus Operator/kube-prometheus-stack or an existing platform monitoring stack providing `ServiceMonitor` and `PodMonitor` CRDs.

## Evidence

ServiceMonitor evidence from `compose-operator/config/prometheus/monitor.yaml`:

```yaml
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: controller-manager-metrics-monitor
spec:
  endpoints:
    - path: /metrics
      port: https
      scheme: https
```

PodMonitor evidence from `unit-operator/api/v1alpha2/unitset_types.go`:

```go
type UnitSetSpec struct {
    PodMonitor PodMonitorInfo `json:"podMonitor,omitempty"`
}

type PodMonitorInfo struct {
    Enable bool `json:"enable,omitempty"`
    Endpoints []PodMonitorEndpoint `json:"endpoints,omitempty"`
}
```

PodMonitor reconcile evidence from `unit-operator/pkg/controller/unitset/unitset_podmonitor.go`:

```go
if !unitset.Spec.PodMonitor.Enable {
    return nil
}
err := r.Get(ctx, client.ObjectKey{Name: upmiov1alpha2.MonitorPodMonitorCrdName}, &exceptedCrd)
// no crd found, creation of service monitor not supported
pm := &serviceMonitorv1.PodMonitor{ ... Spec: expectedSpec }
```

MySQL metrics evidence from `upm-packages/mysql-community/8.4.5/charts/values.yaml`:

```yaml
metrics:
  image:
    registry: quay.io
    repository: upmio/mysqld-exporter
    tag: "0.16.0"
```

Runtime evidence on the prepared cluster:

```text
kubectl get crd | grep -Ei "servicemonitor|podmonitor|prometheus|grafana|alert" || true
# no monitoring CRDs installed

kubectl get pods -A | grep -Ei "prometheus|grafana|monitoring" || true
# no Prometheus or Grafana pods installed
```

Search evidence for dashboard/alert files:

```text
find unit-operator compose-operator upm-packages -type f | grep -Ei "dashboard|grafana|alert|prometheusrule"
# no concrete Grafana dashboard JSON or PrometheusRule manifest found for UPMIO packages
```

## Required Summary

| Monitoring Capability | Provided by UPMIO? | Repository Evidence | Runtime Evidence | Notes |
|---|---|---|---|---|
| Prometheus Server | no | no Prometheus server chart/manifests found | not installed | Use external Prometheus stack |
| Grafana | no | no Grafana chart/manifests found | not installed | Use external Grafana |
| ServiceMonitor | yes, integration | `compose-operator/config/prometheus/monitor.yaml` | CRD not installed | Requires Prometheus Operator CRDs |
| PodMonitor | yes, conditional | `unitset_podmonitor.go`, `UnitSet.spec.podMonitor` | CRD not installed | Created only when PodMonitor CRD exists |
| dashboards | no/unknown | no dashboard files found | not installed | README claims are not enough evidence |
| alert rules | no/unknown | no PrometheusRule files found | not installed | Needs future design |
| MySQL metrics | yes | `mysqld-exporter` values and metrics port | MySQL not deployed | Package-level exporter support |
| ClickHouse metrics | partial/unknown | ClickHouse package has metrics port references | ClickHouse not deployed | Need verify endpoint/exporter details |

## Open Questions

- Which monitoring stack should the hackathon cluster use: kube-prometheus-stack, standalone Prometheus Operator, or an existing company monitoring platform?
- Should UPMIO packages own database-specific dashboards and alert rules, or should the future manager own them?
- What exact ClickHouse metrics endpoint should be exposed in MVP?

## Conclusion

UPMIO public repos provide monitoring integration points, not a complete monitoring stack. Prometheus Operator CRDs are expected external dependencies for ServiceMonitor/PodMonitor support.

## Impact on Future ClickHouse Spec

The ClickHouse spec should declare Prometheus Operator or kube-prometheus-stack as an external dependency. MVP should include ServiceMonitor/PodMonitor wiring and ClickHouse metrics endpoint verification, while dashboards and alert rules can be separate follow-up work.

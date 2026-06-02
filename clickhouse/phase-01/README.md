# Phase 01 ClickHouse Package Runtime Evidence

This directory contains the hackathon-facing Phase 01 evidence assets.

## What This Phase Proves

- `upm-packages/clickhouse` renders secure ClickHouse config without `<no_password/>`.
- ClickHouse admin/default authentication is backed by `CLICKHOUSE_ADMIN_PASSWORD` in Kubernetes Secret data.
- ClickHouse config no longer renders the invalid `max_concurrent_queries` profile setting.
- ClickHouse native Prometheus endpoint is rendered on `9363` and `/metrics`.
- UPMIO `UnitSet` can run a 3-node Keeper ensemble and a 1 shard x 2 replicas ClickHouse baseline.

## Files

- `manifests/00-namespace-project.yaml`: runtime Namespace and UPMIO Project.
- `manifests/01-runtime-secret.example.yaml`: placeholder-only Secret examples for `aes-secret-key` and runtime credentials.
- `manifests/02-clickhouse-keeper-unitset.yaml`: 3-node Keeper UnitSet.
- `manifests/03-clickhouse-unitset.yaml`: 2-replica ClickHouse UnitSet with Secret/env/log/PodMonitor settings.
- `values/*.yaml`: Helm values used for package rendering validation.
- `modified-files/`: symlinks to modified upstream package source files.
- `scripts/validate-runtime.sh`: read-only runtime acceptance command set.

## Static Validation

```bash
helm dependency build upm-packages/clickhouse/26.3.9.8/charts
helm dependency build upm-packages/clickhouse-keeper/26.3.9.8/charts

helm lint upm-packages/clickhouse/26.3.9.8/charts
helm template clickhouse-runtime upm-packages/clickhouse/26.3.9.8/charts \
  --values clickhouse/phase-01/values/clickhouse-values.yaml >/tmp/clickhouse-rendered.yaml

helm lint upm-packages/clickhouse-keeper/26.3.9.8/charts
helm template clickhouse-keeper-runtime upm-packages/clickhouse-keeper/26.3.9.8/charts \
  --values clickhouse/phase-01/values/clickhouse-keeper-values.yaml >/tmp/clickhouse-keeper-rendered.yaml

! grep -R "<no_password" /tmp/clickhouse-rendered.yaml
! grep -R "max_concurrent_queries" /tmp/clickhouse-rendered.yaml
grep -R "<prometheus>" /tmp/clickhouse-rendered.yaml
grep -R "CLICKHOUSE_ADMIN_PASSWORD" /tmp/clickhouse-rendered.yaml
grep -R "password_sha256_hex" /tmp/clickhouse-rendered.yaml
```

## Runtime Secret Creation

Do not commit the generated Secret values. This command stores the AES key in the UPMIO-style `aes-secret-key` Secret and creates an AES-CTR encrypted `CLICKHOUSE_ADMIN_PASSWORD` payload in `clickhouse-runtime-secret`.

```bash
kubectl apply -f clickhouse/phase-01/manifests/00-namespace-project.yaml

secret_tmp_dir="$(mktemp -d)"
aes_key="$(openssl rand -hex 16)"
admin_password="$(openssl rand -base64 24)"
iv_hex="$(openssl rand -hex 16)"
key_hex="$(printf "%s" "$aes_key" | od -t x1 -An -v | tr -d ' \n')"

printf "%s" "$iv_hex" | xxd -r -p >"${secret_tmp_dir}/CLICKHOUSE_ADMIN_PASSWORD"
printf "%s" "$admin_password" | openssl enc -aes-256-ctr -K "$key_hex" -iv "$iv_hex" >>"${secret_tmp_dir}/CLICKHOUSE_ADMIN_PASSWORD"
cp "${secret_tmp_dir}/CLICKHOUSE_ADMIN_PASSWORD" "${secret_tmp_dir}/admin"
cp "${secret_tmp_dir}/CLICKHOUSE_ADMIN_PASSWORD" "${secret_tmp_dir}/default"

kubectl create secret generic aes-secret-key \
  -n upm-clickhouse-runtime \
  --from-literal=AES_SECRET_KEY="$aes_key" \
  --dry-run=client -o yaml | kubectl apply -f -

kubectl create secret generic clickhouse-runtime-secret \
  -n upm-clickhouse-runtime \
  --from-file=CLICKHOUSE_ADMIN_PASSWORD="${secret_tmp_dir}/CLICKHOUSE_ADMIN_PASSWORD" \
  --from-file=admin="${secret_tmp_dir}/admin" \
  --from-file=default="${secret_tmp_dir}/default" \
  --dry-run=client -o yaml | kubectl apply -f -

rm -rf "$secret_tmp_dir"
unset aes_key admin_password iv_hex key_hex
```

## Runtime Deployment

```bash
kubectl apply -f clickhouse/phase-01/manifests/02-clickhouse-keeper-unitset.yaml
kubectl wait -n upm-clickhouse-runtime --for=condition=Ready pod \
  -l unit-operator/unitset.name=clickhouse-runtime-keeper --timeout=300s

kubectl apply -f clickhouse/phase-01/manifests/03-clickhouse-unitset.yaml
kubectl wait -n upm-clickhouse-runtime --for=condition=Ready pod \
  -l unit-operator/unitset.name=clickhouse-runtime --timeout=300s
```

## Runtime Acceptance

Run the read-only acceptance script on the Kubernetes master or any host with a
working kubeconfig:

```bash
bash clickhouse/phase-01/scripts/validate-runtime.sh
```

Expected final line:

```text
PASS phase01_runtime_validation
```

Manual commands:

```bash
kubectl get unitsets,units,pods,pvc,svc,podmonitor -n upm-clickhouse-runtime -o wide

for pod in $(kubectl get pod -n upm-clickhouse-runtime -l unit-operator/unitset.name=clickhouse-runtime-keeper -o name); do
  kubectl exec -n upm-clickhouse-runtime "${pod#pod/}" -c clickhouse-keeper -- \
    timeout 3 bash -lc 'exec 3<>/dev/tcp/127.0.0.1/9181; printf ruok >&3; dd bs=1 count=4 <&3 2>/dev/null'
  kubectl exec -n upm-clickhouse-runtime "${pod#pod/}" -c clickhouse-keeper -- \
    timeout 3 bash -lc 'exec 3<>/dev/tcp/127.0.0.1/9181; printf mntr >&3; cat <&3' | grep zk_server_state
done

clickhouse_pod="$(kubectl get pod -n upm-clickhouse-runtime -l unit-operator/unitset.name=clickhouse-runtime -o jsonpath='{.items[0].metadata.name}')"
kubectl exec -n upm-clickhouse-runtime "$clickhouse_pod" -c clickhouse -- service-ctl.sh health
kubectl exec -n upm-clickhouse-runtime "$clickhouse_pod" -c clickhouse -- service-ctl.sh login --query "SELECT cluster, shard_num, replica_num, host_name, port FROM system.clusters WHERE cluster='upm_cluster' ORDER BY shard_num, replica_num"
kubectl exec -n upm-clickhouse-runtime "$clickhouse_pod" -c clickhouse -- curl -sS --max-time 5 http://127.0.0.1:9363/metrics | head
```

If Prometheus Operator CRDs are absent, UnitSet runtime may skip PodMonitor creation. That is a monitoring prerequisite issue, not a ClickHouse package failure.

## Runtime Upgrade Boundary

These manifests and source changes are safe to validate with `kubectl apply --dry-run=server`.

To make an existing cluster reflect this Phase 01 source change, the runtime must use the updated package chart and a ClickHouse image containing the updated `service-ctl.sh`. Do not replace an existing runtime without an explicit upgrade plan, because enabling password-based users changes readiness/login behavior.

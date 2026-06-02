# ClickHouse GrpcCall Validation

## Purpose

Validate whether ClickHouse day-2 operations can be executed through UPMIO `GrpcCall` and `unit-agent`.

## Commands executed

```bash
kubectl apply -f docs/runtime/manifests/clickhouse-set-variable-runtime.yaml
kubectl get grpccall -n upm-clickhouse-runtime clickhouse-set-variable-runtime -o yaml
kubectl describe grpccall clickhouse-set-variable-runtime -n upm-clickhouse-runtime
kubectl logs -n upm-system deploy/unit-operator --tail=120
kubectl logs -n upm-clickhouse-runtime clickhouse-runtime-0 -c unit-agent --tail=160

kubectl apply -f docs/runtime/manifests/clickhouse-logical-backup-runtime.yaml
kubectl get grpccall -n upm-clickhouse-runtime clickhouse-logical-backup-runtime -o yaml
kubectl describe grpccall clickhouse-logical-backup-runtime -n upm-clickhouse-runtime
```

Manifests used:

```text
docs/runtime/manifests/clickhouse-set-variable-runtime.yaml
docs/runtime/manifests/clickhouse-logical-backup-runtime.yaml
```

Restore was not executed because no valid backup file/object storage was available and restore would be destructive for the validation cluster.

## Runtime evidence

`set-variable` result:

```text
Name: clickhouse-set-variable-runtime
Spec:
  Type: clickhouse
  Action: set-variable
  Target Unit: clickhouse-runtime-0
  Parameters:
    username: default
    key: max_threads
    value: 8
Status:
  Result: Failed
  Message: unsupported unit type "clickhouse"
```

`logical-backup` result:

```text
Name: clickhouse-logical-backup-runtime
Spec:
  Type: clickhouse
  Action: logical-backup
  Target Unit: clickhouse-runtime-0
  Parameters:
    username: default
    backupFile: runtime-validation/full-001
    objectStorage: endpoint=s3.example.invalid, bucket=upm-runtime-validation, keys=REDACTED_RUNTIME_PLACEHOLDER
Status:
  Result: Failed
  Message: unsupported unit type "clickhouse"
```

Operator logs:

```text
start reconciling grpc call instance [upm-clickhouse-runtime/clickhouse-set-variable-runtime]
unsupported unit type "clickhouse"
OperationFailed
```

Source evidence in local repository:

```text
unit-operator/api/v1alpha1/grpccall_types.go:48-49 defines ClickHouseType = "clickhouse".
unit-operator/pkg/controller/grpccall/handler.go:199-216 maps ClickHouse logical-backup, restore, and set-variable to ClickHouse gRPC methods.
unit-operator/pkg/agent/app/clickhouse/impl.go:70-108 implements LogicalBackup.
unit-operator/pkg/agent/app/clickhouse/impl.go:111-149 implements Restore.
unit-operator/pkg/agent/app/clickhouse/impl.go:152-180 begins SetVariable implementation.
```

## Key findings

Runtime `GrpcCall` did not reach the ClickHouse unit-agent. The installed `unit-operator` image rejected `spec.type: clickhouse` with `unsupported unit type "clickhouse"` for both `set-variable` and `logical-backup`.

This conflicts with local source evidence showing ClickHouse handler support. Most likely explanations:

- the installed `quay.io/upmio/unit-operator:v1.1.0` image was built before the ClickHouse `GrpcCall` handler was included;
- the public repository branch inspected locally is newer than the published image;
- the Helm chart app version and local source are not enough to prove image behavior.

Credential model from source:

- operations expect `username`;
- password is decrypted from a file under `SECRET_MOUNT` using `AES_SECRET_KEY`;
- object storage fields are required for backup/restore.

Status model:

```text
status.result: Success or Failed
status.message: terminal message
status.startTime
status.completionTime
```

## Open questions

- Which exact unit-operator image tag includes ClickHouse `GrpcCall` support?
- Should the hackathon build a local operator image from source for runtime validation, or avoid GrpcCall for MVP?
- What object storage should be used for real ClickHouse backup validation?

## Conclusion

Status: FAIL for ClickHouse `GrpcCall` runtime execution with installed public operator image.

The API resource accepts `GrpcCall` objects, but the controller rejects ClickHouse type before invoking unit-agent. Day-2 ClickHouse operations are not usable for MVP unless a compatible operator image is provided or the manager bypasses `GrpcCall`.

## Impact on future ClickHouse Spec

The Spec should not promise ClickHouse backup, restore, or set-variable through `GrpcCall` until the runtime image supports it. MVP should either exclude these operations or mark them as dependent on a verified operator image.

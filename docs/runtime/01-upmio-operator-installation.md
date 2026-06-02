# UPMIO Operator Runtime Installation

## Purpose

Validate that public UPMIO `unit-operator` and `compose-operator` can be installed on the prepared Kubernetes cluster, and identify the CRDs, controllers, images, namespaces, and image handling required before ClickHouse runtime validation.

## Commands executed

Commands were executed from the Kubernetes master shell. SSH credentials and kubeconfig contents are intentionally not recorded.

```bash
kubectl get nodes -o wide
kubectl get sc
helm list -A
kubectl get crd | grep -Ei "upm|unit|grpccall|mysql|redis|postgres|proxy"
kubectl api-resources | grep -Ei "unit|unitset|grpccall|mysql|proxysql|redis|postgres"
kubectl get pods -A | grep -Ei "unit|compose|operator"

helm upgrade --install unit-operator ./unit-operator/charts/unit-operator \
  -n upm-system --create-namespace \
  -f docs/runtime/manifests/unit-operator-install-values.yaml

helm upgrade --install compose-operator ./compose-operator/charts/compose-operator \
  -n upm-system --create-namespace \
  -f docs/runtime/manifests/compose-operator-install-values.yaml
```

Images were loaded into node-local `containerd` because registry access from VMs was not reliable:

```bash
ctr -n k8s.io images import unit-operator-v1.1.0.tar
ctr -n k8s.io images import compose-operator-v1.1.1.tar
ctr -n k8s.io images import clickhouse-26.3.9.8.tar
ctr -n k8s.io images import clickhouse-agent-26.3.tar
ctr -n k8s.io images import clickhouse-keeper-26.3.9.8.tar
ctr -n k8s.io images import unit-agent-v1.1.0.tar
```

## Runtime evidence

Cluster baseline:

```text
NAME               STATUS   ROLES           VERSION   INTERNAL-IP      OS-IMAGE                              CONTAINER-RUNTIME
upm-k8s-master01   Ready    control-plane   v1.35.5   192.168.35.201   Red Hat Enterprise Linux 9.6 (Plow)   containerd://2.1.5
upm-k8s-worker01   Ready    <none>          v1.35.5   192.168.35.202   Red Hat Enterprise Linux 9.6 (Plow)   containerd://2.1.5
upm-k8s-worker02   Ready    <none>          v1.35.5   192.168.35.203   Red Hat Enterprise Linux 9.6 (Plow)   containerd://2.1.5
upm-k8s-worker03   Ready    <none>          v1.35.5   192.168.35.204   Red Hat Enterprise Linux 9.6 (Plow)   containerd://2.1.5
```

StorageClass:

```text
local-path (default)   rancher.io/local-path   Delete   WaitForFirstConsumer
```

Helm releases:

```text
unit-operator             upm-system   deployed   unit-operator-1.1.0              v1.1.0
compose-operator          upm-system   deployed   compose-operator-1.1.1           v1.1.1
clickhouse-keeper-package upm-system   deployed   clickhouse-keeper-26.3.9.8-1.1.0 26.3.9.8
clickhouse-package        upm-system   deployed   clickhouse-26.3.9.8-1.1.0        26.3.9.8
```

Controller pods:

```text
upm-system   compose-operator-6449ddd557-zstwz   1/1   Running
upm-system   unit-operator-8459fc4884-q89j2      1/1   Running
```

Installed UPMIO CRDs:

```text
grpccalls.upm.syntropycloud.io
projects.upm.syntropycloud.io
units.upm.syntropycloud.io
unitsets.upm.syntropycloud.io
mysqlreplications.upm.syntropycloud.io
mysqlgroupreplications.upm.syntropycloud.io
proxysqlsyncs.upm.syntropycloud.io
redisreplications.upm.syntropycloud.io
redisclusters.upm.syntropycloud.io
postgresreplications.upm.syntropycloud.io
mongodbreplicasets.upm.syntropycloud.io
```

API resources:

```text
grpccalls      gc   upm.syntropycloud.io/v1alpha1   true   GrpcCall
units          un   upm.syntropycloud.io/v1alpha2   true   Unit
unitsets       us   upm.syntropycloud.io/v1alpha2   true   UnitSet
mysqlreplications, mysqlgroupreplications, proxysqlsyncs, redisreplications, redisclusters, postgresreplications
```

Repository evidence:

```text
unit-operator/go.mod: github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring v0.80.0
unit-operator/pkg/vars/vars.go: ManagerNamespace defaults to "upm-system"
```

## Key findings

UPMIO operator runtime is installable with Helm charts from the repositories.

The operator images used were:

```text
quay.io/upmio/unit-operator:v1.1.0
quay.io/upmio/compose-operator:v1.1.1
```

ClickHouse package installation is separate from operator installation and installs package templates into `upm-system`.

Runtime namespace preparation required a `Project` object in addition to a Kubernetes Namespace. Without the `Project`, Unit pods failed because the service account `upm-clickhouse-runtime-serviceaccount` was missing.

The `Project` controller created:

```text
upm-clickhouse-runtime-serviceaccount
Role
RoleBinding
aes-secret-key
```

## Open questions

- Should the future manager create a `Project` object automatically before creating UnitSets?
- Should image import/private registry preparation be part of hackathon automation or remain an environment prerequisite?
- Which published operator image version includes ClickHouse `GrpcCall` support? Runtime `v1.1.0` rejected ClickHouse even though local source contains ClickHouse handler code.

## Conclusion

Status: PASS for operator installation.

`unit-operator` and `compose-operator` installed and ran successfully in `upm-system`. Required CRDs and API resources were available. Image pull reliability remains an environment risk; local image import worked.

## Impact on future ClickHouse Spec

The Spec must include an explicit UPMIO runtime prerequisite section:

- `unit-operator` and `compose-operator` Helm releases.
- ClickHouse and ClickHouse Keeper package Helm releases.
- `Project` object creation for each managed namespace.
- image pull or local image import/private registry strategy.
- no host-level application deployment.

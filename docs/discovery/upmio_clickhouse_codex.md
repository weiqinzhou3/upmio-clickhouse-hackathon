# Codex Task: UPMIO ClickHouse Hackathon Phase 00/01 Discovery and Environment Preparation

## 0. Context

We are preparing for an internal AI Coding Hackathon.

The target is to design and later implement a ClickHouse operations management software based on UPMIO Operator capabilities.

Current stage is **NOT feature coding**.

The current stage is for:

1. Kubernetes environment preparation.
2. UPMIO repository inspection.
3. Existing UPMIO MySQL Day1 / Day2 implementation analysis.
4. Monitoring capability verification.
5. Producing markdown evidence documents that will be used to write the first-stage Spec.

Do not start implementing the ClickHouse manager yet.

---

## 1. Known Environment

### 1.1 Kubernetes Nodes

The virtual machines are already prepared.

| Role | IP | OS |
|---|---|---|
| master | 192.168.35.201 | RHEL 9.6 |
| worker | 192.168.35.202 | RHEL 9.6 |
| worker | 192.168.35.203 | RHEL 9.6 |
| worker | 192.168.35.204 | RHEL 9.6 |

### 1.2 Hard Constraints

- These VMs must be used as Kubernetes nodes only.
- Do not install application services directly on the host except what is required for Kubernetes.
- Application components must run inside Kubernetes.
- Avoid installing unnecessary host-level packages.
- Do not store credentials, kubeconfig, tokens, or private keys in git.
- Development environment is local Mac.
- Local project directory name: `upm-clickhouse`.

---

## 2. Overall Goals

### Goal 1: Prepare Kubernetes Cluster

Prepare a Kubernetes cluster suitable for UPMIO and ClickHouse HA demo.

Target architecture:

- 1 control-plane node: `192.168.35.201`
- 3 worker nodes: `192.168.35.202`, `192.168.35.203`, `192.168.35.204`
- Container runtime: `containerd`
- Kubernetes version: choose a stable version compatible with RHEL 9.6 and current UPMIO projects
- CNI: Calico or another stable CNI
- StorageClass: choose a simple local storage solution suitable for demo, such as `local-path-provisioner` or `OpenEBS LocalPV`
- Helm 3 installed
- kubectl configured on master

Do not deploy ClickHouse yet unless explicitly required for validation.

### Goal 2: Clone and Inspect UPMIO Repositories

On the local Mac project directory `upm-clickhouse`, clone:

- `https://github.com/upmio/unit-operator`
- `https://github.com/upmio/compose-operator`
- `https://github.com/upmio/upm-packages`

Inspect the repositories and produce markdown summaries.

### Goal 3: Investigate Existing MySQL Day1 / Day2 Implementation

Find how UPMIO implements MySQL operations today.

Focus on:

1. MySQL HA cluster deployment.
2. MySQL backup.
3. MySQL user creation.
4. MySQL operational tasks.
5. How these capabilities map to `unit-operator`, `compose-operator`, and `upm-packages`.

### Goal 4: Verify Monitoring Capability

Verify whether UPMIO provides:

- Prometheus Server installation
- Grafana installation
- ServiceMonitor / PodMonitor
- metrics endpoint configuration
- Grafana dashboards
- alert rules
- database-specific metrics templates

Do not assume. Verify from repository files, Helm charts, manifests, and actual Kubernetes deployment if available.

### Goal 5: Produce Markdown Evidence Documents

Create reproducible markdown documents under:

```text
upm-clickhouse/
└── docs/
    └── discovery/
        ├── 00-environment-plan.md
        ├── 01-kubernetes-installation-record.md
        ├── 02-upmio-repository-analysis.md
        ├── 03-upmio-crd-model-analysis.md
        ├── 04-mysql-day1-day2-implementation-analysis.md
        ├── 05-monitoring-capability-analysis.md
        └── 06-clickhouse-mapping-hypothesis.md
```

Each document must include:

- Purpose
- Commands executed
- Key findings
- Evidence
  - source file path
  - relevant snippets
  - kubectl output
  - helm output
- Open questions
- Conclusion
- Impact on future ClickHouse Spec

---

## 3. Phase 01-A: Kubernetes Environment Plan

Before executing installation, create:

```text
docs/discovery/00-environment-plan.md
```

This document must include:

### 3.1 Proposed Kubernetes Installation Plan

Include:

- Kubernetes version
- container runtime
- CNI choice
- StorageClass choice
- Helm version
- required host-level packages
- required firewall/SELinux/swap/kernel settings
- reason for each choice

### 3.2 RHEL 9.6 Specific Notes

Check and document:

- SELinux mode
- firewalld status
- swap status
- kernel modules required by Kubernetes
- sysctl settings
- containerd configuration
- image registry access
- DNS resolution
- time synchronization

### 3.3 Installation Commands

Provide commands for:

- all nodes
- master node only
- worker nodes only

Do not execute destructive operations without listing them clearly.

### 3.4 Readiness Criteria

The cluster is considered ready only if the following commands succeed:

```bash
kubectl get nodes -o wide
kubectl get pods -A
kubectl get sc
kubectl cluster-info
kubectl version
helm version
```

Expected readiness:

- all nodes are `Ready`
- CoreDNS is running
- CNI is running
- StorageClass exists
- metrics-server is either installed or explicitly marked as pending
- no application workload is installed directly on hosts

---

## 4. Phase 01-B: Kubernetes Installation Record

After installation, create:

```text
docs/discovery/01-kubernetes-installation-record.md
```

This document must include:

### 4.1 Actual Commands Executed

Record all actual commands executed, grouped by:

- all nodes
- master node
- worker nodes
- local Mac

### 4.2 Actual Outputs

Include outputs of:

```bash
kubectl get nodes -o wide
kubectl get pods -A
kubectl get sc
kubectl cluster-info
kubectl version
helm version
```

### 4.3 Issues Encountered

For each issue:

- symptom
- root cause if known
- command used for diagnosis
- fix applied
- final status

### 4.4 Final Cluster Readiness Summary

State clearly:

- ready / not ready
- blocking issues
- non-blocking issues
- recommended next action

---

## 5. Phase 02: UPMIO Repository Analysis

Create:

```text
docs/discovery/02-upmio-repository-analysis.md
```

Clone repositories into local project directory:

```bash
mkdir -p upm-clickhouse
cd upm-clickhouse

git clone https://github.com/upmio/unit-operator.git
git clone https://github.com/upmio/compose-operator.git
git clone https://github.com/upmio/upm-packages.git
```

Run and record:

```bash
tree -L 3 unit-operator
tree -L 3 compose-operator
tree -L 4 upm-packages
find unit-operator -maxdepth 5 -type f | sort
find compose-operator -maxdepth 5 -type f | sort
find upm-packages -maxdepth 5 -type f | sort
```

Answer:

1. What is the purpose of `unit-operator`?
2. What is the purpose of `compose-operator`?
3. What is the purpose of `upm-packages`?
4. What programming language and framework are used?
5. Where are CRD definitions stored?
6. Where are Helm charts stored?
7. Where are examples stored?
8. Is ClickHouse already present anywhere?
9. Is MySQL present anywhere?
10. Is monitoring present anywhere?

For every conclusion, include evidence from file path and snippet.

---

## 6. Phase 03: UPMIO CRD Model Analysis

Create:

```text
docs/discovery/03-upmio-crd-model-analysis.md
```

### 6.1 Static Repository Inspection

Find all CRD/API definitions.

Run and record:

```bash
find unit-operator -type f | grep -E "api|crd|types|yaml|yml" | sort
find compose-operator -type f | grep -E "api|crd|types|yaml|yml" | sort
grep -R "kind: CustomResourceDefinition" -n unit-operator compose-operator upm-packages || true
grep -R "type .*Spec struct" -n unit-operator/api compose-operator/api || true
grep -R "type .*Status struct" -n unit-operator/api compose-operator/api || true
```

### 6.2 Kubernetes Cluster Inspection

If UPMIO components are installed, run:

```bash
kubectl get crd | grep -Ei "upm|unit|compose|mysql|redis|postgres|proxy|servicegroup" || true
kubectl api-resources | grep -Ei "upm|unit|compose|mysql|redis|postgres|proxy|servicegroup" || true
kubectl get pods -A | grep -Ei "upm|operator|unit|compose" || true
```

### 6.3 Required Questions

Answer these questions with evidence:

1. What is `Unit`?
2. What is `UnitSet`?
3. Is `ServiceGroup` present?
4. If `ServiceGroup` is present:
   - where is it defined?
   - what is its spec?
   - what is its status?
   - what controller reconciles it?
5. If `ServiceGroup` is not present:
   - is it replaced by another CRD?
   - is the cluster-level concept handled by `compose-operator`?
   - is it only available in enterprise/private version?
   - is it only a Helm/package-level concept?
6. What CRDs are provided by `compose-operator`?
7. How does `compose-operator` represent replication topology?
8. What CRDs are relevant to MySQL?
9. What CRDs are potentially reusable for ClickHouse?
10. What is the likely mapping for ClickHouse:
    - ClickHouse Server
    - ClickHouse Keeper
    - ClickHouse Cluster
    - shard
    - replica
    - service endpoint
    - operational task

### 6.4 Output Table

Include a table:

| Concept | Current UPMIO Object | Evidence | Can Map to ClickHouse? | Notes |
|---|---|---|---|---|
| single instance | TBD | file path / kubectl output | yes/no | TBD |
| instance group | TBD | file path / kubectl output | yes/no | TBD |
| database cluster | TBD | file path / kubectl output | yes/no | TBD |
| replication topology | TBD | file path / kubectl output | yes/no | TBD |
| operational task | TBD | file path / kubectl output | yes/no | TBD |

---

## 7. Phase 04: Existing MySQL Day1 / Day2 Implementation Analysis

Create:

```text
docs/discovery/04-mysql-day1-day2-implementation-analysis.md
```

### 7.1 Search Commands

Run and record:

```bash
grep -R "mysql" -n unit-operator compose-operator upm-packages | head -300
find unit-operator compose-operator upm-packages -iname "*mysql*" -print
grep -R "MysqlReplication\|MySQL\|ProxySQL\|backup\|restore\|user\|grant" -n unit-operator compose-operator upm-packages | head -500
```

### 7.2 Case A: Deploy MySQL HA Cluster

Answer:

1. What HA model is used?
   - InnoDB Cluster?
   - primary-replica replication?
   - semi-sync replication?
   - group replication?
   - MySQL + ProxySQL?
   - other?
2. Which UPMIO components are involved?
   - `unit-operator`
   - `compose-operator`
   - `upm-packages`
3. Which CRDs are used?
4. Which Helm charts are used?
5. Which object represents a single MySQL instance?
6. Which object represents a group of MySQL instances?
7. Which object represents the MySQL cluster/topology?
8. Which object represents ProxySQL if present?
9. Who creates:
   - Pod
   - PVC
   - Service
   - Secret
   - ConfigMap
10. Who configures replication?
11. Who handles failover?
12. Where is status stored?
13. How would a human operator deploy this MySQL HA cluster manually using UPMIO?

### 7.3 Case B: MySQL Backup

Answer:

1. Is backup implemented?
2. Is backup modeled as:
   - CRD
   - Kubernetes Job
   - CronJob
   - Helm template
   - script
   - operator action
   - gRPC task
3. Which backup tool is used?
4. Where is backup configuration stored?
5. Where is backup status stored?
6. Where are backup files stored?
7. Is restore supported?
8. How would a human operator trigger backup manually using UPMIO?

### 7.4 Case C: MySQL User Creation

Answer:

1. Is user creation implemented?
2. Is it declarative or task-based?
3. Is it handled through:
   - CRD
   - SQL job
   - agent
   - operator
   - gRPC call
   - script
4. How are passwords represented?
5. Are Kubernetes Secrets used?
6. Is the operation idempotent?
7. Is there audit/status tracking?
8. How would a human operator create a MySQL user manually using UPMIO?

### 7.5 Required Summary

Produce:

| MySQL Operation | UPMIO Component | Implementation Mechanism | Human Operation Path | Relevance to ClickHouse |
|---|---|---|---|---|
| HA deployment | TBD | TBD | TBD | TBD |
| backup | TBD | TBD | TBD | TBD |
| user creation | TBD | TBD | TBD | TBD |
| failover | TBD | TBD | TBD | TBD |
| config change | TBD | TBD | TBD | TBD |
| restart | TBD | TBD | TBD | TBD |

---

## 8. Phase 05: Monitoring Capability Analysis

Create:

```text
docs/discovery/05-monitoring-capability-analysis.md
```

### 8.1 Search Commands

Run and record:

```bash
grep -R "prometheus\|Prometheus\|ServiceMonitor\|PodMonitor\|Grafana\|dashboard\|metrics\|alert" -n unit-operator compose-operator upm-packages | head -500
find unit-operator compose-operator upm-packages -type f | grep -Ei "monitor|prometheus|grafana|dashboard|alert|servicemonitor|podmonitor|metrics"
```

### 8.2 Required Questions

Answer with evidence:

1. Does UPMIO install Prometheus Server?
2. Does UPMIO install Grafana?
3. Does UPMIO provide ServiceMonitor?
4. Does UPMIO provide PodMonitor?
5. Does UPMIO configure database metrics endpoints?
6. Does UPMIO provide Grafana dashboards?
7. Does UPMIO provide alert rules?
8. Does UPMIO provide MySQL metrics integration?
9. Does UPMIO provide ClickHouse metrics integration?
10. If Prometheus Server is not included, what is the recommended external dependency?

### 8.3 Required Summary

Produce:

| Monitoring Capability | Provided by UPMIO? | Repository Evidence | Runtime Evidence | Notes |
|---|---|---|---|---|
| Prometheus Server | yes/no/unknown | TBD | TBD | TBD |
| Grafana | yes/no/unknown | TBD | TBD | TBD |
| ServiceMonitor | yes/no/unknown | TBD | TBD | TBD |
| PodMonitor | yes/no/unknown | TBD | TBD | TBD |
| dashboards | yes/no/unknown | TBD | TBD | TBD |
| alert rules | yes/no/unknown | TBD | TBD | TBD |
| MySQL metrics | yes/no/unknown | TBD | TBD | TBD |
| ClickHouse metrics | yes/no/unknown | TBD | TBD | TBD |

---

## 9. Phase 06: ClickHouse Mapping Hypothesis

Create:

```text
docs/discovery/06-clickhouse-mapping-hypothesis.md
```

This is not final design. It is a hypothesis based on evidence.

### 9.1 Required ClickHouse HA Target

Assume future Day1 target should be production-like HA, not single-node.

Candidate target:

```text
ClickHouse HA Cluster
├── 2 shards × 2 replicas = 4 ClickHouse Server instances
└── 3 ClickHouse Keeper instances
```

### 9.2 Required Questions

Answer:

1. How should ClickHouse Server map to UPMIO objects?
2. How should ClickHouse Keeper map to UPMIO objects?
3. How should shard/replica topology be represented?
4. Is there an existing UPMIO cluster-level object suitable for ClickHouse?
5. If MySQL uses `MysqlReplication`, is there an equivalent extension point for ClickHouse?
6. Can ClickHouse be implemented first as a package/chart plus upper-level manager without modifying `unit-operator` and `compose-operator`?
7. Which parts likely require only `upm-packages`?
8. Which parts likely require a new upper-level Go API server?
9. Which parts might require future changes to `compose-operator`?
10. Which parts should be excluded from MVP?

### 9.3 Required Output Table

| ClickHouse Concept | Proposed UPMIO Mapping | Confidence | Evidence | Open Question |
|---|---|---|---|---|
| ClickHouse Server | TBD | high/medium/low | TBD | TBD |
| ClickHouse Keeper | TBD | high/medium/low | TBD | TBD |
| shard | TBD | high/medium/low | TBD | TBD |
| replica | TBD | high/medium/low | TBD | TBD |
| cluster | TBD | high/medium/low | TBD | TBD |
| Distributed table | TBD | high/medium/low | TBD | TBD |
| ReplicatedMergeTree | TBD | high/medium/low | TBD | TBD |
| metrics | TBD | high/medium/low | TBD | TBD |
| backup | TBD | high/medium/low | TBD | TBD |
| user management | TBD | high/medium/low | TBD | TBD |

---

## 10. Rules

### 10.1 Do Not Implement Product Code

Do not implement:

- ClickHouse manager backend
- frontend UI
- new ClickHouse CRD
- new controller
- production ClickHouse deployment
- business APIs

Current work is discovery and environment preparation only.

### 10.2 Do Not Modify UPMIO Source Code

Do not modify:

- `unit-operator`
- `compose-operator`
- `upm-packages`

Repository inspection is read-only.

### 10.3 Evidence Required

Every non-trivial conclusion must have evidence:

- file path
- code snippet
- README snippet
- Helm chart path
- CRD YAML path
- kubectl output
- helm output

If evidence is missing, mark the conclusion as:

```text
Assumption / Not yet verified
```

### 10.4 No Secrets

Do not commit:

- kubeconfig
- private keys
- tokens
- passwords
- cluster credentials
- registry credentials

### 10.5 Markdown Quality

Markdown files must be clean and structured.

Each file must have:

```markdown
# Title

## Purpose

## Commands Executed

## Key Findings

## Evidence

## Open Questions

## Conclusion

## Impact on Future ClickHouse Spec
```

---

## 11. Final Response Required from Codex

At the end of this task, provide a summary in the terminal/chat:

1. Kubernetes cluster readiness summary.
2. UPMIO component model summary.
3. Existing MySQL Day1 / Day2 implementation summary.
4. Monitoring capability summary.
5. ClickHouse mapping hypothesis.
6. Recommended next experiments before writing the Master Spec.
7. Unresolved questions requiring human decision.
8. File list of all generated markdown documents.

Do not say "done" without listing the generated files and the evidence summary.

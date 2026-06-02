# upm-packages ClickHouse Design

- Version: 0.3
- Date: 2026-05-27
- Status: Sealed
- Owner: zqw
- Related:
  - ../master-spec.md
  - resource-model.md

## 1. Purpose

This document defines the required `upm-packages/clickhouse` and `upm-packages/clickhouse-keeper` adaptations.

The existing public package is treated as an incomplete reference asset, not a finished product capability.

## 2. Required Package Fixes

| Area | MVP Requirement |
|---|---|
| Password initialization | use Secret-based password; never use `<no_password/>` |
| Invalid settings | remove or fix settings rejected by the target ClickHouse version |
| Secret/env injection | support `AES_SECRET_KEY`, `SECRET_MOUNT`, and runtime secret mount |
| Log mount | provide writable log mount for ClickHouse and Keeper |
| Metrics | enable ClickHouse native Prometheus endpoint |
| Keeper config | generate 3-node Keeper configuration |
| Server config | generate Keeper connection, remote servers, macros, ports, users, profiles |
| Values | expose topology, resources, storage, metrics, security, and service parameters |

## 3. Template and Values Boundary

`values.yaml` provides parameters.

Template files such as `clickhouseTemplate.tpl` render final ClickHouse configuration from those parameters.

Rendered content is stored in ConfigMaps and mounted into Pods.

```text
values.yaml
   + template files
        -> rendered ConfigMap
             -> mounted into ClickHouse Pod
                  -> ClickHouse reads config at startup/reload
```

## 4. Topology Parameters

MVP must support:

```yaml
topology:
  shards: 1
  replicasPerShard: 2
keeper:
  replicas: 3
```

Future must support generic:

```yaml
topology:
  shards: N
  replicasPerShard: M
```

Examples should be values files only, not separate hard-coded implementation templates.

## 5. Metrics Design

Use B7: ClickHouse native Prometheus endpoint.

Do not add exporter sidecar in MVP.

Required config should enable:

- metrics port;
- metrics path;
- PodMonitor scrape configuration;
- validation that endpoint is listening.

Exporter sidecar is future only if native metrics are insufficient.

## 6. Version Strategy

- Keep existing `26.3.9.8` as reference.
- Add or validate a newer 26.3 LTS patch version as the recommended MVP default.
- Optionally add a latest stable validation version.
- Server and Keeper package versions should remain aligned unless explicitly tested.

## 7. Acceptance Criteria

- ClickHouse Keeper 3-node UnitSet can deploy repeatably.
- ClickHouse Server 1 shard x 2 replicas can deploy repeatably.
- No manual ConfigMap patch is required.
- Default/admin user uses password from Secret.
- Metrics endpoint listens and can be scraped.
- Package supports topology values without hard-coded examples.

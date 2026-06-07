#!/usr/bin/env python3
import argparse
import re
import sys
import xml.etree.ElementTree as ET
from pathlib import Path


def fail(message):
    print(f"FAIL topology_render_validation: {message}", file=sys.stderr)
    sys.exit(1)


def extract_configmap_block(rendered, suffix):
    docs = re.split(r"(?m)^---\s*$", rendered)
    for doc in docs:
        if "kind: ConfigMap" not in doc:
            continue
        name_match = re.search(r"(?m)^  name: ([^\n]+)$", doc)
        if not name_match:
            continue
        name = name_match.group(1).strip().strip('"')
        if not name.endswith(suffix):
            continue
        lines = doc.splitlines()
        for idx, line in enumerate(lines):
            if line == "  clickhouse: |":
                block = []
                for body_line in lines[idx + 1:]:
                    if body_line.startswith("    "):
                        block.append(body_line[4:])
                    elif body_line.strip() == "":
                        block.append("")
                    else:
                        break
                return "\n".join(block)
    fail(f"missing ConfigMap data block ending with {suffix}")


def yaml_scalar(block, section, key):
    pattern = rf"(?ms)^{re.escape(section)}:\n(?:  [^\n]+\n)*?  {re.escape(key)}: \"?([^\"\n]+)\"?"
    match = re.search(pattern, block)
    if not match:
        fail(f"missing {section}.{key} in config value")
    return match.group(1).strip()


def parse_int(value, name):
    try:
        parsed = int(value)
    except ValueError:
        fail(f"{name} is not an integer: {value}")
    if parsed < 1:
        fail(f"{name} must be >= 1: {value}")
    return parsed


def main():
    parser = argparse.ArgumentParser(description="Validate rendered ClickHouse topology ConfigMaps.")
    parser.add_argument("--rendered", required=True, help="Path to helm-rendered YAML.")
    parser.add_argument("--shards", required=True, type=int)
    parser.add_argument("--replicas-per-shard", required=True, type=int)
    parser.add_argument("--cluster-name", default="upm_cluster")
    args = parser.parse_args()

    rendered_path = Path(args.rendered)
    rendered = rendered_path.read_text(encoding="utf-8")
    config_value = extract_configmap_block(rendered, "-config-value")
    config_template = extract_configmap_block(rendered, "-config-template")

    value_cluster = yaml_scalar(config_value, "cluster", "name")
    value_shards = parse_int(yaml_scalar(config_value, "topology", "shards"), "topology.shards")
    value_replicas = parse_int(
        yaml_scalar(config_value, "topology", "replicasPerShard"),
        "topology.replicasPerShard",
    )
    value_keeper = parse_int(yaml_scalar(config_value, "keeper", "replicas"), "keeper.replicas")

    if value_cluster != args.cluster_name:
        fail(f"cluster.name mismatch: expected {args.cluster_name}, got {value_cluster}")
    if value_shards != args.shards:
        fail(f"topology.shards mismatch: expected {args.shards}, got {value_shards}")
    if value_replicas != args.replicas_per_shard:
        fail(
            "topology.replicasPerShard mismatch: "
            f"expected {args.replicas_per_shard}, got {value_replicas}"
        )

    if "<shard>01</shard>" in config_template:
        fail("template still contains hardcoded <shard>01</shard>")

    try:
        root = ET.fromstring(config_template)
    except ET.ParseError as exc:
        fail(f"clickhouse config template is not XML parseable: {exc}")

    remote_servers = root.find("remote_servers")
    if remote_servers is None:
        fail("missing remote_servers")
    cluster = remote_servers.find(args.cluster_name)
    if cluster is None:
        fail(f"missing remote_servers cluster {args.cluster_name}")
    cluster_secret = cluster.findtext("secret", "")
    if (
        "{{ $clusterSecret }}" not in cluster_secret
        or "sha256sum" not in config_template
        or "AES_SECRET_KEY" not in config_template
    ):
        fail(f"remote_servers cluster secret is not derived from runtime AES key: {cluster_secret}")
    if cluster.findtext("allow_distributed_ddl_queries") != "true":
        fail("remote_servers cluster does not allow distributed DDL queries")

    shard_nodes = cluster.findall("shard")
    if len(shard_nodes) != args.shards:
        fail(f"expected {args.shards} shard blocks, got {len(shard_nodes)}")

    for shard_index, shard in enumerate(shard_nodes):
        replicas = shard.findall("replica")
        if len(replicas) != args.replicas_per_shard:
            fail(
                f"shard {shard_index + 1} expected {args.replicas_per_shard} replicas, "
                f"got {len(replicas)}"
            )
        for replica_index, replica in enumerate(replicas):
            host = replica.findtext("host", "")
            port = replica.findtext("port", "")
            unit_index = shard_index * args.replicas_per_shard + replica_index
            if f"-{unit_index}." not in host:
                fail(f"replica host does not contain expected unit index {unit_index}: {host}")
            if "{{ $serviceName }}" not in host or "{{ $namespace }}" not in host:
                fail(f"replica host is not runtime service/namespace templated: {host}")
            if "{{ $tcpPort }}" not in port:
                fail(f"replica port is not runtime tcp port templated: {port}")

    macros = root.find("macros")
    if macros is None:
        fail("missing macros")
    if macros.findtext("cluster") != args.cluster_name:
        fail("macros.cluster does not match cluster.name")
    shard_macro = macros.findtext("shard", "")
    replica_macro = macros.findtext("replica", "")
    if "shard%02d" not in shard_macro or "$runtimeShardIndex" not in shard_macro:
        fail(f"shard macro is not runtime-derived: {shard_macro}")
    if "replica%02d" not in replica_macro or "$runtimeReplicaIndex" not in replica_macro:
        fail(f"replica macro is not runtime-derived: {replica_macro}")

    keeper_nodes = root.find("zookeeper").findall("node") if root.find("zookeeper") is not None else []
    if len(keeper_nodes) != value_keeper:
        fail(f"keeper node count mismatch: expected {value_keeper}, got {len(keeper_nodes)}")

    distributed_ddl = root.find("distributed_ddl")
    if distributed_ddl is None:
        fail("missing distributed_ddl")
    ddl_path = distributed_ddl.findtext("path", "")
    if "/clickhouse/task_queue/ddl/" not in ddl_path or "{{ $serviceName }}" not in ddl_path:
        fail(f"distributed_ddl path is not service-scoped: {ddl_path}")

    total_units = args.shards * args.replicas_per_shard
    print(
        "topology:",
        f"cluster={args.cluster_name}",
        f"shards={args.shards}",
        f"replicasPerShard={args.replicas_per_shard}",
        f"serverUnits={total_units}",
        f"keeperReplicas={value_keeper}",
    )
    print("remote_servers: shard and replica counts match expected topology")
    print("macros: shard and replica are runtime-derived from UNIT_SN")
    print("distributed_ddl: service-scoped queue path is rendered")
    print("PASS topology_render_validation")


if __name__ == "__main__":
    main()

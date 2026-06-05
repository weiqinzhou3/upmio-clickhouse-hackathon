#!/usr/bin/env bash
set -euo pipefail

IMAGE="${IMAGE:-localhost/upmio/clickhouse:26.3.9.8-runtime}"
BASE_IMAGE="${BASE_IMAGE:-$IMAGE}"
SERVICE_CTL="${SERVICE_CTL:-upm-packages/clickhouse/26.3.9.8/image/service-ctl.sh}"
SSH_USER="${SSH_USER:-root}"
NODES="${NODES:-192.168.35.201 192.168.35.202 192.168.35.203 192.168.35.204}"

if [[ -z "${SSH_PASSWORD:-}" ]]; then
  echo "ERROR: set SSH_PASSWORD for the Kubernetes nodes" >&2
  exit 1
fi

if [[ ! -f "$SERVICE_CTL" ]]; then
  echo "ERROR: service control script not found: $SERVICE_CTL" >&2
  exit 1
fi

for node in $NODES; do
  echo "== ${node}: build and import ${IMAGE} =="
  tmp="$(
    sshpass -p "$SSH_PASSWORD" ssh \
      -o StrictHostKeyChecking=no \
      -o UserKnownHostsFile=/dev/null \
      -o ConnectTimeout=10 \
      "${SSH_USER}@${node}" 'mktemp -d /tmp/ch-runtime-build.XXXXXX' \
      2>/dev/null
  )"

  sshpass -p "$SSH_PASSWORD" scp \
    -o StrictHostKeyChecking=no \
    -o UserKnownHostsFile=/dev/null \
    "$SERVICE_CTL" "${SSH_USER}@${node}:${tmp}/service-ctl.sh" >/dev/null 2>&1

  sshpass -p "$SSH_PASSWORD" ssh \
    -o StrictHostKeyChecking=no \
    -o UserKnownHostsFile=/dev/null \
    -o ConnectTimeout=10 \
    "${SSH_USER}@${node}" \
    "IMAGE='$IMAGE' BASE_IMAGE='$BASE_IMAGE' tmp='$tmp' bash -s" <<'REMOTE'
set -euo pipefail

command -v ctr >/dev/null
command -v podman >/dev/null

cat > "$tmp/Containerfile" <<EOF
FROM ${BASE_IMAGE}
USER root
COPY service-ctl.sh /usr/local/bin/service-ctl.sh
RUN chmod 0755 /usr/local/bin/service-ctl.sh && chown 1001:1001 /usr/local/bin/service-ctl.sh
USER 1001
EOF

if ctr -n k8s.io images ls "name==${BASE_IMAGE}" | tail -n +2 | grep -q .; then
  ctr -n k8s.io images export "$tmp/base.tar" "$BASE_IMAGE"
  podman rmi -f "$BASE_IMAGE" >/dev/null 2>&1 || true
  podman load -i "$tmp/base.tar" >/dev/null
elif ! podman image exists "$BASE_IMAGE"; then
  podman pull "$BASE_IMAGE"
fi

podman build --pull-never -t "$IMAGE" "$tmp" >/dev/null
podman save -o "$tmp/new.tar" "$IMAGE"

ctr -n k8s.io images rm "$IMAGE" >/dev/null 2>&1 || true
ctr -n k8s.io images import "$tmp/new.tar" >/dev/null
ctr -n k8s.io images ls "name==${IMAGE}" | tail -n +2
rm -rf "$tmp"
REMOTE
done

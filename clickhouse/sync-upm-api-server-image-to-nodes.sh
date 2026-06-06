#!/usr/bin/env bash
set -euo pipefail

IMAGE="${IMAGE:-localhost/upmio/upm-api-server:phase-05}"
SSH_USER="${SSH_USER:-root}"
NODES="${NODES:-192.168.35.201 192.168.35.202 192.168.35.203 192.168.35.204}"
GOPROXY="${GOPROXY:-https://goproxy.cn,direct}"
BINARY="${BINARY:-api-server/bin/upm-api-server}"

if [[ -z "${SSH_PASSWORD:-}" ]]; then
  echo "ERROR: set SSH_PASSWORD for the Kubernetes nodes" >&2
  exit 1
fi

command -v go >/dev/null
command -v sshpass >/dev/null

mkdir -p "$(dirname "$BINARY")"
echo "== build linux/amd64 upm-api-server =="
(
  cd api-server
  CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GOPROXY="$GOPROXY" \
    go build -trimpath -ldflags="-s -w" -o ../"$BINARY" ./cmd/upm-api-server
)

for node in $NODES; do
  echo "== ${node}: import ${IMAGE} =="
  tmp="$(
    sshpass -p "$SSH_PASSWORD" ssh \
      -o StrictHostKeyChecking=no \
      -o UserKnownHostsFile=/dev/null \
      -o ConnectTimeout=10 \
      "${SSH_USER}@${node}" 'mktemp -d /tmp/upm-api-server-image.XXXXXX' \
      2>/dev/null
  )"

  sshpass -p "$SSH_PASSWORD" scp \
    -o StrictHostKeyChecking=no \
    -o UserKnownHostsFile=/dev/null \
    "$BINARY" "${SSH_USER}@${node}:${tmp}/upm-api-server" >/dev/null 2>&1

  sshpass -p "$SSH_PASSWORD" ssh \
    -o StrictHostKeyChecking=no \
    -o UserKnownHostsFile=/dev/null \
    -o ConnectTimeout=10 \
    "${SSH_USER}@${node}" \
    "IMAGE='$IMAGE' tmp='$tmp' bash -s" <<'REMOTE'
set -euo pipefail

command -v ctr >/dev/null
command -v podman >/dev/null

chmod 0555 "$tmp/upm-api-server"
cat > "$tmp/Containerfile" <<'EOF'
FROM scratch
COPY upm-api-server /upm-api-server
USER 65532:65532
EXPOSE 8080
ENTRYPOINT ["/upm-api-server"]
EOF

podman build --pull-never -t "$IMAGE" "$tmp" >/dev/null
podman save -o "$tmp/upm-api-server.tar" "$IMAGE"

ctr -n k8s.io images rm "$IMAGE" >/dev/null 2>&1 || true
ctr -n k8s.io images import "$tmp/upm-api-server.tar" >/dev/null
ctr -n k8s.io images ls "name==${IMAGE}" | tail -n +2
rm -rf "$tmp"
REMOTE
done

echo "PASS upm_api_server_image_sync"

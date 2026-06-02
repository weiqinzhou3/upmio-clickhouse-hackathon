# Phase 00 Environment Plan

## Purpose

Define a reproducible Kubernetes environment plan for the UPMIO ClickHouse hackathon discovery phase. This is an infrastructure preparation document only; it does not deploy ClickHouse or UPMIO application workloads.

## Commands Executed

Reference checks used to confirm target host state:

```bash
ssh root@192.168.35.201 'cat /etc/redhat-release; getenforce; systemctl is-active firewalld; swapon --show; sysctl net.ipv4.ip_forward'
ssh root@192.168.35.202 'cat /etc/redhat-release; getenforce; systemctl is-active firewalld; swapon --show; sysctl net.ipv4.ip_forward'
ssh root@192.168.35.203 'cat /etc/redhat-release; getenforce; systemctl is-active firewalld; swapon --show; sysctl net.ipv4.ip_forward'
ssh root@192.168.35.204 'cat /etc/redhat-release; getenforce; systemctl is-active firewalld; swapon --show; sysctl net.ipv4.ip_forward'
```

The SSH password was supplied interactively/by environment variable during execution and is intentionally not recorded.

Planned installation commands were grouped as follows.

All nodes:

```bash
cat >/etc/hosts <<'EOF'
127.0.0.1 localhost localhost.localdomain
192.168.35.201 upm-k8s-master01
192.168.35.202 upm-k8s-worker01
192.168.35.203 upm-k8s-worker02
192.168.35.204 upm-k8s-worker03
EOF

swapoff -a
cp -a /etc/fstab /etc/fstab.bak.$(date +%Y%m%d%H%M%S)
sed -ri '/[[:space:]]swap[[:space:]]/ s/^/# /' /etc/fstab

cat >/etc/modules-load.d/k8s.conf <<'EOF'
overlay
br_netfilter
EOF
modprobe overlay
modprobe br_netfilter

cat >/etc/sysctl.d/99-kubernetes-cri.conf <<'EOF'
net.bridge.bridge-nf-call-iptables = 1
net.bridge.bridge-nf-call-ip6tables = 1
net.ipv4.ip_forward = 1
EOF
sysctl --system

cat >/etc/sysctl.d/98-calico-rpfilter.conf <<'EOF'
net.ipv4.conf.all.rp_filter = 0
net.ipv4.conf.default.rp_filter = 0
net.ipv4.conf.ens192.rp_filter = 0
net.ipv4.conf.tunl0.rp_filter = 0
EOF
sysctl --system

dnf install -y conntrack-tools socat yum-utils iproute-tc iptables-nft ethtool curl tar bash-completion
```

Offline artifact installation on all nodes:

```bash
tar -C /usr/local -xzf /tmp/upm-k8s-artifacts/containerd-2.1.5-linux-amd64.tar.gz
install -m 755 /tmp/upm-k8s-artifacts/runc.amd64 /usr/local/sbin/runc
mkdir -p /opt/cni/bin /etc/containerd
tar -C /opt/cni/bin -xzf /tmp/upm-k8s-artifacts/cni-plugins-linux-amd64-v1.8.0.tgz
cp /tmp/upm-k8s-artifacts/containerd.service /etc/systemd/system/containerd.service
/usr/local/bin/containerd config default >/etc/containerd/config.toml
sed -ri 's/SystemdCgroup = false/SystemdCgroup = true/' /etc/containerd/config.toml
systemctl daemon-reload
systemctl enable --now containerd
dnf install -y /tmp/upm-k8s-artifacts/rpms/*.rpm --disableexcludes=kubernetes
systemctl enable --now kubelet
```

Master only:

```bash
tar -C /tmp -xzf /tmp/upm-k8s-artifacts/helm-v3.19.0-linux-amd64.tar.gz
install -m 755 /tmp/linux-amd64/helm /usr/local/bin/helm
kubeadm init \
  --kubernetes-version v1.35.5 \
  --apiserver-advertise-address=192.168.35.201 \
  --control-plane-endpoint=192.168.35.201 \
  --pod-network-cidr=192.168.0.0/16 \
  --cri-socket=unix:///run/containerd/containerd.sock
mkdir -p /root/.kube
cp -f /etc/kubernetes/admin.conf /root/.kube/config
chmod 600 /root/.kube/config
kubectl apply -f /tmp/upm-k8s-artifacts/calico.yaml
kubectl patch felixconfiguration default --type=merge -p '{"spec":{"defaultEndpointToHostAction":"Accept"}}'
kubectl apply -f /tmp/upm-k8s-artifacts/local-path-storage.yaml
kubectl patch storageclass local-path -p '{"metadata":{"annotations":{"storageclass.kubernetes.io/is-default-class":"true"}}}'
```

Worker nodes only:

```bash
kubeadm join 192.168.35.201:6443 --token <redacted> \
  --discovery-token-ca-cert-hash sha256:<redacted> \
  --cri-socket=unix:///run/containerd/containerd.sock
```

Local Mac artifact preparation:

```bash
mkdir -p /private/tmp/upm-k8s-artifacts/rpms /private/tmp/upm-k8s-artifacts/images
curl -fsSLO https://pkgs.k8s.io/core:/stable:/v1.35/rpm/repodata/repomd.xml
curl -fSLO https://github.com/containerd/containerd/releases/download/v2.1.5/containerd-2.1.5-linux-amd64.tar.gz
curl -fSLO https://github.com/opencontainers/runc/releases/download/v1.3.0/runc.amd64
curl -fSLO https://github.com/containernetworking/plugins/releases/download/v1.8.0/cni-plugins-linux-amd64-v1.8.0.tgz
curl -fsSLo calico.yaml https://raw.githubusercontent.com/projectcalico/calico/v3.30.0/manifests/calico.yaml
curl -fsSLo local-path-storage.yaml https://raw.githubusercontent.com/rancher/local-path-provisioner/v0.0.32/deploy/local-path-storage.yaml
curl -fSLO https://get.helm.sh/helm-v3.19.0-linux-amd64.tar.gz
```

## Key Findings

| Area | Plan |
|---|---|
| Kubernetes | v1.35.5, current patch in the v1.35 stable RPM repo at execution time |
| Runtime | containerd 2.1.5 with systemd cgroups |
| CNI | Calico v3.30.0, using pod CIDR `192.168.0.0/16` |
| StorageClass | Rancher local-path-provisioner v0.0.32, default class `local-path` |
| Helm | Helm v3.19.0 on master |
| Host packages | Only Kubernetes prerequisites and runtime dependencies |
| Metrics server | Deferred; explicitly pending for later validation |

RHEL 9.6 notes:

- SELinux was `Disabled` on all nodes before Kubernetes installation.
- `firewalld` was `inactive` on all nodes.
- Swap was initially present in the VM image and was disabled for Kubernetes.
- Required kernel modules are `overlay` and `br_netfilter`.
- Required sysctls are bridge netfilter forwarding and `net.ipv4.ip_forward=1`.
- Calico IPIP on this RHEL/VM network requires `rp_filter=0` on physical/tunnel/default interfaces; otherwise worker Pods cannot connect to the API Server endpoint.
- Calico Felix was configured with `defaultEndpointToHostAction: Accept` to allow workload-to-host traffic needed by in-cluster clients.
- The VM subnet gateway `192.168.35.1` responds, but outbound HTTPS registry access from the VMs was blocked/timeouts. Installation therefore uses locally downloaded artifacts and preloaded container images.
- DNS resolution on the hosts was adequate for local package repositories, but external registry access was not usable from the VMs.
- `chronyd` was active during node inspection.

## Evidence

Host inspection evidence:

```text
192.168.35.201 upm-k8s-master01 Red Hat Enterprise Linux release 9.6 (Plow)
192.168.35.201 selinux=Disabled firewalld=inactive swap=0 ip_forward=1
192.168.35.202 upm-k8s-worker01 Red Hat Enterprise Linux release 9.6 (Plow)
192.168.35.202 selinux=Disabled firewalld=inactive swap=0 ip_forward=1
192.168.35.203 upm-k8s-worker02 Red Hat Enterprise Linux release 9.6 (Plow)
192.168.35.203 selinux=Disabled firewalld=inactive swap=0 ip_forward=1
192.168.35.204 upm-k8s-worker03 Red Hat Enterprise Linux release 9.6 (Plow)
192.168.35.204 selinux=Disabled firewalld=inactive swap=0 ip_forward=1
```

Network evidence:

```text
ip route before fix: 192.168.35.0/24 dev ens192 proto kernel scope link src 192.168.35.201
ping 192.168.35.1: reachable
curl https://registry.k8s.io/v2/: connection timed out from VM
net.ipv4.conf.ens192.rp_filter = 0
net.ipv4.conf.tunl0.rp_filter = 0
worker Pod nc 192.168.35.201 6443: open
worker Pod nc 10.96.0.1 443: open
```

Version evidence from installed tools:

```text
containerd github.com/containerd/containerd/v2 v2.1.5
kubeadm version: GitVersion:"v1.35.5"
kubectl Client Version: v1.35.5
helm v3.19.0
```

## Open Questions

- Whether VM outbound HTTPS should be fixed permanently by platform networking, or whether future demos should continue to use an offline artifact workflow.
- Whether metrics-server should be installed before UPMIO runtime validation.
- Whether local-path-provisioner is acceptable for all hackathon demos, or whether a multi-node replicated storage layer is needed later.

## Conclusion

The recommended environment is kubeadm Kubernetes v1.35.5 on RHEL 9.6 with containerd, Calico, local-path-provisioner, and Helm 3. This fits the hackathon objective and avoids direct host-level application services.

## Impact on Future ClickHouse Spec

The first ClickHouse spec should assume Kubernetes-native deployment only. It should also account for offline image/package preparation or require a working cluster image registry path before any UPMIO or ClickHouse workloads are deployed.

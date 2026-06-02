# Phase 01 Kubernetes Installation Record

## Purpose

Record the actual Kubernetes installation performed on the four RHEL 9.6 VMs for UPMIO ClickHouse discovery. This document is evidence for cluster readiness only; no ClickHouse or UPMIO product workload was deployed.

## Commands Executed

All nodes:

```bash
ssh root@<node> 'cat >/etc/hosts ...'
ssh root@<node> 'swapoff -a'
ssh root@<node> 'sed -ri "/[[:space:]]swap[[:space:]]/ s/^/# /" /etc/fstab'
ssh root@<node> 'modprobe overlay; modprobe br_netfilter'
ssh root@<node> 'cat >/etc/sysctl.d/99-kubernetes-cri.conf ...; sysctl --system'
ssh root@<node> 'dnf install -y conntrack-tools socat yum-utils iproute-tc iptables-nft ethtool curl tar bash-completion'
ssh root@<node> 'ip route replace default via 192.168.35.1 dev ens192'
ssh root@<node> 'cat >/etc/sysctl.d/98-calico-rpfilter.conf ...; sysctl --system'
ssh root@<node> 'tar -C /usr/local -xzf /tmp/upm-k8s-artifacts/containerd-2.1.5-linux-amd64.tar.gz'
ssh root@<node> 'install -m 755 /tmp/upm-k8s-artifacts/runc.amd64 /usr/local/sbin/runc'
ssh root@<node> 'tar -C /opt/cni/bin -xzf /tmp/upm-k8s-artifacts/cni-plugins-linux-amd64-v1.8.0.tgz'
ssh root@<node> 'containerd config default >/etc/containerd/config.toml'
ssh root@<node> 'sed -ri "s/SystemdCgroup = false/SystemdCgroup = true/" /etc/containerd/config.toml'
ssh root@<node> 'systemctl enable --now containerd'
ssh root@<node> 'dnf install -y /tmp/upm-k8s-artifacts/rpms/*.rpm --disableexcludes=kubernetes'
ssh root@<node> 'systemctl enable --now kubelet'
ssh root@<node> 'ctr -n k8s.io images import /tmp/upm-k8s-artifacts/images/*.tar'
ssh root@<node> 'ctr -n k8s.io images tag registry.k8s.io/pause:3.10.1 registry.k8s.io/pause:3.10'
```

Master node:

```bash
ssh root@192.168.35.201 'install -m 755 /tmp/linux-amd64/helm /usr/local/bin/helm'
ssh root@192.168.35.201 'kubeadm init --kubernetes-version v1.35.5 --apiserver-advertise-address=192.168.35.201 --control-plane-endpoint=192.168.35.201 --pod-network-cidr=192.168.0.0/16 --cri-socket=unix:///run/containerd/containerd.sock'
ssh root@192.168.35.201 'mkdir -p /root/.kube; cp -f /etc/kubernetes/admin.conf /root/.kube/config; chmod 600 /root/.kube/config'
ssh root@192.168.35.201 'kubectl apply -f /tmp/upm-k8s-artifacts/calico.yaml'
ssh root@192.168.35.201 'kubectl patch felixconfiguration default --type=merge -p ...'
ssh root@192.168.35.201 'kubectl apply -f /tmp/upm-k8s-artifacts/local-path-storage.yaml'
ssh root@192.168.35.201 'kubectl patch storageclass local-path -p ...'
```

Worker nodes:

```bash
ssh root@192.168.35.202 'kubeadm join 192.168.35.201:6443 --token <redacted> --discovery-token-ca-cert-hash sha256:<redacted> --cri-socket=unix:///run/containerd/containerd.sock'
ssh root@192.168.35.203 'kubeadm join 192.168.35.201:6443 --token <redacted> --discovery-token-ca-cert-hash sha256:<redacted> --cri-socket=unix:///run/containerd/containerd.sock'
ssh root@192.168.35.204 'kubeadm join 192.168.35.201:6443 --token <redacted> --discovery-token-ca-cert-hash sha256:<redacted> --cri-socket=unix:///run/containerd/containerd.sock'
```

Local Mac:

```bash
mkdir -p /private/tmp/upm-k8s-artifacts/rpms /private/tmp/upm-k8s-artifacts/images
curl -fsSLO https://pkgs.k8s.io/core:/stable:/v1.35/rpm/repodata/repomd.xml
curl -fSL -o go-containerregistry_Darwin_x86_64.tar.gz https://github.com/google/go-containerregistry/releases/download/v0.20.6/go-containerregistry_Darwin_x86_64.tar.gz
./crane pull --platform=linux/amd64 <image> /private/tmp/upm-k8s-artifacts/images/<image>.tar
scp -r /private/tmp/upm-k8s-artifacts root@<node>:/tmp/upm-k8s-artifacts
```

## Key Findings

- Cluster is ready: one control-plane and three worker nodes are `Ready`.
- Kubernetes, kubeadm, kubelet, and kubectl are v1.35.5.
- Runtime is containerd 2.1.5.
- CNI is Calico v3.30.0.
- StorageClass is `local-path (default)`.
- Helm is installed on master as v3.19.0.
- metrics-server is not installed and is intentionally marked pending.
- UPMIO operators are not installed in this phase.

## Issues Encountered

| Issue | Symptom | Root Cause | Diagnosis Command | Fix Applied | Final Status |
|---|---|---|---|---|---|
| VM external registry unreachable | `curl https://registry.k8s.io/v2/` timed out from VM | VM outbound HTTPS/NAT path unavailable despite local gateway being reachable | `ip route`, `curl -I https://registry.k8s.io/v2/` | Download RPMs/images on local Mac, copy to nodes, import into containerd | Resolved for installation; external access remains an environment limitation |
| pause image tag mismatch | kubelet tried to pull `registry.k8s.io/pause:3.10` although `3.10.1` was preloaded | container runtime sandbox image tag differed from kubeadm image list | `journalctl -u kubelet`, `crictl images` | `ctr -n k8s.io images tag registry.k8s.io/pause:3.10.1 registry.k8s.io/pause:3.10` | Resolved |
| Pod to API Server timeout | `local-path-provisioner` entered `CrashLoopBackOff`; worker Pod could resolve DNS but timed out to `10.96.0.1:443` | physical/tunnel interface `rp_filter=1` dropped asymmetric Pod CIDR traffic in Calico IPIP path; workload-to-host traffic also needed explicit Calico host action | `kubectl logs deploy/local-path-provisioner`, worker `nc 10.96.0.1 443`, `sysctl net.ipv4.conf.*.rp_filter` | Set `rp_filter=0` for all/default/ens192/tunl0 and patched Felix `defaultEndpointToHostAction: Accept` | Resolved; worker Pod can connect to API Server |

## Evidence

`kubectl get nodes -o wide`:

```text
NAME               STATUS   ROLES           AGE   VERSION   INTERNAL-IP      EXTERNAL-IP   OS-IMAGE                              KERNEL-VERSION                 CONTAINER-RUNTIME
upm-k8s-master01   Ready    control-plane   95s   v1.35.5   192.168.35.201   <none>        Red Hat Enterprise Linux 9.6 (Plow)   5.14.0-570.12.1.el9_6.x86_64   containerd://2.1.5
upm-k8s-worker01   Ready    <none>          49s   v1.35.5   192.168.35.202   <none>        Red Hat Enterprise Linux 9.6 (Plow)   5.14.0-570.12.1.el9_6.x86_64   containerd://2.1.5
upm-k8s-worker02   Ready    <none>          48s   v1.35.5   192.168.35.203   <none>        Red Hat Enterprise Linux 9.6 (Plow)   5.14.0-570.12.1.el9_6.x86_64   containerd://2.1.5
upm-k8s-worker03   Ready    <none>          47s   v1.35.5   192.168.35.204   <none>        Red Hat Enterprise Linux 9.6 (Plow)   5.14.0-570.12.1.el9_6.x86_64   containerd://2.1.5
```

`kubectl get pods -A -o wide`:

```text
NAMESPACE            NAME                                       READY   STATUS    RESTARTS   AGE   IP                NODE
kube-system          calico-kube-controllers-846967dd59-5ct4r   1/1     Running   0          13m   192.168.27.65     upm-k8s-master01
kube-system          calico-node-429lq                          1/1     Running   0          3m    192.168.35.204    upm-k8s-worker03
kube-system          calico-node-48l9g                          1/1     Running   0          2m    192.168.35.202    upm-k8s-worker01
kube-system          calico-node-d666m                          1/1     Running   0          2m    192.168.35.201    upm-k8s-master01
kube-system          calico-node-qvbw7                          1/1     Running   0          3m    192.168.35.203    upm-k8s-worker02
kube-system          coredns-7d764666f9-cpgp7                   1/1     Running   0          14m   192.168.27.66     upm-k8s-master01
kube-system          coredns-7d764666f9-wxq4b                   1/1     Running   0          14m   192.168.27.67     upm-k8s-master01
kube-system          etcd-upm-k8s-master01                      1/1     Running   0          14m   192.168.35.201    upm-k8s-master01
kube-system          kube-apiserver-upm-k8s-master01            1/1     Running   0          14m   192.168.35.201    upm-k8s-master01
kube-system          kube-controller-manager-upm-k8s-master01   1/1     Running   0          14m   192.168.35.201    upm-k8s-master01
kube-system          kube-proxy-4tlg5                           1/1     Running   0          13m   192.168.35.203    upm-k8s-worker02
kube-system          kube-proxy-547ph                           1/1     Running   0          14m   192.168.35.201    upm-k8s-master01
kube-system          kube-proxy-6slx7                           1/1     Running   0          13m   192.168.35.204    upm-k8s-worker03
kube-system          kube-proxy-kbht2                           1/1     Running   0          13m   192.168.35.202    upm-k8s-worker01
kube-system          kube-scheduler-upm-k8s-master01            1/1     Running   0          14m   192.168.35.201    upm-k8s-master01
local-path-storage   local-path-provisioner-9c88668cf-kwqx9     1/1     Running   0          46s   192.168.244.66    upm-k8s-worker02
```

Post-fix API connectivity evidence:

```text
192.168.35.201 (192.168.35.201:6443) open
10.96.0.1 (10.96.0.1:443) open
local-path-storage   local-path-provisioner-9c88668cf-kwqx9     1/1     Running   0
```

`kubectl get sc`:

```text
NAME                   PROVISIONER             RECLAIMPOLICY   VOLUMEBINDINGMODE      ALLOWVOLUMEEXPANSION   AGE
local-path (default)   rancher.io/local-path   Delete          WaitForFirstConsumer   false                  26s
```

`kubectl cluster-info`:

```text
Kubernetes control plane is running at https://192.168.35.201:6443
CoreDNS is running at https://192.168.35.201:6443/api/v1/namespaces/kube-system/services/kube-dns:dns/proxy
```

`kubectl version`:

```text
Client Version: v1.35.5
Kustomize Version: v5.7.1
Server Version: v1.35.5
```

`helm version`:

```text
version.BuildInfo{Version:"v3.19.0", GitCommit:"3d8990f0836691f0229297773f3524598f46bda6", GitTreeState:"clean", GoVersion:"go1.24.7"}
```

UPMIO runtime inspection:

```text
kubectl get crd | grep -Ei "upm|unit|compose|mysql|redis|postgres|proxy|servicegroup" || true
# no UPMIO CRDs installed

kubectl api-resources | grep -Ei "upm|unit|compose|mysql|redis|postgres|proxy|servicegroup" || true
# no UPMIO API resources installed
```

## Open Questions

- Should metrics-server be installed before UPMIO operator validation?
- Should a persistent default gateway be configured at the VM network layer rather than temporary `ip route replace`?
- Should the hackathon cluster use a private registry mirror to avoid repeated local image import?

## Conclusion

The Kubernetes cluster is ready for UPMIO discovery and later demo validation. No application service was installed directly on the host; only Kubernetes prerequisites, container runtime, kubeadm/kubelet/kubectl, Helm, CNI, and local-path storage were installed.

## Impact on Future ClickHouse Spec

The future ClickHouse spec can target a 1-control-plane/3-worker Kubernetes cluster with default local persistent volumes. Any ClickHouse package validation should plan for image preloading or a reachable private registry.

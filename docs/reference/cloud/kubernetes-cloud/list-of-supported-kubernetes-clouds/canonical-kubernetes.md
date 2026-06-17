---
myst:
  html_meta:
    description: "Set up Canonical Kubernetes cloud with Juju, including required services like DNS, ingress, local storage, and bootstrap configuration."
---

(cloud-canonical-k8s)=
# Canonical Kubernetes

In Juju, [Canonical Kubernetes](https://documentation.ubuntu.com/canonical-kubernetes/) is a {ref}`Kubernetes cloud <kubernetes-cloud>`. It behaves like all Kubernetes clouds, except for a few points of variation related to the cloud, described below.

```{dropdown} Example workflow
1. Ensure required services are enabled in the cluster (`dns`, `ingress`, `local-storage`, `network`).
2. Add the Kubernetes cloud with `juju add-k8s canonical-k8s`.
3. Select the `kubeconfig` context when prompted; Juju imports both cloud and credential data from that context.
4. Bootstrap with `juju bootstrap canonical-k8s canonical-k8s-controller`.
```

## Requirements

**Services that must be enabled:**

- `dns`
- `ingress` (technically not required, but you need it if you want to do anything meaningful).
- `local-storage`
- `network`

(canonical-k8s-cloud)=
## The cloud

```{ibnote}
See also: {ref}`cloud`, {ref}`Juju | Manage clouds <manage-clouds>`, {ref}`Terraform Provider for Juju | Manage clouds <tfjuju:manage-clouds>`
```

(canonical-k8s-controller)=
## Controller

```{ibnote}
See also: {ref}`controller`, {ref}`Juju | Manage controllers <manage-controllers>`, {ref}`Terraform Provider for Juju | Manage controllers <tfjuju:manage-controllers>`
```

### Bootstrap preparation

Before bootstrapping this cloud:

- Create a custom `containerd` path. For example:

```text
export containerdBaseDir="/run/containerd-k8s"
```

- Resize `/run`. For example:

```text
sudo mount -o remount,size=10G /run
```


```{ibnote}
See more: https://github.com/canonical/k8s-snap/issues/1612
```
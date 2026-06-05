---
myst:
  html_meta:
    description: "Set up Canonical Kubernetes cloud with Juju, including required services like DNS, ingress, local storage, and bootstrap configuration."
---

(cloud-canonical-k8s)=
# Canonical Kubernetes

In Juju, [Canonical Kubernetes](https://documentation.ubuntu.com/canonical-kubernetes/) is a {ref}`Kubernetes cloud <kubernetes-cloud>`. It behaves like all {ref}`Kubernetes clouds <kubernetes-clouds>`, except for a few points of variation related to the cloud, described below.

(canonical-k8s-cloud)=
## The cloud

```{ibnote}
See also: {ref}`Juju | Manage clouds <manage-clouds>`, {ref}`Terraform Provider for Juju | Manage clouds <tfjuju:manage-clouds>`
```

### Requirements

**Services that must be enabled:**

- `dns`
- `ingress` (technically not required, but you need it if you want to do anything meaningful)
- `local-storage`
- `network`

### Adding the cloud

Before you bootstrap:

- Create a custom `containerd` path, e.g., `export containerdBaseDir="/run/containerd-k8s"`.

- Resize `/run`, e.g., `sudo mount -o remount,size=10G /run`.

```{ibnote}
See more: https://github.com/canonical/k8s-snap/issues/1612
```
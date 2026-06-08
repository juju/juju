---
myst:
  html_meta:
    description: "Set up MicroK8s cloud with Juju for localhost Kubernetes deployments, including snap installation and required service configuration."
---

(cloud-kubernetes-microk8s)=
# MicroK8s

In Juju, [MicroK8s](https://microk8s.io/) is a {ref}`Kubernetes cloud <kubernetes-cloud>`. It behaves like all Kubernetes clouds, except for a few points of variation related to the cloud, described below.

```{dropdown} Example workflow
Before starting, install MicroK8s and enable required add-ons (`dns`, `hostpath-storage`).

1. Add the Kubernetes cloud with `juju add-k8s microk8s`.
2. Select or confirm the kubeconfig context and credentials when prompted.
3. Bootstrap with `juju bootstrap microk8s microk8s-controller`.
```

(microk8s-cloud)=
## Cloud definition

```{ibnote}
See also: {ref}`cloud`, {ref}`Juju | Manage clouds <manage-clouds>`, {ref}`Terraform Provider for Juju | Manage clouds <tfjuju:manage-clouds>`
```

### Requirements

**Services that must be enabled:**

- `dns`
- `hostpath-storage`

### Adding the cloud

For a localhost MicroK8s cloud, if you would like to be able to skip `juju add-k8s`, install MicroK8s from the strictly confined snap.

```{ibnote}
See more: [MicroK8s | Strict MicroK8s](https://microk8s.io/docs/install-strict)
```
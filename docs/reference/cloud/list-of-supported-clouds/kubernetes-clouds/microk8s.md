---
myst:
  html_meta:
    description: "Set up MicroK8s cloud with Juju for localhost Kubernetes deployments, including snap installation and required service configuration."
---

(cloud-kubernetes-microk8s)=
# MicroK8s

In Juju, [MicroK8s](https://microk8s.io/) is a {ref}`Kubernetes cloud <kubernetes-cloud>`. It behaves like all {ref}`Kubernetes clouds <kubernetes-clouds>`, except for a few cloud-specific notes and storage providers, described below.

## Cloud-specific notes

## Cloud-specific notes

### Requirements

### MicroK8s snap

For a localhost MicroK8s cloud, if you would like to be able to skip `juju add-k8s`, install MicroK8s from the strictly confined snap.

```{ibnote}
See more: [MicroK8s | Strict MicroK8s](https://microk8s.io/docs/install-strict)
```

### Services that must enabled

- `dns`
- `hostpath-storage`

## Storage

Storage provisioned on the MicroK8s cloud.

```{ibnote}
See first: {ref}`storage-provider`
```

As for all Kubernetes clouds. See {ref}`storage-provider-kubernetes`.
---
myst:
  html_meta:
    description: "Set up MicroK8s cloud with Juju for localhost Kubernetes deployments, including snap installation and required service configuration."
---

(cloud-kubernetes-microk8s)=
# The MicroK8s cloud and Juju

This document describes details specific to using a MicroK8s cloud with Juju.

```{ibnote}
See more: [Getting started on Microk8s](https://microk8s.io/docs/getting-started)
```

In Juju, MicroK8s is a {ref}`kubernetes-cloud`.

```{ibnote}
See more: {ref}`kubernetes-clouds` (for complete Kubernetes cloud documentation)
```

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

## Cloud-specific storage providers

```{ibnote}
See first: {ref}`storage-provider`
```

As for all Kubernetes clouds. See {ref}`storage-provider-kubernetes`.
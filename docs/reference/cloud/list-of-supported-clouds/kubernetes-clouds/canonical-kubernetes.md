---
myst:
  html_meta:
    description: "Set up Canonical Kubernetes cloud with Juju, including required services like DNS, ingress, local storage, and bootstrap configuration."
---

(cloud-canonical-k8s)=
# Canonical Kubernetes

In Juju, [Canonical Kubernetes](https://documentation.ubuntu.com/canonical-kubernetes/) is a {ref}`Kubernetes cloud <kubernetes-cloud>`. It behaves like all {ref}`Kubernetes clouds <kubernetes-clouds>`, except for a few cloud-specific notes and storage providers, described below.

## Cloud-specific notes

## Cloud-specific notes

### Requirements

### Services that must enabled

- `dns`
- `ingress` (technically not required, but you need it if you want to do anything meaningful)
- `local-storage`
- `network`

## Notes on `juju add-k8s`

Before you bootstrap:

- Create a custom `containerd` path, e.g., `export containerdBaseDir="/run/containerd-k8s"`.

- Resize `/run`, e.g., `sudo mount -o remount,size=10G /run`.

```{ibnote}
See more: https://github.com/canonical/k8s-snap/issues/1612
```

## Storage

Storage provisioned on the Canonical Kubernetes cloud.

```{ibnote}
See first: {ref}`storage-provider`
```

As for all Kubernetes clouds. See {ref}`storage-provider-kubernetes`.
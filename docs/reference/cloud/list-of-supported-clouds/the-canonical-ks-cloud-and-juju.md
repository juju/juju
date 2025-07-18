(cloud-canonical-k8s)=
# The Canonical Kubernetes cloud and Juju

This document describes details specific to using a Canonical Kubernetes cloud with Juju.

> See more: [Canonical Kubernetes documentation](https://documentation.ubuntu.com/canonical-kubernetes/)

When using this cloud with Juju, it is important to keep in mind that it is a (1) Kubernetes cloud and (2) not some other cloud.

> See more: {ref}`cloud-differences`

As the differences related to (1) are already documented generically in the rest of the docs, here we record just those that follow from (2).


## Requirements

### Services that must enabled

- `dns`
- `ingress` (technically not required, but you need it if you want to do anything meaningful)
- `local-storage`
- `network`

## Notes on `juju add-k8s`

Before you bootstrap:

- You need to create a custom `containerd` path, e.g., `export containerdBaseDir="/run/containerd-k8s"`.

- For most purposes, you should also resize `/run`, e.g., `sudo mount -o remount,size=10G /run`.

> See more: https://github.com/canonical/k8s-snap/issues/1612
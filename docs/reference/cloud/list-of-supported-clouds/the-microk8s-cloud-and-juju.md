(cloud-kubernetes-microk8s)=
# The MicroK8s cloud and Juju

This document describes details specific to using your a MicroK8s cloud with Juju.

> See more: [Getting started on Microk8s](https://microk8s.io/docs/getting-started)

When using this cloud with Juju, it is important to keep in mind that it is a (1) Kubernetes cloud and (2) not some other cloud.

> See more: {ref}`cloud-differences`

As the differences related to (1) are already documented generically in the rest of the docs, here we record just those that follow from (2).


## Requirements

### MicroK8s snap

For a localhost MicroK8s cloud, if you would like to be able to skip `juju add-k8s`, install MicroK8s from the strictly confined snap.

> See more: [MicroK8s | Strict MicroK8s](https://microk8s.io/docs/install-strict)

### Services that must enabled

- `dns`
- `hostpath-storage`


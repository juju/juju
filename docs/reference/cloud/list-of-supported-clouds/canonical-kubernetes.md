---
myst:
  html_meta:
    description: "Set up Canonical Kubernetes cloud with Juju, including required services like DNS, ingress, local storage, and bootstrap configuration."
---

(cloud-canonical-k8s)=
# Canonical Kubernetes

In Juju, [Canonical Kubernetes](https://documentation.ubuntu.com/canonical-kubernetes/) is a {ref}`Kubernetes cloud <kubernetes-cloud>` and works as described below.

```{note}
This reference assumes basic familiarity with Juju. If you are new to Juju, start with the {ref}`Tutorial <tutorial>`, then use this page together with the generic materials it links to.
```

## Concepts

```{include} ./reuse/k8s/concepts-table.md
```

## The cloud

```{include} ./reuse/k8s/cloud-definition.md
```

(canonical-k8s-requirements)=
### Requirements

**Services that must be enabled:**

- `dns`
- `ingress` (technically not required, but you need it if you want to do anything meaningful).
- `local-storage`
- `network`

## Credentials

```{include} ./reuse/k8s/auth-types.md
```

## Controllers

```{include} ./reuse/k8s/bootstrap-resources.md
```

```{include} ./reuse/k8s/controller-service-type.md
```

(canonical-k8s-controller)=
### Bootstrap preparation

```{ibnote}
See also: {ref}`controller`, {ref}`Juju | Manage controllers <manage-controllers>`, {ref}`Terraform Provider for Juju | Manage controllers <tfjuju:manage-controllers>`
```

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

## Models

```{include} ./reuse/k8s/model-config-keys.md
```

## Pods

```{include} ./reuse/k8s/constraints.md
```

```{include} ./reuse/k8s/pod-deployment-patterns.md
```

## Storage

```{include} ./reuse/k8s/storage-provider.md
```

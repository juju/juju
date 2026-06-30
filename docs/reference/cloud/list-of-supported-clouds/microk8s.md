---
myst:
  html_meta:
    description: "Set up MicroK8s cloud with Juju for localhost Kubernetes deployments, including snap installation and required service configuration."
---

(cloud-kubernetes-microk8s)=
# MicroK8s

In Juju, [MicroK8s](https://microk8s.io/) is a {ref}`Kubernetes cloud <kubernetes-cloud>` and works as described below.

```{note}
This reference assumes basic familiarity with Juju. If you are new to Juju, start with the {ref}`Tutorial <tutorial>`, then use this page together with the generic materials it links to.
```

(microk8s-requirements)=
## Requirements

**Services that must be enabled:**

- `dns`
- `hostpath-storage`

## Concepts

```{include} ./reuse/k8s/concepts-table.md
```

## The cloud

```{include} ./reuse/k8s/cloud-definition.md
```

(microk8s-cloud-adding)=
### Adding the cloud

For a localhost MicroK8s cloud, if you would like to be able to skip `juju add-k8s`, install MicroK8s from the strictly confined snap.

```{ibnote}
See more: [MicroK8s | Strict MicroK8s](https://microk8s.io/docs/install-strict)
```

## Credentials

```{include} ./reuse/k8s/auth-types.md
```

## Controllers

```{include} ./reuse/k8s/bootstrap-resources.md
```

```{include} ./reuse/k8s/controller-service-type.md
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

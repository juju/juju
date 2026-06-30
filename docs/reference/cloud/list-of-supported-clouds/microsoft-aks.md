---
myst:
  html_meta:
    description: "Deploy applications on Microsoft Azure Kubernetes Service (AKS) with Juju, including Kubernetes cloud setup and storage configuration."
---

(cloud-kubernetes-aks)=
# Microsoft AKS

In Juju, [Microsoft AKS](https://docs.microsoft.com/en-us/azure/aks/) is a {ref}`Kubernetes cloud <kubernetes-cloud>` and works as described below.

```{note}
This reference assumes basic familiarity with Juju. If you are new to Juju, start with the {ref}`Tutorial <tutorial>`, then use this page together with the generic materials it links to.
```

## Concepts

```{include} ./reuse/k8s/concepts-table.md
```

## The cloud

```{include} ./reuse/k8s/cloud-definition.md
```

(aks-cloud-adding)=
### Adding the cloud

When adding this cloud to Juju using the {ref}`juju CLI client <juju-client>`, starting with Juju 3.0 you must run the `add-k8s` command with the 'raw' client because the `juju` client snap is strictly confined but the AKS cloud CLI snap is not.

```{ibnote}
See more: {ref}`add-a-kubernetes-cloud`
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

```Distribution-specific notes

(aks-cloud-adding)=
### Adding the cloud

When adding this cloud to Juju using the {ref}`juju CLI client <juju-client>`, starting with Juju 3.0 you must run the `add-k8s` command with the 'raw' client because the `juju` client snap is strictly confined but the AKS cloud CLI snap is not.

```{ibnote}
See more: {ref}`add-a-kubernetes-cloud`
```

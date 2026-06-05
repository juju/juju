---
myst:
  html_meta:
    description: "Deploy applications on Microsoft Azure Kubernetes Service (AKS) with Juju, including Kubernetes cloud setup and storage configuration."
---

(cloud-kubernetes-aks)=
# The Microsoft AKS cloud and Juju

This document describes details specific to using your existing Microsoft AKS cloud with Juju.

```{ibnote}
See more: [Microsoft AKS](https://azure.microsoft.com/en-us/products/kubernetes-service)
```

In Juju, Microsoft AKS is a {ref}`kubernetes-cloud`.

```{ibnote}
See more: {ref}`kubernetes-clouds-and-juju` (for complete Kubernetes cloud documentation)
```

## Cloud-specific notes

### Notes on `add-k8s`

Starting with Juju 3.0, because of  the  fact that the `juju` client snap is strictly confined but the AKS cloud CLI snap is not, you must run the `add-k8s` command with the 'raw' client. See note in {ref}`add-a-kubernetes-cloud`.

## Cloud-specific storage providers

As for all Kubernetes clouds. See {ref}`storage-provider-kubernetes`.
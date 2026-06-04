---
myst:
  html_meta:
    description: "Deploy applications on Amazon EKS with Juju, including Kubernetes cloud setup, storage providers, and EKS-specific configuration details."
---

(cloud-kubernetes-eks)=
# The Amazon EKS cloud and Juju

This document describes details specific to using your existing Amazon EKS cloud with Juju.

```{ibnote}
See more: [Amazon EKS](https://docs.aws.amazon.com/eks/index.html)
```

In Juju, Amazon EKS is a {ref}`kubernetes-cloud`.

```{ibnote}
See more: {ref}`kubernetes-clouds-and-juju` (for complete Kubernetes cloud documentation)
```

## Cloud-specific notes

### Notes on `add-k8s`

Starting with Juju 3.0, because of the  fact that the `juju` client snap is strictly confined but the EKS cloud CLI snap is not, you must run the `add-k8s` command with the 'raw' client. See note in {ref}`add-a-kubernetes-cloud`.

## Cloud-specific storage providers

```{ibnote}
See first: {ref}`storage-provider`
```

As for all Kubernetes clouds. See {ref}`storage-provider-kubernetes`.
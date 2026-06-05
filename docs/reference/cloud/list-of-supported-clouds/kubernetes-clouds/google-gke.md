---
myst:
  html_meta:
    description: "Deploy workloads on Google Kubernetes Engine (GKE) with Juju, including add-k8s command usage and storage provider configuration."
---

(cloud-kubernetes-gke)=
# The Google GKE cloud and Juju

This document describes details specific to using your existing Google GKE cloud with Juju.

```{ibnote}
See more: [Google GKE](https://cloud.google.com/kubernetes-engine/docs)
```

In Juju, Google GKE is a {ref}`kubernetes-cloud`.

```{ibnote}
See more: {ref}`kubernetes-clouds-and-juju` (for complete Kubernetes cloud documentation)
```

## Cloud-specific notes

### Notes on `add-k8s`

Starting with Juju 3.0, because of the  fact that the `juju` client snap is strictly confined but the GKE cloud CLI snap is not, you must run the `add-k8s` command with the 'raw' client. See note in {ref}`add-a-kubernetes-cloud`.

## Cloud-specific storage providers

```{ibnote}
See first: {ref}`storage-provider`
```

As for all Kubernetes clouds. See {ref}`storage-provider-kubernetes`.
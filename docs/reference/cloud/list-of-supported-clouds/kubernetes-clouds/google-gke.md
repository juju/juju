---
myst:
  html_meta:
    description: "Deploy workloads on Google Kubernetes Engine (GKE) with Juju, including add-k8s command usage and storage provider configuration."
---

(cloud-kubernetes-gke)=
# Google GKE

In Juju, [Google GKE](https://cloud.google.com/kubernetes-engine/docs) is a {ref}`Kubernetes cloud <kubernetes-cloud>`. It behaves like all {ref}`Kubernetes clouds <kubernetes-clouds>`, except for a few cloud-specific notes and storage providers, described below.

## Cloud-specific notes

### Notes on `add-k8s`

Starting with Juju 3.0, because of the  fact that the `juju` client snap is strictly confined but the GKE cloud CLI snap is not, you must run the `add-k8s` command with the 'raw' client. See note in {ref}`add-a-kubernetes-cloud`.

## Storage

Storage provisioned on the Google GKE cloud.

```{ibnote}
See first: {ref}`storage-provider`
```

As for all Kubernetes clouds. See {ref}`storage-provider-kubernetes`.
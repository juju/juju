---
myst:
  html_meta:
    description: "Deploy applications on Microsoft Azure Kubernetes Service (AKS) with Juju, including Kubernetes cloud setup and storage configuration."
---

(cloud-kubernetes-aks)=
# Microsoft AKS

In Juju, [Microsoft AKS](https://docs.microsoft.com/en-us/azure/aks/) is a {ref}`Kubernetes cloud <kubernetes-cloud>`. It behaves like all {ref}`Kubernetes clouds <kubernetes-clouds>`, except for a few cloud-specific notes described below.

## Cloud-specific notes

### Notes on `add-k8s`

Starting with Juju 3.0, because of  the  fact that the `juju` client snap is strictly confined but the AKS cloud CLI snap is not, you must run the `add-k8s` command with the 'raw' client. See note in {ref}`add-a-kubernetes-cloud`.
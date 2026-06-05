---
myst:
  html_meta:
    description: "Deploy applications on Microsoft Azure Kubernetes Service (AKS) with Juju, including Kubernetes cloud setup and storage configuration."
---

(cloud-kubernetes-aks)=
# Microsoft AKS

In Juju, [Microsoft AKS](https://docs.microsoft.com/en-us/azure/aks/) is a {ref}`Kubernetes cloud <kubernetes-cloud>`. It behaves like all {ref}`Kubernetes clouds <kubernetes-clouds>`, except for a few points of variation related to the cloud, described below.

(aks-cloud)=
## The cloud

```{ibnote}
See also: {ref}`Juju | Manage clouds <manage-clouds>`, {ref}`Terraform Provider for Juju | Manage clouds <tfjuju:manage-clouds>`
```

### Adding the cloud

When adding this cloud to Juju using the {ref}`juju CLI client <juju-client>`, starting with Juju 3.0 you must run the `add-k8s` command with the 'raw' client because the `juju` client snap is strictly confined but the AKS cloud CLI snap is not.

```{ibnote}
See more: {ref}`add-a-kubernetes-cloud`
```
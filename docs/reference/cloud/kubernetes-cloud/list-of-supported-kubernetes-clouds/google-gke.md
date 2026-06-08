---
myst:
  html_meta:
    description: "Deploy workloads on Google Kubernetes Engine (GKE) with Juju, including add-k8s command usage and storage provider configuration."
---

(cloud-kubernetes-gke)=
# Google GKE

In Juju, [Google GKE](https://cloud.google.com/kubernetes-engine/docs) is a {ref}`Kubernetes cloud <kubernetes-cloud>`. It behaves like all Kubernetes clouds, except for a few points of variation related to the cloud, described below.

```{dropdown} Example workflow
Before starting, authenticate to Google Cloud and fetch kubeconfig for the target GKE cluster.

1. Add the Kubernetes cloud with `juju add-k8s gke`.
2. Select or confirm the kubeconfig context and credentials when prompted.
3. Bootstrap with `juju bootstrap gke gke-controller`.
```

(gke-cloud)=
## Cloud definition

```{ibnote}
See also: {ref}`cloud`, {ref}`Juju | Manage clouds <manage-clouds>`, {ref}`Terraform Provider for Juju | Manage clouds <tfjuju:manage-clouds>`
```

### Adding the cloud

When adding this cloud to Juju using the {ref}`juju CLI client <juju-client>`, starting with Juju 3.0 you must run the `add-k8s` command with the 'raw' client because the `juju` client snap is strictly confined but the GKE cloud CLI snap is not.

```{ibnote}
See more: {ref}`add-a-kubernetes-cloud`
```
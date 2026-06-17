---
myst:
  html_meta:
    description: "Deploy workloads on Google Kubernetes Engine (GKE) with Juju, including add-k8s command usage and storage provider configuration."
---

(cloud-kubernetes-gke)=
# Google GKE

In Juju, [Google GKE](https://cloud.google.com/kubernetes-engine/docs) is a {ref}`Kubernetes cloud <kubernetes-cloud>`. It behaves like all Kubernetes clouds, except for a few points of variation related to the cloud, described below.

```{note}
This reference assumes basic familiarity with Juju. If you are new to Juju, start with the {ref}`tutorial`, then use this page together with the generic materials it links to. For a cloud-specific starting point, see {ref}`gke-appendix-example-workflows`.
```

(gke-cloud)=
## The cloud

```{ibnote}
See also: {ref}`cloud`, {ref}`Juju | Manage clouds <manage-clouds>`, {ref}`Terraform Provider for Juju | Manage clouds <tfjuju:manage-clouds>`
```

### Adding the cloud

When adding this cloud to Juju using the {ref}`juju CLI client <juju-client>`, starting with Juju 3.0 you must run the `add-k8s` command with the 'raw' client because the `juju` client snap is strictly confined but the GKE cloud CLI snap is not.

```{ibnote}
See more: {ref}`add-a-kubernetes-cloud`
```

(gke-appendix-example-workflows)=
## Appendix: Example workflows

(gke-appendix-quickstart)=
### Add cloud, add credential, bootstrap

1. Authenticate to Google Cloud and fetch `kubeconfig` for the target GKE cluster.
2. Add the Kubernetes cloud with `juju add-k8s gke`.
3. Select the `kubeconfig` context when prompted; Juju imports both cloud and credential data from that context.
4. Bootstrap with `juju bootstrap gke gke-controller`.
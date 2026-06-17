---
myst:
  html_meta:
    description: "Deploy applications on Amazon EKS with Juju, including Kubernetes cloud setup, storage providers, and EKS-specific configuration details."
---

(cloud-kubernetes-eks)=
# Amazon EKS

In Juju, [Amazon EKS](https://docs.aws.amazon.com/eks/index.html) is a {ref}`Kubernetes cloud <kubernetes-cloud>`. It behaves like all Kubernetes clouds, except for a few points of variation related to the cloud, described below.

```{note}
This reference assumes basic familiarity with Juju. If you are new to Juju, start with the {ref}`tutorial`, then use this page together with the generic materials it links to. For a cloud-specific starting point, see {ref}`eks-appendix-example-workflows`.
```

(eks-cloud)=
## The cloud

```{ibnote}
See also: {ref}`cloud`, {ref}`Juju | Manage clouds <manage-clouds>`, {ref}`Terraform Provider for Juju | Manage clouds <tfjuju:manage-clouds>`
```

### Adding the cloud

When adding this cloud to Juju using the {ref}`juju CLI client <juju-client>`, starting with Juju 3.0 you must run the `add-k8s` command with the 'raw' client because the `juju` client snap is strictly confined but the EKS cloud CLI snap is not.

```{ibnote}
See more: {ref}`add-a-kubernetes-cloud`
```

(eks-appendix-example-workflows)=
## Appendix: Example workflows

(eks-appendix-quickstart)=
### Add cloud, add credential, bootstrap

1. Configure AWS/EKS access and update `kubeconfig` for the target cluster.
2. Add the Kubernetes cloud with `juju add-k8s eks`.
3. Select the `kubeconfig` context when prompted; Juju imports both cloud and credential data from that context.
4. Bootstrap with `juju bootstrap eks eks-controller`.
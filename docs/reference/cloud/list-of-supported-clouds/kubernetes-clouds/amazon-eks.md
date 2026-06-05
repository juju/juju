---
myst:
  html_meta:
    description: "Deploy applications on Amazon EKS with Juju, including Kubernetes cloud setup, storage providers, and EKS-specific configuration details."
---

(cloud-kubernetes-eks)=
# Amazon EKS

In Juju, [Amazon EKS](https://docs.aws.amazon.com/eks/index.html) is a {ref}`Kubernetes cloud <kubernetes-cloud>`. It behaves like all {ref}`Kubernetes clouds <kubernetes-clouds>`, except for a few cloud-specific notes described below.

## Cloud-specific notes

### Notes on `add-k8s`

Starting with Juju 3.0, because of the  fact that the `juju` client snap is strictly confined but the EKS cloud CLI snap is not, you must run the `add-k8s` command with the 'raw' client. See note in {ref}`add-a-kubernetes-cloud`.
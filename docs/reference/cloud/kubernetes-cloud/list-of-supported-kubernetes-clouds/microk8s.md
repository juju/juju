---
myst:
  html_meta:
    description: "Set up MicroK8s cloud with Juju for localhost Kubernetes deployments, including snap installation and required service configuration."
---

(cloud-kubernetes-microk8s)=
# MicroK8s

In Juju, [MicroK8s](https://microk8s.io/) is a {ref}`Kubernetes cloud <kubernetes-cloud>`. It behaves like all Kubernetes clouds, except for a few points of variation related to the cloud, described below.

```{note}
This reference assumes basic familiarity with Juju. If you are new to Juju, start with the {ref}`tutorial`, then use this page together with the generic materials it links to. For a cloud-specific starting point, see {ref}`microk8s-appendix-example-workflows`.
```

## Requirements

**Services that must be enabled:**

- `dns`
- `hostpath-storage`

(microk8s-cloud)=
## The cloud

```{ibnote}
See also: {ref}`cloud`, {ref}`Juju | Manage clouds <manage-clouds>`, {ref}`Terraform Provider for Juju | Manage clouds <tfjuju:manage-clouds>`
```

### Adding the cloud

For a localhost MicroK8s cloud, if you would like to be able to skip `juju add-k8s`, install MicroK8s from the strictly confined snap.

```{ibnote}
See more: [MicroK8s | Strict MicroK8s](https://microk8s.io/docs/install-strict)
```

(microk8s-appendix-example-workflows)=
## Appendix: Example workflows

(microk8s-appendix-quickstart)=
### Add cloud, add credential, bootstrap

1. Install MicroK8s and enable required add-ons (`dns`, `hostpath-storage`).
2. Add the Kubernetes cloud with `juju add-k8s microk8s`.
3. Select the `kubeconfig` context when prompted; Juju imports both cloud and credential data from that context.
4. Bootstrap with `juju bootstrap microk8s microk8s-controller`.
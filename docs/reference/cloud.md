---
myst:
  html_meta:
    description: "Juju cloud substrate reference: AWS, Azure, GCP, Kubernetes, OpenStack, MAAS, LXD, and other supported cloud platforms."
---

(cloud)=
# Cloud (substrate)
```{ibnote}
See also: {ref}`manage-clouds`
```

```{toctree}
:hidden:

cloud/list-of-supported-clouds/index

```

To Juju, a **cloud** (or backing cloud) is any entity that has an API that can provide compute, networking, and optionally storage resources in order for application units to be deployed on them. This includes public clouds such as Amazon Web Services, Google Compute Engine, Microsoft Azure and Kubernetes as well as private OpenStack-based clouds. Juju can also make use of environments which are not clouds per se, but which Juju can nonetheless treat as a cloud. MAAS and LXD fit into this last category. Because of this, in Juju a cloud is sometimes also called, more generally, a **substrate**.

## Supported clouds

See {ref}`list-of-supported-clouds`.

(cloud-taxonomy)=
## Cloud taxonomy

(machine-cloud)=
### Machine cloud

A **machine cloud** is a cloud based on bare metal machines (e.g., MAAS), virtual machines (e.g., Amazon EC2, Google GCE, Microsoft Azure), or system containers (e.g., LXD).

When you deploy to a machine cloud, Juju provisions or adopts infrastructure resources (machines, networks, storage) and deploys machine charms onto those resources.

```{ibnote}
See more: {ref}`machine-clouds`, {ref}`list-of-supported-clouds` -- Amazon EC2, Google GCE, Microsoft Azure, OpenStack, Oracle OCI, VMware vSphere, MAAS, LXD, Manual, Equinix Metal
```

(kubernetes-cloud)=
### Kubernetes cloud

A **Kubernetes cloud** is a cloud based on an existing Kubernetes cluster (e.g., Amazon EKS, Google GKE, Microsoft AKS, MicroK8s, Canonical Kubernetes).

When you deploy to a Kubernetes cloud, Juju does not provision the cluster infrastructure itself. Instead, it manages application workloads within the cluster by deploying Kubernetes charms as pods, services, and other Kubernetes resources.

```{ibnote}
See more: {ref}`kubernetes-clouds`, {ref}`list-of-supported-clouds` -- Amazon EKS, Google GKE, Microsoft AKS, MicroK8s, Canonical Kubernetes
```

(cloud-definition)=
## Cloud definition

In Juju, cloud definitions can be provided either interactively or via a YAML file. When provided via file, they are saved in:
- `public-clouds.yaml` for public clouds (on Linux, typically: `~/.local/share/juju/public-clouds.yaml`)
- `clouds.yaml` for user-defined clouds (on Linux: `~/.local/share/juju/clouds.yaml`)

For YAML file templates and schema details, see:
- {ref}`machine-clouds` > Cloud > Cloud definition file
- {ref}`kubernetes-clouds` > Cloud > Cloud definition file

```{ibnote}
See more: {ref}`manage-clouds`
```

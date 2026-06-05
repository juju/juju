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

cloud/kubernetes-cloud/index
cloud/kubernetes-cloud/list-of-supported-kubernetes-clouds/index
cloud/machine-cloud/index
cloud/machine-cloud/list-of-supported-machine-clouds/index
```

To Juju, a **cloud** (or backing cloud) is any entity that has an API that can provide compute, networking, and optionally storage resources in order for application units to be deployed on them. This includes public clouds such as Amazon Web Services, Google Compute Engine, Microsoft Azure and Kubernetes as well as private OpenStack-based clouds. Juju can also make use of environments which are not clouds per se, but which Juju can nonetheless treat as a cloud. MAAS and LXD fit into this last category. Because of this, in Juju a cloud is sometimes also called, more generally, a **substrate**.

(cloud-taxonomy)=
## Cloud types

Juju supports two fundamental cloud types:

- **{ref}`Machine cloud <machine-cloud>`**: Clouds where Juju provisions and manages machines (bare metal, VMs, or containers). See {ref}`list-of-supported-machine-clouds` for supported platforms.

- **{ref}`Kubernetes cloud <kubernetes-cloud>`**: Clouds where Juju deploys applications into an existing Kubernetes cluster. See {ref}`list-of-supported-kubernetes-clouds` for supported distributions.

(cloud-definition)=
## Cloud definition

In Juju, cloud definitions can be provided either interactively or via a YAML file. When provided via file, they are saved in:
- `public-clouds.yaml` for public clouds (on Linux, typically: `~/.local/share/juju/public-clouds.yaml`)
- `clouds.yaml` for user-defined clouds (on Linux: `~/.local/share/juju/clouds.yaml`)

For YAML file templates and schema details, see:
- {ref}`machine-cloud` > Cloud > Cloud definition file
- {ref}`kubernetes-cloud` > Cloud > Cloud definition file

```{ibnote}
See more: {ref}`manage-clouds`
```

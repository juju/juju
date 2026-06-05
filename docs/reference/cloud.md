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
cloud/machine-cloud/index
```

To Juju, a **cloud** (or backing cloud) is any entity that has an API that can provide compute, networking, and optionally storage resources in order for application units to be deployed on them. This includes public clouds such as Amazon Web Services, Google Compute Engine, Microsoft Azure and Kubernetes as well as private OpenStack-based clouds. Juju can also make use of environments which are not clouds per se, but which Juju can nonetheless treat as a cloud. MAAS and LXD fit into this last category. Because of this, in Juju a cloud is sometimes also called, more generally, a **substrate**.

(cloud-types)=
## Supported clouds

Juju supports Kubernetes and machine clouds. Each cloud type has distinct characteristics and supported providers.

Kubernetes clouds deploy applications into existing Kubernetes clusters. You can choose from many Kubernetes distributions.

```{ibnote}
See more: {ref}`kubernetes-cloud`, {ref}`list-of-supported-kubernetes-clouds`
```

Machine clouds provision and manage machines on various platforms. You can choose from bare metal, virtual machine, or container-based clouds.

```{ibnote}
See more: {ref}`machine-cloud`, {ref}`list-of-supported-machine-clouds`
```

(cloud-definition)=
## Cloud definition

In Juju, cloud definitions can be provided either interactively or via a YAML file. When provided via file, they are saved in:
- `public-clouds.yaml` for public clouds (on Linux, typically: `~/.local/share/juju/public-clouds.yaml`)
- `clouds.yaml` for user-defined clouds (on Linux: `~/.local/share/juju/clouds.yaml`)

For YAML file templates and schema details, see:
- {ref}`machine-cloud` > Clouds > Cloud definition file
- {ref}`kubernetes-cloud` > Clouds > Cloud definition file

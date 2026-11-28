---
myst:
  html_meta:
    description: "Juju cloud substrate reference: AWS, Azure, GCP, Kubernetes, OpenStack, MAAS, LXD, and other supported cloud platforms."
---

(cloud)=
# Cloud
```{ibnote}
See also: {ref}`manage-clouds`
```

```{toctree}
:hidden:

cloud/list-of-supported-clouds/index
```

To Juju, a **cloud** (or backing cloud) is any entity that has an API that can provide compute, networking, and optionally storage resources in order for application units to be deployed on them. This includes public clouds such as Amazon Web Services, Google Compute Engine, Microsoft Azure and Kubernetes as well as private OpenStack-based clouds. Juju can also make use of environments which are not clouds per se, but which Juju can nonetheless treat as a cloud. MAAS and LXD fit into this last category. Because of this, in Juju a cloud is sometimes also called, more generally, a **substrate**.

(cloud-access-levels)=
## Cloud access levels

On a cloud registered with a controller, {ref}`cloud add-model access <user-access-cloud-add-model>` permits creating a model there; {ref}`cloud admin <user-access-cloud-admin>` also permits administering that cloud. The cloud owner receives cloud admin at creation. A cloud grant does not include provider credentials or give access to existing models.

(changing-cloud-access)=
### Changing cloud access

```{ibnote}
See also: {ref}`Juju CLI | Change a user's cloud access <change-a-users-cloud-access>`
```

Changing the grant updates the user's direct access to this cloud.

**Who can do this:** A {ref}`cloud admin <user-access-cloud-admin>` on this cloud or a {ref}`controller superuser <user-access-controller-superuser>`.

**Errors:** Changing cloud access can fail with this error:

- *Error message:* `cannot change your own cloud access`. *Trigger:* The caller is changing their own grant. *What to do:* Ask a different authorized administrator to change it.

(cloud-types)=
## Types of clouds

Juju supports two types of cloud: machine clouds and Kubernetes clouds.

(machine-cloud)=
### Machine cloud

A **machine cloud** is a cloud that provides machine-level infrastructure. Juju uses the cloud API to provision or allocate machines (bare metal, virtual machines, or system containers), plus the networking and storage resources those machines require.

```{ibnote}
See more: {ref}`List of supported machine clouds <list-of-supported-machine-clouds>`
```

(kubernetes-cloud)=
### Kubernetes cloud

A **Kubernetes cloud** is a cloud backed by an existing Kubernetes cluster. Juju uses the Kubernetes API to deploy and manage applications in that cluster, rather than provisioning machine-level infrastructure directly.

```{ibnote}
See more: {ref}`List of supported Kubernetes clouds <list-of-supported-kubernetes-clouds>`
```

## Cloud definition

The structure of a cloud definition and its supported authentication types and configuration keys depend on the specific cloud. See the relevant {ref}`cloud reference page <list-of-supported-clouds>` for details.


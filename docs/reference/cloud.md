---
myst:
  html_meta:
    description: "Juju cloud substrate reference: AWS, Azure, GCP, Kubernetes, OpenStack, MAAS, LXD, and other supported cloud platforms. Cloud declaration, persistence (the record, types), execution (watchers), and rules."
---

(cloud)=
# Cloud

```{toctree}
:hidden:

cloud/list-of-supported-clouds/index
```

To Juju, a **cloud** (or backing cloud) is any entity that has an API that can provide compute, networking, and optionally storage resources in order for application units to be deployed on them. This includes public clouds such as Amazon Web Services, Google Compute Engine, Microsoft Azure and Kubernetes as well as private OpenStack-based clouds. Juju can also make use of environments which are not clouds per se, but which Juju can nonetheless treat as a cloud. MAAS and LXD fit into this last category. Because of this, in Juju a cloud is sometimes also called, more generally, a **substrate**.

A cloud's neighbours: a {ref}`credential <credential>` authenticates against it, a {ref}`model <model>` records which cloud it deploys into, and the {ref}`machines <machine>` and {ref}`applications <application>` that run on it draw their resources from it; its {ref}`regions <list-of-supported-clouds>` are the sub-scopes a model lands in.

## The cloud in the declaration layer

How clients add a cloud and manage its definition.

- **Adding, updating, removing:** You add, update, or remove a cloud through a Juju client; adding a cloud requires controller {ref}`superuser access <user-access-controller-superuser>`.
- **Regions:** A cloud is divided into regions, the sub-scopes a model lands in (see {ref}`the list of supported clouds <list-of-supported-clouds>`).

```{ibnote}
See also: {ref}`Juju | Manage clouds <manage-clouds>`, {ref}`Terraform Provider for Juju | Manage clouds <tfjuju:manage-clouds>`
```

(the-cloud-declaration-rules)=
### Declaration rules and errors

- **Rules:**
  - The cloud type must be one of Juju's known types, and the definition must carry at least one admitted authentication type.
  - A cloud with models cannot be removed; the bootstrapped cloud's controller model references it, so it cannot be removed while its controller stands.
- **Errors:**
  - **`cloud already exists`:** Triggered when adding a cloud whose name is taken. Remediation: choose an unused name.
  - **`cloud still in use`:** Triggered when deleting a cloud that one or more models still reference. Remediation: move the models to another cloud or remove them before deleting.

(the-clouds-persistence)=
## The cloud in the persistence layer

In the controller database, a cloud is a definition record set (`0005-cloud.sql`):

- **`cloud`**: The name (unique), the cloud type, the endpoints Juju talks to (the cloud's API, identity, and storage endpoints, and whether to skip TLS verification), and the CA certificate for TLS.
- **`cloud_region`**: The regions with their per-region defaults.
- **`cloud_auth_type`**: The admitted authentication types, drawn from the seeded `auth_type` lookup.
- **`cloud_defaults`**: The cloud's default configuration values.
- **The pointers that name it:** {ref}`Models <model>` carry the denormalised copy of the cloud they deploy into (cloud name, type, and region on the model's record), and each model's {ref}`credential <credential>` names its cloud half of the pair. The cloud a {ref}`controller <controller>` was bootstrapped on is seeded as a record at bootstrap.

Writers: the cloud service in the controller performs the writes; adding a cloud inserts the record with its endpoints, regions, and authentication types; updating rewrites them; removing deletes it.

(the-cloud-persistence-rules)=
### Persistence rules and errors

- **Rules:**
  - A cloud has no state machine: it is a definition record, added, updated, or removed; its reachability is not stored either, whether the cloud is actually reachable is discovered per operation.
- **Errors:**
  - **`cloud not found`:** Triggered when querying a cloud that does not exist. Remediation: check the cloud name.

(the-types-of-cloud)=
### Types of cloud

Juju supports two types of cloud: machine clouds and Kubernetes clouds. The cloud **type** is the record's stored discriminator, and it is what Juju reads to decide which machinery a {ref}`model <model>` on the cloud runs: a Kubernetes cloud makes a CAAS model; every other type makes an IAAS one (see {ref}`IAAS and CAAS models <iaas-caas-models>`). The seeded type list is: `ec2`, `gce`, `azure`, `openstack`, `vsphere`, `oci`, `maas`, `lxd`, `unmanaged`, `kubernetes`.

(machine-cloud)=
#### Machine cloud

A **machine cloud** is a cloud that provides machine-level infrastructure. Juju uses the cloud API to provision or allocate machines (bare metal, virtual machines, or system containers), plus the networking and storage resources those machines require.

```{ibnote}
See more: {ref}`List of supported machine clouds <list-of-supported-machine-clouds>`
```

(kubernetes-cloud)=
#### Kubernetes cloud

A **Kubernetes cloud** is a cloud backed by an existing Kubernetes cluster. Juju uses the Kubernetes API to deploy and manage applications in that cluster, rather than provisioning machine-level infrastructure directly.

```{ibnote}
See more: {ref}`List of supported Kubernetes clouds <list-of-supported-kubernetes-clouds>`
```

## The cloud in the execution layer

By the time the setting call returns, the cloud's record exists, and Juju has not yet spoken to the cloud itself. A cloud has no machinery of its own: the controller reads the definition whenever it talks to the provider on a model's behalf (provisioning machines, resolving details), and the models using the cloud discover its reachability per operation.

(the-cloud-watchers)=
### Cloud watchers

One watch surface: **a single cloud's changes**, whoever resolves cloud details on demand (the model creation machinery, the credential services) watches the cloud it cares about and re-fetches on change.

Every watcher fires once immediately when it is created, the initial query being the baseline snapshot, and again on each qualifying change. See {ref}`the watcher pattern <watchers>`.
---
myst:
  html_meta:
    description: "Juju cloud substrate reference: AWS, Azure, GCP, Kubernetes, OpenStack, MAAS, LXD, and other supported cloud platforms. Cloud declaration, persistence (the record, states, types), execution (watchers), and rules."
---

(cloud)=
# Cloud
```{audience} user
```

```{toctree}
:hidden:

cloud/list-of-supported-clouds/index
```

To Juju, a **cloud** (or backing cloud) is any entity that has an API that can provide compute, networking, and optionally storage resources in order for application units to be deployed on them. This includes public clouds such as Amazon Web Services, Google Compute Engine, Microsoft Azure and Kubernetes as well as private OpenStack-based clouds. Juju can also make use of environments which are not clouds per se, but which Juju can nonetheless treat as a cloud. MAAS and LXD fit into this last category. Because of this, in Juju a cloud is sometimes also called, more generally, a **substrate**.

A cloud's neighbours: a {ref}`credential <credential>` authenticates against it, a {ref}`model <model>` records which cloud it deploys into, and the {ref}`machines <machine>` and {ref}`applications <application>` that run on it draw their resources from it; its {ref}`regions <list-of-supported-clouds>` are the sub-scopes a model lands in.

(the-clouds-declaration)=
## Clouds in the declaration layer

You add, update, or remove a cloud through a Juju client; adding a
cloud requires controller superuser access.

```{ibnote}
See also: {ref}`Juju | Manage clouds <manage-clouds>`, {ref}`Terraform Provider for Juju | Manage clouds <tfjuju:manage-clouds>`
```

(the-clouds-persistence)=
(the-cloud-record)=
## Clouds in the persistence layer

In the **controller** database, a cloud is a record: its name (unique),
its cloud type, the endpoints Juju talks to (the cloud's API, identity
and storage endpoints, and whether to skip TLS verification), its
{ref}`regions <list-of-supported-clouds>` with their per-region
defaults, the authentication types it admits, its default configuration
values, and its CA certificate for TLS. The cloud a
{ref}`controller <controller>` was bootstrapped on is seeded as a
record at bootstrap.

The records live alongside their pointers: {ref}`models <model>`
carry the denormalised copy of the cloud they deploy into -- cloud
name, type and region on the model row -- and each model's
{ref}`credential <credential>` names its cloud half of the pair. The
cloud definition's exact shape -- which attributes and auth types it
supports -- is per cloud type: see the relevant
{ref}`cloud reference page <list-of-supported-clouds>` for details.

(the-cloud-states)=
### Cloud states

A cloud has no state machine: it is a definition record -- added,
updated, or removed. Its reachability is not stored either: the
record keeps the definition, and whether the cloud is actually
reachable is discovered per operation.

(types-of-cloud)=
### Types of cloud

Juju supports two types of cloud: machine clouds and Kubernetes clouds. The cloud **type** is the record's stored discriminator, and it is what Juju reads to decide which machinery a {ref}`model <model>` on the cloud runs: a Kubernetes cloud makes a CAAS model; every other type makes an IAAS one (see
{ref}`IAAS and CAAS models <iaas-caas-models>`). The seeded type list
is: `ec2`, `gce`, `azure`, `openstack`, `vsphere`, `oci`, `maas`,
`lxd`, `unmanaged`, `kubernetes`.

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

(the-clouds-execution)=
## Clouds in the execution layer

By the time the command returns, the cloud's record exists -- and Juju
has not yet spoken to the cloud itself. A cloud has no machinery of
its own: the controller reads the definition whenever it talks to the
provider on a model's behalf -- provisioning machines, resolving
details -- and the models using the cloud discover its reachability
per operation.

(the-cloud-watchers)=
### Cloud watchers
```{audience} juju-dev
```

One watch surface: **a single cloud's changes** -- whoever resolves
cloud details on demand (the model creation machinery, the
credential services) watches the cloud it cares about and
re-fetches on change.

Every watcher fires once immediately when it is created -- the initial
query is the baseline snapshot -- and again on each qualifying change
(see {ref}`the watcher pattern <watchers>`).

(the-cloud-rules-and-errors)=
## Cloud rules and errors

- the cloud name is unique;
- the cloud type must be one of Juju's known types;
- a cloud with models cannot be removed, and the bootstrapped cloud
  cannot be removed while its controller stands;
- the cloud definition must carry at least one admitted
  authentication type.

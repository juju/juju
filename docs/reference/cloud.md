---
myst:
  html_meta:
    description: "Juju cloud substrate reference: AWS, Azure, GCP, Kubernetes, OpenStack, MAAS, LXD, and other supported cloud platforms. The cloud record, cloud types, operations, and watchers."
---

(cloud)=
# Cloud
```{audience} user
```

```{ibnote}
See also: {ref}`manage-clouds`
```

```{toctree}
:hidden:

cloud/list-of-supported-clouds/index
```

To Juju, a **cloud** (or backing cloud) is any entity that has an API that can provide compute, networking, and optionally storage resources in order for application units to be deployed on them. This includes public clouds such as Amazon Web Services, Google Compute Engine, Microsoft Azure and Kubernetes as well as private OpenStack-based clouds. Juju can also make use of environments which are not clouds per se, but which Juju can nonetheless treat as a cloud. MAAS and LXD fit into this last category. Because of this, in Juju a cloud is sometimes also called, more generally, a **substrate**.

(the-clouds-records)=
## The cloud's records

(the-cloud-record)=
### The cloud's identity

In the **controller** database, a cloud is a record: its name (unique),
its cloud type, the endpoints Juju talks to (the cloud's API, identity
and storage endpoints, and whether to skip TLS verification), its
{ref}`regions <list-of-supported-clouds>` with their per-region
defaults, the authentication types it admits, its default configuration
values, and its CA certificate for TLS. The cloud a
{ref}`controller <controller>` was bootstrapped on is seeded as a
record at bootstrap; a user with controller superuser access can add
further clouds.

(the-cloud-in-the-data-model)=
### The cloud in the data model

The cloud's records live in the controller database: the cloud row,
its regions and per-region defaults, its admitted authentication
types, its default configuration, and its CA certificate. The
{ref}`models <model>` carry the denormalised copy of the cloud they
deploy into -- cloud name, type and region on the model row -- and
each model's {ref}`credential <credential>` names its cloud half of
the pair. The cloud definition's exact shape -- which attributes and
auth types it supports -- is per cloud type: see the relevant
{ref}`cloud reference page <list-of-supported-clouds>` for details.

(the-cloud-states)=
### Cloud states

A cloud has no state machine: it is a definition record -- added,
updated, or removed. Its reachability is discovered per operation, not
tracked as state.

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

(the-clouds-machinery)=
## The cloud's machinery

A cloud has no machinery of its own: it is a stored definition the
controller reads when it talks to the provider; the one watch surface
(cloud changes) reports the stored set.

(the-cloud-operations)=
### Cloud operations

Adding a cloud requires controller superuser access; updating rewrites
the definition; removing a cloud refuses while
{ref}`models <model>` still deploy into it; listing is the
`juju clouds` view over the controller's records. The bootstrapped
cloud cannot be removed while its controller stands.

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
- a cloud with models cannot be removed;
- the cloud definition must carry at least one admitted
  authentication type.

(the-cloud-definition)=
## Cloud definition

The structure of a cloud definition and its supported authentication types and configuration keys depend on the specific cloud. See the relevant {ref}`cloud reference page <list-of-supported-clouds>` for details.

(related-entities-cloud)=
## Entities related to the cloud

- **Credentials** authenticate against clouds -- one half of the
  model's cloud/credential pair (see {ref}`credential <credential>`).
- **Models** record the cloud they deploy into, and their type
  follows the cloud's (see {ref}`model <model>`,
  {ref}`IAAS and CAAS models <iaas-caas-models>`).
- **Regions** are the cloud's sub-scopes a model lands in
  (see {ref}`list of supported clouds <list-of-supported-clouds>`).
- Clouds provide **machines and pods** (see
  {ref}`machine <machine>`, {ref}`application <application>`).

---
myst:
  html_meta:
    description: "Complete Juju charm reference: the charm record, charm kinds, the charm in the data model, charm states, operations, watchers, and rules. Charmhub integration."
---

(charm)=
# Charm
```{audience} user
```

```{ibnote}
See also: {ref}`manage-charms`
```

```{toctree}
:hidden:
charm/charm-maturity
```

In Juju, a **charm** is an operator -- software that wraps an {ref}`application <application>` and that contains all of the instructions necessary for deploying, configuring, scaling, integrating, etc., the application on any {ref}`Juju-supported cloud <list-of-supported-clouds>`.

Charms are often published on [Charmhub](https://charmhub.io/).

(the-charms-records)=
## The charm's records

(the-charm-record)=
### The charm's identity

In the model database, a charm is one record per revision: each
revision the model knows is a separate charm row, identified by its
source (a local upload, the Charmhub store, or a cross-model
import), its reference name, and its revision number. The row
carries the charm's archive (a pointer into the controller's object
store), its metadata (the name, description and subordinate-ness
from `metadata.yaml`), and an **available** flag that says whether
the archive has actually arrived (see {ref}`Charm states
<the-charm-states>`).

(the-charm-origins)=
```{ggarch}
:file: ../juju.ggarch
:view: Charm origins
:no-legend:
:caption: Topology: There is no charm-revision table: each charm REVISION is its own charm row (unique on source + reference name + revision); the application's charm_uuid is a mutable pointer refreshed on update; channels (track/risk/branch) are per-application, not per-charm; download provenance and the immutable charmhub hash hang off the charm row 1:1; every deployed unit pins its own charm revision.
:alt: Application and unit records point at the charm record; charm metadata and download info hang off charm; application channel and platform records point at application.
```

(charm-revision)=
#### Charm revision

A **charm revision** is a number that uniquely identifies the version of the charm that a charm author has uploaded to Charmhub.

```{caution}
The revision increases with every new version of the charm being uploaded to Charmhub. This can lead to situations of mismatch between the semantic version of a charm and its revision number. That is, whether the changes are for a semantically newer or older version, the revision number always goes up.
```

A revision only becomes available for consumption once it's been released into a {ref}`channel <charm-channel>`. At that point, charm users will be able to see the revision at `charmhub.io/<charm/channel>` or access it via `juju info <charm>` or `juju deploy <charm> --channel`. And to inspect a specific revision of a charm, use the `--revision` flag. The syntax is `juju info <charm> --revision <revision>`.

(the-charm-in-the-data-model)=
### The charm in the data model

```{ggarch}
:file: ../juju.ggarch
:view: Charm attributes
:alt: The charm's stored tables as an entity-relationship slice: the charm row at the centre; its metadata and its download bookkeeping west; the charm-defined relations and config schema east; the actions south. Every arrow starts at the foreign-key column that stores the pointer.
:caption: Entity relationship diagram: The charm's stored records and every foreign key between them -- each arrow starts at the fk column that stores the pointer (the only directionality the storage layer has). The charm row is one record per revision; its metadata and its Charmhub download bookkeeping are 1:1 satellites; the relations (the {ref}`endpoints <application-endpoint>`), the config schema and the actions are the charm-defined payloads the application instantiates.
```

The charm row carries the identity (source, reference name, revision),
the archive pointer, and the available flag; the
`charm_metadata` record carries what `metadata.yaml` declared; the
`charm_download_info` record carries the Charmhub identifier and the
download URL and size the downloader fetches. The charm-defined
payloads are their own records: `charm_relation` (the
{ref}`endpoints <application-endpoint>`, with role and scope),
`charm_config` (the {ref}`configuration <application-configuration>`
schema: key, type, default), `charm_action` (the
{ref}`actions <action>`: key, description, parallelism), and --
not drawn above -- the charm's storage definitions, devices,
containers, terms, tags and store categories, plus the manifest of
bases the charm supports.

(the-charm-states)=
### Charm states

A charm has one state of its own: the **available** flag -- whether
the archive has arrived. A charm row is created as a *placeholder*
(reserved, not yet available) when the model resolves a revision it
does not have yet; it becomes available when the archive lands in the
object store. There is no life column on the charm: a charm is not an
active thing -- charms are deleted as bookkeeping when the last
application using them is removed (see
{ref}`Charm operations <the-charm-operations>`).

(types-of-charm)=
### Types of charm

A charm's kinds are not mutually exclusive -- a Kubernetes charm can
also be an Ops charm, a workloadless charm, and an integrator, all at
once -- so they are not a partition; each group below names one
discriminating fact about the charm.

(charm-taxonomy-by-substrate)=
#### Charm substrates

(kubernetes-charm)=
##### Kubernetes charm

A **Kubernetes charm** is a charm designed to run on a resource from a Kubernetes cloud -- i.e., in a container in a pod.

Example Kubernetes charms:

- [Discourse K8s](https://charmhub.io/discourse-k8s)
- [Zinc K8s](https://charmhub.io/zinc-k8s)
- [Postgresql K8s](https://charmhub.io/postgresql-k8s)

(machine-charm)=
##### Machine charm

A **machine charm** is a charm designed to run on a resource from a machine cloud -- i.e., a bare metal machine, a virtual machine, or a system container.

Example machine charms:
- [Ubuntu](https://charmhub.io/ubuntu)
- [Vault](https://charmhub.io/vault)
- [Rsyslog](https://charmhub.io/rsyslog)

(infrastructure-agnostic-charm)=
##### Infrastructure-agnostic charm

While charms are still very much either for {ref}`Kubernetes <kubernetes-charm>` or {ref}`machines <machine-charm>`, some {ref}`workloadless <workloadless-charm>` charms are in fact infrastructure-agnostic and can be deployed on both.

```{note}
That is because most of the difference between a machine charm and a Kubernetes charm comes from how the charm handles the workload. So, if a charm does not have a workload, and its metadata does not stipulate Kubernetes, and the charm does not do anything that would only make sense on machines / Kubernetes, it can run perfectly fine on both machines and Kubernetes -- the details of the deployment will differ (the charm will be deployed on a machine vs. a container in a pod), but the deployment will be successful. Example workloadless charms that are cloud-agnostic: [Azure Storage Integrator](https://charmhub.io/azure-storage-integrator).
```

(charm-taxonomy-by-function)=
#### Charm functions

While charms are fundamentally about codifying operations for a given workload, some have a slightly different function.

(workloadless-charm)=
##### Workloadless charm

A **workloadless charm** is a charm that does not run any workload locally.

Because of their nature, workloadless charms are often {ref}`infrastructure-agnostic <infrastructure-agnostic-charm>`.

Examples:

- [Data Integrator](https://charmhub.io/data-integrator)

(configurator-charm)=
##### Configurator charm

A **configurator charm** is a {ref}`workloadless charm <workloadless-charm>` that configures another charm, once they're integrated.

Examples:

- [Canonical Observability Stack Proxy](https://charmhub.io/cos-proxy)
- [Prometheus Scrape Config (K8s)](https://charmhub.io/prometheus-scrape-config-k8s)

(proxy-charm)=
##### Proxy charm

A **proxy charm** is a {ref}`configurator charm <configurator-charm>` where the configuration is about how to interact with a non-charmed workload.

Examples:

- [Parca Scrape Target](https://charmhub.io/parca-scrape-target)

(integrator-charm)=
##### Integrator charm

An **integrator charm** is a {ref}`proxy charm <proxy-charm>` where the non-charmed workload is some {ref}`cloud <cloud>`-related functionality.

Examples:

- [AWS-Integrator](https://charmhub.io/aws-integrator)
- [VMware vSphere Integrator](https://charmhub.io/vsphere-integrator)

(charm-taxonomy-by-role)=
#### Charm roles

(principal-charm)=
##### Principal charm

In {ref}`machine charms <machine-charm>`, a **principal charm** is any charm that has a {ref}`subordinate <subordinate-charm>`.

(subordinate-charm)=
##### Subordinate charm

In {ref}`machine charms <machine-charm>`, a **subordinate charm** is a charm designed to be deployed adjacent to another charm and to augment the functionality of that charm, known as its {ref}`principal <principal-charm>`.

When a subordinate charm is deployed, no units are created; this happens only once a relation has been established between the principal and the subordinate.

Examples:
- [Telegraf](https://charmhub.io/telegraf)
- [Canonical Livepatch](https://charmhub.io/canonical-livepatch)
- [Nrpe](https://charmhub.io/nrpe)

(charm-taxonomy-by-architecture)=
#### Charm architectures

(sidecar-charm)=
##### Sidecar charm

> This is the state-of-the-art way to develop Kubernetes charms. Both [Charmcraft](https://canonical-charmcraft.readthedocs-hosted.com/) and [Ops](https://ops.readthedocs.io/) are designed to produce *sidecar* Kubernetes charms, where the way these charms manage the workload across their respective container boundaries is through [Pebble](https://documentation.ubuntu.com/pebble/).

In {ref}`Kubernetes charms <kubernetes-charm>`, a **sidecar charm** is a {ref}`Kubernetes charm <kubernetes-charm>` designed to be placed in a container that is in the same pod as container where the workload is, following the [sidecar pattern](https://www.learncloudnative.com/blog/2020-09-30-sidecar-container). As in Juju a Kubernetes pod corresponds to a unit, that means that there is an operator inside each unit of the workload.

Examples:

- [Traefik k8s](https://charmhub.io/traefik-k8s)

(podspec-charm)=
##### Podspec charm

> No longer supported starting with Juju 4.

In {ref}`Kubernetes charms <kubernetes-charm>`, a **podspec** charm is a {ref}`Kubernetes charm <kubernetes-charm>` designed to create and manage Kubernetes resources that are used by other charms or applications running on the cloud. As this pattern was difficult to implement correctly and also sidestepped Juju's model (the resources created by a podspec charm were not under Juju's control), this pattern has been dropped in favor of {ref}`sidecar charms <sidecar-charm>`.

(charm-taxonomy-by-generation)=
#### Charm generations

Charm development has been going on for years, so naturally many attempts have been made at making the development easier. The 'raw' API Juju exposes can be interacted with directly, but most people will want to use (at least) the Bash scripts that come by default with every charm deployment, that is, {ref}`'hook commands' (or 'hook tools') <hook-command>`. If your charm only uses those, then you're writing a 'bare' charm. If you would prefer to use a higher-level, object-oriented Python library to interact with the Juju model, then you should be using [Ops](https://ops.readthedocs.io/en/latest/). There exists another Python framework that also wraps the hook tools but offers a different (less OOP, less idiomatic) interface, called `reactive`. This framework…

(ops-charm)=
##### Ops charm

> This is the state-of-the-art way to develop a charm.

An **Ops charm** is a charm developed using the [Ops](https://ops.readthedocs.io/) (operator) framework.

Examples:
- [LXD](https://charmhub.io/lxd)
- [Discourse K8s](https://charmhub.io/discourse-k8s)
- [Zinc K8s](https://charmhub.io/zinc-k8s)
- [Postgresql K8s](https://charmhub.io/postgresql-k8s)

(12-factor-app-charm)=
##### 12-Factor app charm

A **12-Factor app charm** is a charm that has been created using certain coordinated pairs of [Rockcraft](https://documentation.ubuntu.com/rockcraft/en/latest/index.html) and [Charmcraft](https://canonical-charmcraft.readthedocs-hosted.com/en/stable/) profiles designed to give you most of the content you will need to generate a [rock](https://documentation.ubuntu.com/rockcraft/en/latest/explanation/rocks/) for a charm, and then the charm itself, for a particular type of workload (e.g., an application developed with Flask).

When you initialise a rock with a 12-Factor-app-charm-geared profile, the initialisation will generate all the basic structure and content you'll need for the rock, including a [`rockcraft.yaml`](https://canonical-rockcraft.readthedocs-hosted.com/en/latest/reference/rockcraft.yaml/#) file pre-populated with an extension matching the profile. Similarly, when you initialise a charm with a 12-Factor-app-charm-geared profile, that will generate all the basic structure content you'll need for the charm, including a `charmcraft.yaml` pre-populated with an extension matching the profile as well as a `src/charm.py` pre-loaded with a library (`paas_charm`) with constructs matching the profile and the extension.

```{ibnote}
See more:

 - [Charmcraft | Write your first Kubernetes charm for a Django app](https://documentation.ubuntu.com/charmcraft/stable/tutorial/kubernetes-charm-django/)
 - [Charmcraft | Write your first Kubernetes charm for a FastAPI app](https://documentation.ubuntu.com/charmcraft/stable/tutorial/kubernetes-charm-fastapi/)
 - [Charmcraft | Write your first Kubernetes charm for a Flask app](https://documentation.ubuntu.com/charmcraft/stable/tutorial/kubernetes-charm-flask/)
 - [Charmcraft | Write your first Kubernetes charm for a Go app](https://documentation.ubuntu.com/charmcraft/stable/tutorial/kubernetes-charm-go/)
```

(reactive-charm)=
##### Reactive charm
> Superseded by {ref}`Ops <ops-charm>`.

A **Reactive charm** is a charm developed using the [Reactive](https://charmsreactive.readthedocs.io/en/latest/) framework.

Examples:
- [Prometheus2](https://charmhub.io/prometheus2) (obsolete; replaced by [Prometheus K8s](https://charmhub.io/prometheus-k8s))
- [Telegraf](https://charmhub.io/telegraf)
- [Canonical Livepatch](https://charmhub.io/canonical-livepatch) (no longer maintained)

(bare-charm)=
##### Bare charm
> Superseded by {ref}`Ops <ops-charm>`.

A **bare charm** is a charm developed without the help of a framework, with all the {ref}`hook <hook>` invocations being coded manually (which is why such charms are sometimes also called 'hooks-based' or 'hooks-only').

Examples:

- [this tiny bash charm](https://charmhub.io/tiny-bash), ideal for educational purposes
- [Mediawiki](https://charmhub.io/mediawiki)
- [Nrpe](https://charmhub.io/nrpe)

(the-charms-machinery)=
## The charm's machinery

A charm has no machinery of its own -- its units' agents execute it;
what the model runs on a charm's behalf is controller bookkeeping
(resolving, downloading, and reserving revisions), not a charm
process.

(the-charm-operations)=
### Charm operations

Operations on charms are about getting the right revision into the
model: resolving a revision from its channel, uploading a local
archive, downloading the store archive, and keeping track of new
revisions. The store side -- publishing a charm, its listings, its
promoted releases -- is Charmhub's domain, not the model's: the model
only tracks what it has resolved.

(charm-channel)=
#### Charm channels

A **charm channel** is a charm release identifier built on the pattern `<track>/<risk>/<branch>` (e.g., `juju deploy kafka --channel 3/stable`). Resolution reads it: the controller asks the store for the best revision for the channel and platform the deployment asks for, and the channel the application tracks is recorded per application, not per charm (see {ref}`the application's origin <the-application-in-the-data-model>`).

(charm-channel-track)=
##### Track

A `<track>` is a way to collect multiple supported releases of your charm under the same name.
When deploying a charm, specifying a track is optional; if none is specified, the default option is the `latest`.
To ensure consistency between tracks of the same charm, tracks must comply with a guardrail.

(charm-channel-track-guardrail)=
###### Track guardrail

A **track guardrail** is a regex generated by a Charmhub admin at the request of a charm author whose purpose is to ensure that any new track of the charm complies with the specific pattern selected by the charm author for the charm, usually in conformity with the pattern established by the upstream workload (e.g., no numbers, cf, e.g., [OpenStack](https://docs.openstack.org/charm-guide/latest/project/charm-delivery.html); numbers in the major.minor format; just integers; etc.)

##### Risk
The `<risk>` refers to one of the following risk levels:
- **stable**: (default) This is the latest, tested, working stable version of the charm.
- **candidate**: A release candidate. There is high confidence this will work fine, but there may be minor bugs.
- **beta**: A beta testing milestone release.
- **edge**: The very latest version - expect bugs!

##### Branch
Finally, the `<branch>` is an optional finer subdivision of a channel for a published charm that allows for the creation of short-lived sequences of charms (guaranteed for only 30 days without modification) that can be pushed on demand by charm authors to help with fixes or temporary experimentation. Note that, if you use `--channel` to specify a branch (e.g., during `juju deploy` or `juju refresh`), you must specify a track and a risk level as well.

#### Charm resolution

Deploying or refreshing resolves the charm before anything is written:
the controller asks the Charmhub store for the revision that matches
the requested channel and platform, reserves the charm row (a
placeholder with its metadata, config schema, actions and manifest,
and its download info) and starts the download; when the archive
arrives, the charm becomes available and the
{ref}`deployment <the-application-deployment>` proceeds. A local
upload skips the store: the archive is stored, verified against its
hash prefix, and the charm is available immediately.

#### Charm revision updates

The controller runs a revision updater that watches the store for the
charms the model's applications track and reserves new revisions as
they appear -- as placeholders, so a later {ref}`refresh
<the-application-refresh>` finds the revision already resolved.

(the-charm-watchers)=
### Charm watchers
```{audience} juju-dev
```

The charm domain exposes one watch surface: **charm changes** -- it
fires on any change to the model's charm records: a revision reserved,
a placeholder becoming available, a charm removed. Whatever needs to
react to the set of charms a model knows (for example, the machinery
behind `juju charms` and the application's charm bookkeeping)
subscribes to it.

Every watcher fires once immediately when it is created -- the initial
query is the baseline snapshot -- and again on each qualifying change
(see {ref}`the watcher pattern <watchers>`).

(the-charm-rules-and-errors)=
## Charm rules and errors
```{audience} charm-dev
```

The rules a **charm URL** must satisfy:

- the schema is `ch` (Charmhub) or `local`; a URL without schema reads
  as `ch`;
- the name is a valid reference name; the revision, when present, is a
  `-<number>` suffix; the architecture is optional (`ch:amd64/jammy/
  wordpress-30`);
- there is no registry host and no `~user` namespace in a Juju 4 charm
  URL -- the charm's identity is the (source, reference name,
  revision) triple.

The rules a **charm record** must satisfy:

- the metadata must be valid and the name a valid charm name;
- the manifest must list at least one base;
- a Charmhub charm must carry its download info;
- a sequenced revision (a revision reserved ahead of its archive) is
  created with the revision unset.

The errors that encode them:

- *Existence*: `charm not found`, `charm already exists`,
  `charm already available`, `charm not resolved`,
  `charm already resolved`, `unable to resolve charm`.
- *Validation*: `charm not valid`, `charm origin not valid`,
  `charm name not valid`, `charm source not valid`,
  `charm revision not valid`, `charm metadata not valid`,
  `charm manifest not valid`, `charm base name not supported`.
- *Downloads*: `charm hash mismatch`, `charm download info not
  found`, `charm download URL not valid`, `charm sha256 prefix
  mismatch`, `charm already exists with different size`.
- *From the store*: `channel not found`, `resource not found`,
  `rate limit exceeded`, `revision conflict`.

(related-entities-charm)=
## Entities related to the charm

- **Applications** are what a charm runs: the application references
  one charm revision by UUID, and its units pin their own
  (see {ref}`application <application>`, {ref}`unit <unit>`).
- **Charmhub** is the store charms come from: the controller resolves
  revisions and downloads archives from it; the model keeps the
  per-model copy (see [Charmhub](https://charmhub.io/)).
- **Endpoints, configuration, actions, storage and containers** are
  the payloads the charm defines and the application instantiates
  (see {ref}`application endpoint <application-endpoint>`,
  {ref}`application configuration <application-configuration>`,
  {ref}`action <action>`, {ref}`charm resource <charm-resource>`,
  {ref}`storage <storage>`).
- **Removal** deletes the model's charm records when the last
  application using them goes (see
  {ref}`application removal <the-application-removal>`).

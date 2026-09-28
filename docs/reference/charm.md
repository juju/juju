---
myst:
  html_meta:
    description: "Juju charm reference: the charm in the declaration layer (charm URLs, channels, revisions), the charm record and its satellites in the persistence layer, the execution-layer bookkeeping (downloads, revision updates, watchers), and the interpretive taxonomy of charm kinds."
---

(charm)=
# Charm

```{toctree}
:hidden:
charm/charm-maturity
```

In Juju, a **charm** is an operator: software that wraps an {ref}`application <application>` and that contains all of the instructions necessary for deploying, configuring, scaling, integrating, etc., the application on any {ref}`Juju-supported cloud <list-of-supported-clouds>`.

Charms are often published on [Charmhub](https://charmhub.io/). Applications run the charm: each application references one charm revision, and its {ref}`units <unit>` pin their own.

(the-charms-declaration)=
## Charms in the declaration layer

A charm enters the model through two acts, both model-write operations:
resolving it from Charmhub by reference (a deploy or a refresh), or
uploading a local archive; both require {ref}`model write access
<user-access-model-write>`.

```{ibnote}
See also: {ref}`Juju | Manage charms <manage-charms>`, {ref}`Terraform Provider for Juju | Manage charms <tfjuju:manage-charms>`
```

(charm-channel)=
### Charm channels

A **charm channel** identifies a charm release stream, written
`<track>/<risk>/<branch>` (for example, `3/stable`). Channels are a
[Charmhub](https://charmhub.io/) concept: tracks, risk levels
(`stable` is the default), branches and their guardrails are defined
and managed on the store side, and their detail lives in the
Charmhub/charmcraft documentation. Juju touches a channel in exactly
two places: a deploy and a refresh take a channel as input, and the
channel an application tracks is recorded per application, not per
charm (see {ref}`the application's origin
<the-application-in-the-data-model>`).

(charm-revision)=
### Charm revision

A **charm revision** is a number that uniquely identifies the version of the charm that a charm author has uploaded to Charmhub.

```{caution}
The revision increases with every new version of the charm being uploaded to Charmhub. This can lead to situations of mismatch between the semantic version of a charm and its revision number. That is, whether the changes are for a semantically newer or older version, the revision number always goes up.
```

A revision only becomes available for consumption once it's been released into a {ref}`channel <charm-channel>`. Charm users see the released revision on the store's charm page, at `charmhub.io/<charm>/<channel>`; to deploy a specific revision, address the charm by its revision: the `-<number>` suffix on the charm URL's name (see the URL rules below).

(the-charm-declaration-rules)=
### Declaration rules and errors

- **The URL rules:**
  - The schema is `ch` (Charmhub) or `local`; a URL without schema
    reads as `ch`.
  - The name matches the reference-name grammar; the revision, when
    present, is a `-<number>` suffix; the architecture, when present,
    must be a supported architecture.
  - No `~user` namespace, no query, fragment or user-info part.
- **Errors:**
  - **`charm not found`:** Triggered when a client addresses a charm
    the model does not have, or a reference the store cannot
    resolve. Remediation: check the charm reference and revision.
  - **`charm origin not valid`:** Triggered when a Charmhub origin
    carries no architecture. Remediation: name the platform.
  - **`charm source not valid`:** Triggered when a charm's source is
    none of the stored kinds (local, Charmhub, cross-model).
    Remediation: check the charm's origin.
  - **From the store:** The Charmhub API's own errors pass through:
    a channel or a resource not found, a rate limit exceeded, a
    revision conflict. Remediation: read the store's message; these
    are not model errors.

(the-charms-persistence)=
(the-charm-origins)=
(the-charm-states)=
## Charms in the persistence layer

```{ggarch}
:file: ../juju.ggarch
:view: Charm origins
:no-legend:
:caption: Entity relationship diagram: There is no charm-revision table: each charm REVISION is its own charm row (unique on source + reference name + revision); the application's charm_uuid is a mutable pointer refreshed on update; channels (track/risk/branch) are per-application, not per-charm; download provenance and the immutable charmhub hash hang off the charm row 1:1; every deployed unit pins its own charm revision.
:alt: Application and unit records point at the charm record; charm metadata and download info hang off charm; application channel and platform records point at application.
```

In the model database, a charm is one record per revision: each
revision the model knows is a separate charm row (the DDL:
`0015-charm.sql`). The row carries the charm's archive (a pointer
into the controller's object store), its metadata (the name,
description and subordinate-ness from `metadata.yaml`), and an
**available** flag. The charm service in the controller performs the
writes: resolving a revision from its channel and platform reserves
the charm row as a *placeholder* (the metadata, the config schema,
the actions and the manifest, plus its download info) and starts
the store download. A local upload skips the store: the archive is
stored and verified against its hash prefix. The store side:
publishing a charm, its listings, its promoted releases, is
Charmhub's domain, not the model's: the model only tracks what it has
resolved.

```{ggarch}
:file: ../juju.ggarch
:view: Charm attributes
:alt: The charm's stored tables as an entity-relationship slice: the charm row at the centre with its uuid, source, reference name, revision and available flag; its metadata and its download bookkeeping west; the charm-defined relations and config schema east; the actions south. Each line is a stored pointer; 1/m at each end; nothing dashed -- every pointer here is mandatory.
:caption: Entity relationship diagram: The charm's stored records and the schema associations between them -- each line starts at the fk column that holds the pointer (the only directionality the storage layer has; the DDL and the fk: badges own it -- the drawing states the association, 1/m at each end, dashed = the row may be absent). The charm row is one record per revision; its metadata and its Charmhub download bookkeeping are 1:1 satellites; the relations (the {ref}`endpoints <application-endpoint>`), the config schema and the actions are the charm-defined payloads the application instantiates.
```

The identity pair: the primary key (`charm.uuid`) is the join handle,
so the metadata, the download bookkeeping and the charm-defined
payloads have something to point at, and so the
{ref}`application's <application>` and {ref}`unit's <unit>` charm
pointers have a row to name. The natural key is the revision triple:
UNIQUE on `source_id`, `reference_name`, `revision`: a local
upload, the Charmhub store, or a cross-model import, plus the charm's
transient name and the revision number. There is no charm-revision
table: each revision is its own row.

Every foreign key is an assertion the record holds: the charm_metadata
row holds the charm pointer (its primary key is the pointer, one
row per charm); the charm_download_info row likewise; the
charm_relation, charm_config and charm_action rows hold the charm
pointer each: the payloads one charm defines, which the
{ref}`application <application>` instantiates ({ref}`endpoints
<application-endpoint>` with role and scope; the
{ref}`configuration <application-configuration>` schema: key, type,
default; the {ref}`actions <action>`: key, description,
parallelism). Not drawn above, all assertions the schema states: the
charm's storage definitions, devices, containers, terms, tags and
store categories, plus the manifest of bases the charm supports.

The record set is written in one transaction per charm: the metadata,
the relations, the config schema, the actions, the storage, device,
container and resource definitions, the terms, tags and categories,
and the manifest bases all carry the charm's UUID as their pointer.
A charm is unmodifiable after it lands: the schema triggers refuse
updates to the charm-defined tables (`charm_action`, `charm_config`,
the containers and their mounts are the schema's own comment:
"unmodifiable, only insertions and deletions are allowed").

One projection, with no writer of its own:

- **the available flag** (the charm's one state of its own): A charm
  row is created as a *placeholder* (reserved, not yet available)
  when the model resolves a revision it does not have yet; it becomes
  available when the archive lands in the object store. The
  downloader in the execution layer delivers it; the resolve path
  writes `available = TRUE` in the same UPDATE that stamps the
  archive path. There is no life column on the charm: a charm is not
  an active thing, and nothing transitions: a flag flips once, from
  reserved to available.

(the-charm-persistence-rules)=
### Persistence rules and errors

- **Rules:**
  - One record per revision: no charm-revision table; the natural
    key is UNIQUE on source, reference name and revision.
  - A local or Charmhub charm carries an architecture; a
    cross-model charm must not.
  - The charm-defined tables take insertions and deletions, never
    updates.
  - The metadata must parse and the name must be a valid charm name;
    the manifest must list at least one base, and a base must be
    Ubuntu (the only base Juju supports); a Charmhub charm must
    carry its download info, with a valid download URL; a sequenced
    revision is created with the revision unset.
- **Errors:**
  - **`charm already exists`:** Triggered when reserving a charm
    whose (source, reference name, revision) triple the model
    already has. Remediation: none; the existing revision is
    returned.
  - **`charm name not valid`:** Triggered when the charm's metadata
    name or the reference name fails the name grammar. Remediation:
    rename the charm to the grammar above.
  - **`charm metadata not valid`:** Triggered when a charm carries
    no metadata. Remediation: check the archive's `metadata.yaml`.
  - **`charm manifest not found` / `charm manifest not valid`:**
    Triggered when a Charmhub charm carries no manifest, or an empty
    one. Remediation: publish the charm with a manifest listing at
    least one base.
  - **`charm base name not valid` / `charm base name not
    supported`:** Triggered when a manifest base has no name, or is
    not Ubuntu. Remediation: build the charm on an Ubuntu base.
  - **`charm revision not valid`:** Triggered when a sequenced charm
    is given an explicit revision. Remediation: let the sequence
    mint the revision.
  - **`charm not valid`:** Triggered when the record to store is not
    a readable charm. Remediation: check the archive.
  - **`charm download info not found` / `charm download URL not
    valid`:** Triggered when a Charmhub charm is stored without its
    download info, or the info carries no download URL.
    Remediation: re-resolve the charm from its channel.

(the-charms-execution)=
## Charms in the execution layer

By the time the command returns, the record exists, and the archive
may still be in flight: nothing has been executed, because a charm
has no machinery of its own. Its units' agents execute it. What the
model runs on a charm's behalf is controller bookkeeping, split by
owner:

- **The async downloader:** The controller watches its applications
  for pending charm downloads (applications whose charm is a
  placeholder), downloads the archive from the store URL, verifies
  the sha256 against the hash the reservation recorded, stores the
  blob in the object store, and resolves the download: the same
  UPDATE that writes the archive path flips the charm available.
  A failed attempt is retried with a delay; a resolve that fails ends
  the worker's run, and the pending-charms watcher picks the
  application up again.
- **The revision updater:** The controller watches the store for the
  charms the model's applications track and reserves new revisions
  as placeholders (the full metadata, the hash and the download
  info), so a later {ref}`refresh <the-application-refresh>` finds
  the revision already resolved.
- **Removal:** Charms are deleted as bookkeeping when the last
  application using them is removed: the removal path deletes the
  charm only if no application and no unit still references it (see
  {ref}`application removal <the-application-removal>`).

(the-charm-watchers)=
### Charm watchers

The charm domain exposes one watch surface: **charm changes**. It
fires on any change to the model's charm records (the `charm` table's
change stream): a revision reserved, a placeholder becoming
available, a charm removed. No worker subscribes to it in the current
code; the surface is the charm domain's own, and the model's charm
listing reads the records directly.

Every watcher fires once immediately when it is created, the initial
query being the baseline snapshot, and again on each qualifying change
(see {ref}`the watcher pattern <watchers>`).

(the-charm-rules-and-errors)=
### Execution rules and errors

- **Rules:**
  - Nothing runs on a charm's behalf but bookkeeping; the archive
    lands asynchronously, and the available flag flips in the same
    UPDATE that stamps the archive path.
  - The archive is checked against its reservation: the upload's
    sha256 prefix, the download's full sha256.
  - Deletion is bookkeeping: the charm row and its satellites go
    only when no application and no unit still references the
    charm.
- **Errors:**
  - **`charm already available`:** Triggered when a download or an
    upload resolves a charm that already carries its archive.
    Remediation: none; the charm is ready for deployment.
  - **`charm not resolved` / `charm already resolved`:** Triggered
    when a download resolves a charm other than the one reserved,
    or one that is already resolved. Remediation: let the
    downloader retry; do not re-resolve a resolved charm.
  - **`charm hash mismatch`:** Triggered when the archive fails its
    reservation: an uploaded archive's hash does not match the
    prefix the client sent, a downloaded archive's sha256 does not
    match the reservation's hash. Remediation: re-upload or
    re-download the charm.
  - **`charm already exists with different size`:** Triggered when
    the object store already holds a blob under the charm's hash,
    at a different size. Remediation: none; this is a store
    conflict, retry or re-resolve.
  - Two more values live in the charm's error vocabulary with no
    raiser in the current code: `unable to resolve charm` and
    `charm SHA256 prefix mismatch`. Nothing produces them; the
    resolve failures surface the underlying store or parse errors,
    and the prefix check reports `charm hash mismatch`.

(types-of-charm)=
## Types of charm

A charm's kinds are an interpretive taxonomy, not a stored
discriminator: they are not mutually exclusive (a Kubernetes charm can
also be an Ops charm, a workloadless charm, and an integrator, all at
once), so they are not a partition; each group below names one
discriminating fact about the charm.

(charm-taxonomy-by-substrate)=
### Charm substrates

(kubernetes-charm)=
#### Kubernetes charm

A **Kubernetes charm** is a charm designed to run on a resource from a Kubernetes cloud, that is, in a container in a pod.

Example Kubernetes charms:

- [Discourse K8s](https://charmhub.io/discourse-k8s)
- [Zinc K8s](https://charmhub.io/zinc-k8s)
- [Postgresql K8s](https://charmhub.io/postgresql-k8s)

(machine-charm)=
#### Machine charm

A **machine charm** is a charm designed to run on a resource from a machine cloud, that is, a bare metal machine, a virtual machine, or a system container.

Example machine charms:
- [Ubuntu](https://charmhub.io/ubuntu)
- [Vault](https://charmhub.io/vault)
- [Rsyslog](https://charmhub.io/rsyslog)

(infrastructure-agnostic-charm)=
#### Infrastructure-agnostic charm

While charms are still very much either for {ref}`Kubernetes <kubernetes-charm>` or {ref}`machines <machine-charm>`, some {ref}`workloadless <workloadless-charm>` charms are in fact infrastructure-agnostic and can be deployed on both.

```{note}
That is because most of the difference between a machine charm and a Kubernetes charm comes from how the charm handles the workload. So, if a charm does not have a workload, and its metadata does not stipulate Kubernetes, and the charm does not do anything that would only make sense on machines / Kubernetes, it can run perfectly fine on both machines and Kubernetes; the details of the deployment will differ (the charm will be deployed on a machine vs. a container in a pod), but the deployment will be successful. Example workloadless charms that are cloud-agnostic: [Azure Storage Integrator](https://charmhub.io/azure-storage-integrator).
```

(charm-taxonomy-by-function)=
### Charm functions

While charms are fundamentally about codifying operations for a given workload, some have a slightly different function.

(workloadless-charm)=
#### Workloadless charm

A **workloadless charm** is a charm that does not run any workload locally.

Because of their nature, workloadless charms are often {ref}`infrastructure-agnostic <infrastructure-agnostic-charm>`.

Examples:

- [Data Integrator](https://charmhub.io/data-integrator)

(configurator-charm)=
#### Configurator charm

A **configurator charm** is a {ref}`workloadless charm <workloadless-charm>` that configures another charm, once they're integrated.

Examples:

- [Canonical Observability Stack Proxy](https://charmhub.io/cos-proxy)
- [Prometheus Scrape Config (K8s)](https://charmhub.io/prometheus-scrape-config-k8s)

(proxy-charm)=
#### Proxy charm

A **proxy charm** is a {ref}`configurator charm <configurator-charm>` where the configuration is about how to interact with a non-charmed workload.

Examples:

- [Parca Scrape Target](https://charmhub.io/parca-scrape-target)

(integrator-charm)=
#### Integrator charm

An **integrator charm** is a {ref}`proxy charm <proxy-charm>` where the non-charmed workload is some {ref}`cloud <cloud>`-related functionality.

Examples:

- [AWS-Integrator](https://charmhub.io/aws-integrator)
- [VMware vSphere Integrator](https://charmhub.io/vsphere-integrator)

(charm-taxonomy-by-role)=
### Charm roles

(principal-charm)=
#### Principal charm

In {ref}`machine charms <machine-charm>`, a **principal charm** is any charm that has a {ref}`subordinate <subordinate-charm>`.

(subordinate-charm)=
#### Subordinate charm

In {ref}`machine charms <machine-charm>`, a **subordinate charm** is a charm designed to be deployed adjacent to another charm and to augment the functionality of that charm, known as its {ref}`principal <principal-charm>`.

When a subordinate charm is deployed, no units are created; this happens only once a relation has been established between the principal and the subordinate.

Examples:
- [Telegraf](https://charmhub.io/telegraf)
- [Canonical Livepatch](https://charmhub.io/canonical-livepatch)
- [Nrpe](https://charmhub.io/nrpe)

(charm-taxonomy-by-architecture)=
### Charm architectures

(sidecar-charm)=
#### Sidecar charm

> This is the state-of-the-art way to develop Kubernetes charms. Both [Charmcraft](https://canonical-charmcraft.readthedocs-hosted.com/) and [Ops](https://ops.readthedocs.io/) are designed to produce *sidecar* Kubernetes charms, where the way these charms manage the workload across their respective container boundaries is through [Pebble](https://documentation.ubuntu.com/pebble/).

In {ref}`Kubernetes charms <kubernetes-charm>`, a **sidecar charm** is a {ref}`Kubernetes charm <kubernetes-charm>` designed to be placed in a container that is in the same pod as container where the workload is, following the [sidecar pattern](https://www.learncloudnative.com/blog/2020-09-30-sidecar-container). As in Juju a Kubernetes pod corresponds to a unit, that means that there is an operator inside each unit of the workload.

Examples:

- [Traefik k8s](https://charmhub.io/traefik-k8s)

(podspec-charm)=
#### Podspec charm

> No longer supported starting with Juju 4.

In {ref}`Kubernetes charms <kubernetes-charm>`, a **podspec** charm is a {ref}`Kubernetes charm <kubernetes-charm>` designed to create and manage Kubernetes resources that are used by other charms or applications running on the cloud. As this pattern was difficult to implement correctly and also sidestepped Juju's model (the resources created by a podspec charm were not under Juju's control), this pattern has been dropped in favor of {ref}`sidecar charms <sidecar-charm>`.

(charm-taxonomy-by-generation)=
### Charm generations

Charm development has been going on for years, so naturally many attempts have been made at making the development easier. The 'raw' API Juju exposes can be interacted with directly, but most people will want to use (at least) the Bash scripts that come by default with every charm deployment, that is, {ref}`'hook commands' (or 'hook tools') <hook-command>`. If your charm only uses those, then you're writing a 'bare' charm. If you would prefer to use a higher-level, object-oriented Python library to interact with the Juju model, then you should be using [Ops](https://ops.readthedocs.io/en/latest/). There exists another Python framework that also wraps the hook tools but offers a different (less OOP, less idiomatic) interface, called `reactive`. This framework is deprecated and no longer maintained; it is mentioned here only for historical reasons.

(ops-charm)=
#### Ops charm

> This is the state-of-the-art way to develop a charm.

An **Ops charm** is a charm developed using the [Ops](https://ops.readthedocs.io/) (operator) framework.

Examples:
- [LXD](https://charmhub.io/lxd)
- [Discourse K8s](https://charmhub.io/discourse-k8s)
- [Zinc K8s](https://charmhub.io/zinc-k8s)
- [Postgresql K8s](https://charmhub.io/postgresql-k8s)

(12-factor-app-charm)=
#### 12-Factor app charm

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
#### Reactive charm
> Superseded by {ref}`Ops <ops-charm>`.

A **Reactive charm** is a charm developed using the [Reactive](https://charmsreactive.readthedocs.io/en/latest/) framework.

Examples:
- [Prometheus2](https://charmhub.io/prometheus2) (obsolete; replaced by [Prometheus K8s](https://charmhub.io/prometheus-k8s))
- [Telegraf](https://charmhub.io/telegraf)
- [Canonical Livepatch](https://charmhub.io/canonical-livepatch) (no longer maintained)

(bare-charm)=
#### Bare charm
> Superseded by {ref}`Ops <ops-charm>`.

A **bare charm** is a charm developed without the help of a framework, with all the {ref}`hook <hook>` invocations being coded manually (which is why such charms are sometimes also called 'hooks-based' or 'hooks-only').

Examples:

- [this tiny bash charm](https://charmhub.io/tiny-bash), ideal for educational purposes
- [Mediawiki](https://charmhub.io/mediawiki)
- [Nrpe](https://charmhub.io/nrpe)

---
myst:
  html_meta:
    description: "Charm resource reference: file and OCI image resources for charms. The resource in the declaration layer (staging at deploy, pushes, revision records), the resource's records in the persistence layer, and the execution layer's push, repository poll, revision update and unit-resource machinery."
---

(charm-resource)=
# Resource (charm)

```{ibnote}
See also: {ref}`manage-charm-resources`
```

In Juju, a **charm resource** is additional content that a
{ref}`charm <charm>` can make use of, or may require, to run.

Resources exist for content the charm needs but that does not have to
travel with the charm itself: large blobs whose update cadence differs
from the charm's. Keeping them separate lets each side move on its own
schedule, and spares routine upgrades from re-downloading large files
from Charmhub.

(the-resource-operations)=
## The resource in the declaration layer

No client creates a resource record by naming it: a resource's
definition is the {ref}`charm <charm>`'s own declaration (the name,
the type, the storage path within the charm, a description), and a
resource record is born only when content is stored or when the
repository's revision is recorded. The client's acts on resources are:

- **Staging at deploy:** deploying with resources resolves each named
  resource up front: a store resource must carry its revision, an
  upload must not (the upload's content, once stored, is the blob the
  revision would name). The records are written before the
  application exists, held by the pending-application record until
  the application is created.
- **Uploading:** a push stores the blob and writes a new resource
  record with no revision; the application's usage is repointed at
  it.
- **Recording revisions:** the repository is polled and a newer
  revision is recorded as a stored blob of its own, so an
  {ref}`application <application>` can be refreshed to it (see
  {ref}`the application refresh <the-application-refresh>`).

The store revisions come from Charmhub; a push bypasses it (see
{ref}`charm origins <the-charm-origins>`).

(the-resource-declaration-rules)=
### Declaration rules and errors

- **Rules:**
  - A resource's name must match one of the charm's definitions:
    resources cannot be invented per application, and the definition
    is keyed by the charm and the name together.
  - A store resource must carry its revision; an upload must not.
  - A revision is a non-negative number: the client-facing update
    refuses a negative revision, and the model import refuses it
    with its own vocabulary.
  - A resource has one of two types, `file` or `oci-image` (the
    schema's kind lookup). An OCI image resource can be given as a
    repository revision, as a local file, or as a reference to a
    public OCI image; the local form is a JSON or YAML document
    carrying the image reference and, optionally, a username and a
    password.

````{dropdown} Expand to view an example JSON file

```text
{
  "ImageName": "my.private.repo.com/a/b:latest",
  "username": "harry",
  "password": "supersecretpassword"
}
```

````

````{dropdown} Expand to view an example YAML file

```text
registrypath: my.private.repo.com/a/b:latest
username: harry
password: supersecretpassword

```
````

- **Errors:**
  - **`charm resource not found`:** Triggered when resources are
    staged for a charm whose definitions do not carry the name: the
    pending insert points at the charm's definition record, and the
    composite foreign key refuses the pair. Remediation: check the
    name against the charm's metadata.
  - **`resource name not valid`:** Triggered when a lookup, a staged
    set or an import names an empty resource, and when an import
    carries two resources of the same name for one application.
    Remediation: name the resource exactly as the charm's metadata
    declares it.
  - **`argument not valid`:** Triggered when the store-and-upload
    dichotomy is broken (a store resource without its revision, an
    upload with one) and when a revision update is given a negative
    number. Remediation: a store resource takes a revision, an
    upload takes content, and revisions count from zero.
  - **`resource revision not valid`:** Triggered when the model
    import's records carry a negative upload revision. Remediation:
    a revision counts from zero.
  - **`charm ID not valid`:** Triggered when a repository record
    names a charm ID that does not validate. Remediation: check the
    charm the record points at.
  - **`origin not valid`:** Triggered when an import's resource
    origin is neither `store` nor `upload`. Remediation: the origin
    is one of the two values.

(the-resource-in-the-data-model)=
## The resource in the persistence layer

In the model database a charm resource is two records plus the blob:
the charm's **definition** (the resource's name, its type, its
storage path within the charm, a description; declared by the charm's
metadata) and the **content** record, one per stored blob, carrying
the revision (for store resources), the origin (a store revision or
an upload) and the state of its store lifecycle. The blob itself
lives outside the record set, in the store its type selects.

The record set over the DDL (`0015-charm.sql`, `0023-resource.sql`,
the `0052` and `0054` patch migrations):

- **`charm_resource`:** the charm's definition: a composite primary
  key of the charm and the resource name, the type (`kind_id` into
  the `charm_resource_kind` lookup, `file` or `oci-image`), the
  nullable in-charm `path` and the description.
- **`resource`:** one row per stored blob: a `uuid` primary key, the
  composite foreign key naming the definition it fills, the nullable
  `revision` (empty for uploads), the origin (`resource_origin_type`,
  `upload` or `store`), the store-lifecycle state (`resource_state`),
  `created_at`, and `last_polled`, set only for potential rows.
- **`application_resource`:** the application's usage:
  `resource_uuid` is its primary key, so one stored blob serves at
  most one application; the blob may come from a different charm
  revision than the application runs.
- **`pending_application_resource`:** resources staged before the
  application exists: the resource's UUID and the application by
  name, with no foreign key, because the application does not exist
  yet.
- **`resource_retrieved_by`:** who put the blob there: a type
  (`user`, `unit` or `application`) and that entity's name, keyed by
  the resource's UUID.
- **`unit_resource`:** the unit's own copy: a composite primary key
  of the resource's UUID and the unit's UUID, plus the time it was
  added.
- **The store links, split by type:** `resource_file_store` points a
  file resource at its object-store blob (`store_uuid` into
  `object_store_metadata`, with the size and the sha384 digest);
  `resource_image_store` points an OCI image resource at its
  metadata record (`resource_container_image_metadata_store`: the
  registry path, the username and the password nullable).
- **The views the reads use:** `v_resource` coalesces the two store
  links into one row's size and digest; `v_application_resource`
  (reshaped by the `0052` patch to an inner join) lists the
  resources an application actually uses; `v_unit_resource` lists
  the units' own copies.
- **The adjacent satellite:** `application_k8s_resources_managed`
  (the `0054` patch) is the application side's record that the
  provisioner manages an application's Kubernetes resources; it
  blocks removal until cleared (see {ref}`application
  <application>`).

The identity pair: the definition's natural key (the charm and the
name) is what the charm's metadata declares and what a deploy names;
the blob record's `uuid` is the join handle every satellite points
at (the usage, the retrieval, the unit copy, the store link).

Every foreign key is an assertion the record holds: the resource row
holds the definition pointer; the usage row holds both of its
neighbours'; the store links hold the blob pointer and the store-side
pointer each. Nothing transitions: `resource_state` is two values,
`available` (the blob the application's units use now) and
`potential` (a newer revision the repository reports, the upgrade's
hint, stamped with `last_polled`), not a life the record works
through.

(the-resource-persistence-rules)=
### Persistence rules and errors

- **Rules:**
  - One stored blob serves at most one application: repointing an
    application's usage rewrites the single usage row, and the
    application may keep using a blob acquired for a different charm
    revision than the one it runs.
  - An upload replaces: the re-upload's record takes over the usage
    and the prior blob is removed from its store, an absent blob
    being tolerated; a failed store removes the blob it just wrote.
  - The retrieval is recorded at store time: the type says who (a
    user pushed it, a unit or the application fetched it) and the
    name says which.
  - Two error constants in the resource vocabulary have no raisers
    in the current code: `resource state not valid` and `resource
    already found in storage`.
- **Errors:**
  - **`resource not found`:** Triggered when a resource is queried
    by name for an application, by UUID, or for its name and type,
    and no row answers. Remediation: check the name against the
    charm's definitions or the listing.
  - **`stored resource not found`:** Triggered when the blob behind
    a resource row is missing from its store: the read refuses,
    while the delete paths tolerate the absence. Remediation: push
    the resource again.
  - **`stored resource already exists`:** Triggered when a store put
    finds an object already present at the resource's key.
    Remediation: check the store for the resource's UUID before
    pushing again.
  - **`cleanup state not valid`:** Triggered when an application's
    records are cleaned up out of order: the guard marks an internal
    ordering fault. Remediation: none from the client's side; the
    error reports the fault, it does not gate it.

(the-resources-machinery)=
## The resource in the execution layer

A resource has no machinery of its own: nothing runs a resource. What
runs is the storing and the repointing, split by owner:

- **The push:** the resources API stores a client's blob: the type's
  store takes the content, the record carries the retrieval (the
  user's name for a manual push), and, when the resource already
  belongs to an application, the application's charm-modified
  version is bumped so its units notice. The pending case does not
  bump: the application does not exist yet (the push path's own
  comment).
- **The repository poll:** the charm revision updater, the worker
  that reserves the charm's newer revision, records the revision's
  resources per application: rows land as `potential`, stamped with
  the poll time.
- **The revision update:** a client-facing update repoints the
  application: a store resource's revision update writes a new
  resource row and removes the prior blob, and an upload's update
  replaces the blob through the same store path.
- **The unit's copy:** a unit fetches its resource through the unit
  resources API: the blob is opened from the store, the retrieval
  recorded as the unit's, and the unit's own copy is set; an
  application-level fetch records the retrieval and sets nothing.
- **The model import:** a migrating model's resources are imported
  explicitly: each record's name (non-empty, unique per
  application), origin (one of the two values) and upload revision
  (non-negative) are validated before the insert.
- **The pending teardown:** a deployment that fails cleans up the
  resources it staged: the staged blobs are deleted and the cleanup
  errors are logged rather than reported, so they do not mask the
  deployment failure (the teardown's own comment).
- **The unused removal surfaces:** the service exposes an
  application-resource removal and a unit-resource removal (the
  application one guarded by the cleanup-ordering check); neither
  has a non-test caller in the current code. The surfaces are
  stated, not narrated: nothing invokes them.

(the-resource-watchers)=
### Resource watchers

The resource domain exposes no watch surfaces: resources are pushed
(upload, deploy staging, revision polling), not watched. A charm
learns of a new resource revision through its
{ref}`upgrade <the-application-refresh>`, not through a change
stream.

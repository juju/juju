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
(the-resource-declaration-rules)=
## The resource in the declaration layer

No client creates a resource record by naming it: a resource's
definition is the {ref}`charm <charm>`'s own declaration (the name,
the type, the storage path within the charm, a description), and a
resource record is born only when content is stored or when the
repository's revision is recorded. The client's acts on resources are:

- **Staging at deploy:** Deploying with resources resolves each named resource up front: a store resource must carry its revision, an upload must not (the upload's content, once stored, is the blob the revision would name). The records are written before the application exists, held by the pending-application record until the application is created.
  - *Rule:* A resource's name must match one of the charm's definitions: resources cannot be invented per application, and the definition is keyed by the charm and the name together.
  - *Rule:* A store resource must carry its revision; an upload must not.
  - *Related errors:*
    - **`charm resource not found`**: *Trigger:* Resources are staged for a charm whose definitions do not carry the name: the pending insert points at the charm's definition record, and the composite foreign key refuses the pair. *Remediation:* Check the name against the charm's metadata.
    - **`resource name not valid`**: *Trigger:* A lookup or a staged set names an empty resource. *Remediation:* Name the resource exactly as the charm's metadata declares it.
    - **`argument not valid`**: *Trigger:* The store-and-upload dichotomy is broken: a store resource without its revision, or an upload with one. *Remediation:* A store resource takes a revision, an upload takes content.
- **Uploading:** A push stores the blob and writes a new resource record with no revision; the application's usage is repointed at it.
- **Recording revisions:** The repository is polled and a newer revision is recorded as a stored blob of its own, so an {ref}`application <application>` can be refreshed to it (see {ref}`the application refresh <the-application-refresh>`).
  - *Rule:* A revision is a non-negative number: the client-facing update refuses a negative revision, and the model import refuses it with its own vocabulary.
  - *Related error:*
    - **`argument not valid`**: *Trigger:* A revision update is given a negative number. *Remediation:* Revisions count from zero.
- **Resource types:** A resource has one of two types, `file` or `oci-image` (the schema's kind lookup).
  - *Rule:* An OCI image resource can be given as a repository revision, as a local file, or as a reference to a public OCI image; the local form is a JSON or YAML document carrying the image reference and, optionally, a username and a password.

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

The store revisions come from Charmhub; a push bypasses it (see
{ref}`charm origins <the-charm-origins>`).

(the-resource-in-the-data-model)=
(the-resource-persistence-rules)=
## The resource in the persistence layer

A charm resource is persisted in the {ref}`model database <database>` as follows: the charm's **definition** (the
resource's name, its type, its storage path within the charm, a
description; declared by the charm's metadata) and the **content**
record, one per stored blob, carrying the revision (for store
resources), the origin (a store revision or an upload) and the state
of its store lifecycle. The blob itself lives outside the record set,
in the store its type selects.

The record set:

- **Definition record:** A composite primary key of the charm and the resource name, the type (a kind lookup, `file` or `oci-image`), the nullable in-charm path and the description. The definition's natural key (the charm and the name) is what the charm's metadata declares and what a deploy names.
- **Content record:** One per stored blob: a UUID primary key (the join handle every satellite points at: the usage, the retrieval, the unit copy, the store link), the composite foreign key naming the definition it fills, the nullable revision (empty for uploads), the origin (`upload` or `store`), the store-lifecycle state, the creation time, and the poll time, set only for potential records.
  - *Rule:* The record's state is two values, `available` (the blob the application's units use now) and `potential` (a newer revision the repository reports, the upgrade's hint, stamped with the poll time), not a life the record works through.
  - *Related errors:*
    - **`resource not found`**: *Trigger:* A resource is queried by name for an application, by UUID, or for its name and type, and no record answers. *Remediation:* Check the name against the charm's definitions or the listing.
    - **`stored resource not found`**: *Trigger:* The blob behind a resource record is missing from its store: the read refuses, while the delete paths tolerate the absence. *Remediation:* Push the resource again.
    - **`stored resource already exists`**: *Trigger:* A store put finds an object already present at the resource's key. *Remediation:* Check the store for the resource's UUID before pushing again.
- **Application usage record:** Keyed by the blob, so one stored blob serves at most one application; the blob may come from a different charm revision than the application runs.
  - *Rule:* One stored blob serves at most one application: repointing an application's usage rewrites the single usage record, and the application may keep using a blob acquired for a different charm revision than the one it runs.
- **Staged resource records:** Resources staged before the application exists, keyed by the resource's UUID and the application's name, with no foreign key, because the application does not exist yet.
- **Retrieval record:** Who put the blob there, a type (`user`, `unit` or `application`) and that entity's name, keyed by the resource's UUID. The retrieval is recorded at store time: the type says who (a user pushed it, a unit or the application fetched it) and the name says which.
- **Unit copy record:** A composite primary key of the resource's UUID and the unit's UUID, plus the time it was added.
- **Store links, split by type:** A file resource's link points at its object-store blob (keyed by the blob record, with the size and the sha384 digest); an OCI image resource's link points at its metadata record (the registry path, the username and the password nullable).
- **Reads:** The reads run over the joined set: the two store links coalesce into one record's size and digest; one read lists the resources an application actually uses; another lists the units' own copies.
- **Adjacent satellite:** One more application-side record says the provisioner manages an application's Kubernetes resources; it blocks removal until cleared (see {ref}`application <application>`).
  - *Related error:*
    - **`cleanup state not valid`**: *Trigger:* An application's records are cleaned up out of order: the guard marks an internal ordering fault. *Remediation:* None from the client's side; the error reports the fault, it does not gate it.
- **Unused error constants:** Two error constants in the resource vocabulary have no raisers in the current code: `resource state not valid` and `resource already found in storage`.

Every foreign key is an assertion the record holds: the resource record
holds the definition pointer; the usage record holds both of its
neighbours'; the store links hold the blob pointer and the store-side
pointer each.

(the-resources-machinery)=
## The resource in the execution layer

A resource has no machinery of its own: nothing runs a resource. What
runs is the storing and the repointing, split by owner:

- **The push:** The resources API stores a client's blob: the type's store takes the content, the record carries the retrieval (the user's name for a manual push), and, when the resource already belongs to an application, the application's charm-modified version is bumped so its units notice. The pending case does not bump: the application does not exist yet (the push path's own comment).
  - *Rule:* An upload replaces: the re-upload's record takes over the usage and the prior blob is removed from its store, an absent blob being tolerated; a failed store removes the blob it just wrote.
- **The repository poll:** The charm revision updater, the worker that reserves the charm's newer revision, records the revision's resources per application: records land as `potential`, stamped with the poll time.
  - *Related error:*
    - **`charm ID not valid`**: *Trigger:* A repository record names a charm ID that does not validate. *Remediation:* Check the charm the record points at.
- **The revision update:** A client-facing update repoints the application: a store resource's revision update writes a new resource record and removes the prior blob, and an upload's update replaces the blob through the same store path.
- **The unit's copy:** A unit fetches its resource through the unit resources API: the blob is opened from the store, the retrieval recorded as the unit's, and the unit's own copy is set; an application-level fetch records the retrieval and sets nothing.
- **The model import:** A migrating model's resources are imported explicitly: each record's name (non-empty, unique per application), origin (one of the two values) and upload revision (non-negative) are validated before the insert.
  - *Related errors:*
    - **`resource name not valid`**: *Trigger:* An import names an empty resource, or carries two resources of the same name for one application. *Remediation:* Name the resource exactly as the charm's metadata declares it.
    - **`resource revision not valid`**: *Trigger:* The import's records carry a negative upload revision. *Remediation:* A revision counts from zero.
    - **`origin not valid`**: *Trigger:* An import's resource origin is neither `store` nor `upload`. *Remediation:* The origin is one of the two values.
- **The pending teardown:** A deployment that fails cleans up the resources it staged: the staged blobs are deleted and the cleanup errors are logged rather than reported, so they do not mask the deployment failure (the teardown's own comment).
- **The unused removal surfaces:** The service exposes an application-resource removal and a unit-resource removal (the application one guarded by the cleanup-ordering check); neither has a non-test caller in the current code. The surfaces are stated, not narrated: nothing invokes them.

(the-resource-watchers)=
### Resource watchers

The resource domain exposes no watch surfaces: resources are pushed
(upload, deploy staging, revision polling), not watched. A charm
learns of a new resource revision through its
{ref}`upgrade <the-application-refresh>`, not through a change
stream.

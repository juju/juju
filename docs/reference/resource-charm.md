---
myst:
  html_meta:
    description: "Charm resource reference: file and OCI image resources for charms. The resource record, resource types, operations, and rules."
---

(charm-resource)=
# Resource (charm)
```{audience} user
```

```{ibnote}
See also: {ref}`manage-charm-resources`
```

In Juju, a **charm resource** is additional content that a {ref}`charm <charm>` can make use of, or may require, to run.

Resources are used where a charm author needs to include large blobs (perhaps a database, media file, or otherwise) that may not need to be updated with the same cadence as the charm or workload itself. By keeping resources separate, they can control the lifecycle of these elements more carefully, and in some situations avoid the need for repeatedly downloading large files from Charmhub during routine upgrades/maintenance.

(the-resources-records)=
## The resource's records

(the-resource-record)=
### The resource's identity

In the model database, a charm resource has two records: the charm's
**definition** -- the resource's name, its type, its storage path within
the charm, and a description, declared in the charm's metadata -- and
the **content** record: one per stored blob, carrying the revision
(for store resources), the origin (a store revision or an upload),
and the state of its store lifecycle, plus the record of which
application is *using* it. The blob itself lives in the controller's
object store.

(the-resource-in-the-data-model)=
### The resource in the data model

The stored records: the `charm_resource` rows are the charm's
definitions (keyed by charm and name, with the type -- `file` or
`oci-image` -- as a lookup). The `resource` rows are the stored
blobs: a pointer to the charm's definition, the revision (empty for
uploads), the origin, the store-lifecycle state, and the timestamps;
`application_resource` says which application *uses* which stored
blob -- it may name a resource from a different charm revision than
the application currently runs -- and a pending-application record
holds resources attached before the application exists. A
retrieved-by record names who stored the blob (a user, a unit, or the
application).

(the-resource-states)=
### Resource states

A resource has no state machine of its own: the record carries store
bookkeeping (its origin, its store-lifecycle state, who retrieved it),
not a negotiated state graph -- the state churn is upload and
revision bookkeeping, not a lifecycle.

(types-of-resource)=
### Types of resource

A resource can have one of two basic types -- `file` and `oci-image`.
These can be specified as follows:

1. If the resource is type 'file', you can specify it by providing

    a. the resource revision number or

    b.  a path to a local file.

2. If the resource is type 'oci-image', you can specify it by providing

    a. the resource revision number,

    b. a path to a local file = private OCI image,

    c. a link to a public OCI image.

If you choose to provide a path to a  local file, the file can be a JSON or a YAML file with an image reference and optionally a username and a password (i.e., an OCI image resource).


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

(the-resources-machinery)=
## The resource's machinery

A charm resource has no machinery of its own: resources are pushed --
uploaded, attached at deploy, polled for revisions -- and a charm
learns of a new revision through its {ref}`upgrade
<the-application-refresh>`, not through any machinery of the
resource's.

(the-resource-operations)=
### Resource operations

#### Attaching resources at deploy time

Deploying with resources resolves each named resource up front: a
store resource must carry its revision; an upload must not (the
upload's content becomes the revision). The blobs are stored before
the application exists -- held by the pending-application record
until it does.

#### Uploading and updating resources

Uploading a resource (for example, `juju attach-resource <app>
<resource>`) stores a new blob -- a new resource record with no
revision -- repoints the application's usage at it, removes the
previous blob from the store, and (for the charm to notice) bumps the
application's charm-modified version. Revision updates work the other
way: a repository revision is polled and recorded as a new stored
blob, and the application is repointed.

#### Unit resources

A resource can also be attached to a single unit -- the unit's own
copy, distinct from the application's.

(the-resource-watchers)=
### Resource watchers
```{audience} juju-dev
```

The resource domain exposes no watch surfaces: resources are pushed
(upload, deploy, revision polling), not watched -- a charm learns of
a new resource revision through its
{ref}`upgrade <the-application-refresh>`, not through a change
stream.

(the-resource-rules-and-errors)=
## Resource rules and errors
```{audience} charm-dev
```

- a resource's name must match one of the charm's definitions --
  resources cannot be invented per application, and an application's
  resource names are unique;
- a store resource must carry its revision; an upload must not;
- a revision must be a non-negative number (`resource revision not
  valid`).

The errors that encode them: `resource not found`,
`charm resource not found`, `resource name not valid`,
`resource revision not valid`, `stored resource not found`,
`stored resource already exists`, `origin not valid`.

(related-entities-resource)=
## Entities related to the resource

- **Charms** define the resources (see {ref}`charm <charm>`).
- **Applications** use one stored blob per resource, possibly from a
  different charm revision than they run (see
  {ref}`application <application>`).
- **Charmhub** serves the store revisions; uploads bypass it
  (see {ref}`charm origins <the-charm-origins>`).
- **Units** can carry their own attached copy (see {ref}`unit <unit>`).

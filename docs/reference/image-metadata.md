---
myst:
  html_meta:
    description: "Juju image metadata reference: the catalogue of cloud images that Juju picks a machine's image from, the record the controller database caches, and the calls that list, save, and delete it."
---

(image-metadata)=
(metadata)=
# Image metadata

```{ibnote}
See also: {ref}`Juju | Manage metadata <manage-metadata>`
```

**Image metadata** is the catalogue of the images (the compute instances a {ref}`cloud <cloud>` offers, such as one Ubuntu image per release) that Juju picks a {ref}`machine <machine>`'s image ID from when it starts the machine. Each entry names one image for one combination of a cloud, a region, an operating system version, an architecture, and the way the machine is stored and virtualised. The {ref}`controller database <database>` caches the entries it is given or has used. A client, for a user with superuser access to the controller, manages the cache. The metadata of the {ref}`agent binaries <agent-version>` is a separate catalogue, described on its own page.

(image-metadata-database-schema)=
## Image metadata database schema

```{ggarch}
:file: ../juju.ggarch
:view: Image metadata database schema
:caption: One row per cached image, keyed by the whole combination -- source, stream, region, operating system version, architecture, virtualisation type, and root storage type. The priority is the tie-breaker: when several images match a machine, the highest priority wins.
:alt: Two boxes of the same size in a row inside a group named Controller database, each with a filled table mark in the corner. Image is on the left and Architecture is on the right (m to 1). Architecture has a second, outlined arrow mark in the corner for a record that belongs to no page.
```

The image metadata database schema is the set of records that the controller database keeps about cached images. The records, with the rules that apply to each, are:

- **Image:** One cached image: its ID, the source it came from, the stream it belongs to (such as `released` or `daily`), the cloud region it is available in, the version of the operating system it holds, the virtualisation type it runs under, the type and size of its root storage, the date it was cached, and its priority.
  - *Rules:*
    - The controller caches each combination of source, stream, region, version, architecture, virtualisation type, and root storage type once; saving an entry for a combination that is cached replaces its image ID.
    - The entry with the highest priority wins when several entries match a machine.
- **Architecture:** The architecture the image is built for, such as `amd64` or `arm64`. The architectures are the shared names of the platforms Juju builds for; no page owns them.

(image-metadata-calls)=
## Image metadata calls

Image metadata calls are the calls that manage the controller's cache of image metadata. Each is made by a client, for a user with {ref}`superuser access <user-access-controller-superuser>` to the controller. The cache also fills without a client: a {ref}`bootstrap <bootstrap-a-controller>` with `--metadata-source` uploads the metadata of a local directory to the controller. When Juju starts a machine, it looks for an image in this order: the controller's cache, the URL the model's `image-metadata-url` configuration names, the locations the provider offers, and the public `http://cloud-images.ubuntu.com/releases` location, whose `daily` stream lives under `http://cloud-images.ubuntu.com/daily`. Each location is searched for signed metadata first and unsigned second.

(image-metadata-listing)=
### Listing image metadata

Listing image metadata returns the cached entries that match a filter, each with its image ID and its attributes. The entries come ordered by priority.

The caller and the rules are:

- *Caller:* A client, for a user with {ref}`superuser access <user-access-controller-superuser>` to the controller. No block applies.
- *Rules:*
  - A filter names a region, versions, architectures, a stream, a virtualisation type, and a root storage type; an empty filter matches every entry.
  - The answer is one list, ordered by priority. An entry that did not come from a `custom` source matches only for five minutes after it was cached.

(image-metadata-saving)=
### Saving image metadata

Saving image metadata stores entries in the controller's cache, one result for each list of entries in the request. An entry for a combination that the controller already caches replaces the cached image ID.

The caller, the rules, and the errors are:

- *Caller:* A client, for a user with {ref}`superuser access <user-access-controller-superuser>` to the controller. No block applies.
- *Rules:*
  - An entry names its image ID, its source, its stream, its region, the version of the operating system, its architecture, and, when it has them, its virtualisation type and its root storage type and size.
  - An entry that names no stream uses the model's `image-stream` configuration value. An entry that names no region uses the model's cloud region.
  - A save also removes the cached entries that did not come from a `custom` source and are older than five minutes.
  - An entry from a `custom` source with no priority gets the priority of custom cloud data, which outranks the public metadata.
- *Errors:* The client receives each in the result for the list of entries:
  - **`missing <fields>: invalid metadata for image <image ID>`**: *Trigger:* The entry names no version, stream, source, architecture, or region. The client receives it with no error code. *Remediation:* Fill the missing fields.
  - **`unsupported architecture "<name>" (should be any of [<architectures>]): invalid metadata`**: *Trigger:* The architecture is not one Juju builds for. The client receives it with no error code. *Remediation:* Name an architecture Juju builds for.
  - **`image metadata already exists`**: *Trigger:* One list of the request holds two entries for the same combination. The client receives it with no error code. *Remediation:* Send each combination once per list.

(image-metadata-deleting)=
### Deleting image metadata

Deleting image metadata removes the cached entries of one image ID, one result for each image in the request.

The caller, the rules, and the errors are:

- *Caller:* A client, for a user with {ref}`superuser access <user-access-controller-superuser>` to the controller. No block applies.
- *Rules:*
  - Deleting removes every entry that names the image ID.
- *Errors:* The client receives each in the result for the image:
  - **`image id is empty`**: *Trigger:* The request names no image ID. The client receives it with no error code. *Remediation:* Name the image ID to delete.

(image-metadata-and-model-migration)=
## Image metadata and model migration

A migration carries the model's custom image metadata -- the entries whose source is `custom` -- to the target controller, so that machines of the model can start there unchanged. Entries from other sources are not carried: they expire from the cache five minutes after they were cached. See {ref}`Migrating models <model-migrating>`.

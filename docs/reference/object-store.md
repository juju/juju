---
myst:
  html_meta:
    description: "Juju object store reference: the records the controller and model databases keep about stored files, the backends that hold them (file system or S3), the drain that switches between backends, and the operations that store, get, and drain files."
---

(object-store)=
# Object store

The **object store** is the storage of files that a {ref}`controller <controller>` keeps outside its {ref}`databases <database>`: the agent binaries, the files of charm {ref}`resources <charm-resource>`, the output of action tasks, and the files that a backup copies into its archive. The files live in a backend, which is either the controller's own file system or an S3-compatible store. The controller database and each {ref}`model <model>` database keep the records of the files they hold.

(object-store-database-schema)=
## Object store database schema

```{ggarch}
:file: ../juju.ggarch
:view: Object store database schema
:caption: The controller database and each model database keep one record for each stored file, and the controller database also keeps the backend records. A stored object has many paths (each path belongs to one stored object) and many placements (each names one controller node). An agent binary names one stored object. The file store and the task output are records of the model databases alone: a file store names one stored object, and a task output names one path. The controller database keeps one record for each backend, which has one life (alive, dying, or dead) and one type, `file` or `s3`; an S3 backend has one S3 config. A drain names the backend it moves files from and the backend it moves them to, and has one drain phase. The agent binary, the file store, the task output, and the life lookup are described elsewhere and the figure shows them only for context: all boxes are records, and the four with a second, outlined arrow mark in the corner are the records that another page describes or that belong to no page.
:alt: Three groups of boxes of the same size. The group named Controller database and model databases holds Stored object in the middle, with Path above it (m to 1), Placement below it (m to 1), and Agent binary to its left (m to 1). The group named Model databases holds File store, to the right of Stored object (m to 1), and Task output above File store, joined to Path by a line that crosses the group border (m to 1). The group named Controller database holds Backend in the middle, with Life above it (m to 1), S3 config below it (1 to 1), Drain to its left (m to 1), and Drain phase type above Drain (m to 1). Agent binary, File store, Task output, and Life have an outlined arrow mark for records described elsewhere.
```

The object store database schema is the set of records that the controller database and each model database keep about the files they hold. The controller database also keeps the records of the backend that holds them. The records, with the rules that apply to each, are:

- **Stored object:** One stored file: its SHA-256 hash, its SHA-384 hash, and its size. The hashes identify the content, so the same content is one stored object however many paths name it.
  - *Rules:*
    - A stored object has a UUID that is not empty.
    - A SHA-256 appears once.
    - A SHA-384 appears once.
- **Path:** One name by which a stored object is known, such as the path of an agent binary or the path of a charm resource.
  - *Rules:*
    - A path appears once.
    - Each path belongs to one stored object.
- **Placement:** A statement that one {ref}`controller node <controller>` holds the file, named by the node's ID. A controller node adds a placement when it writes or fetches the file, so that other nodes can ask it for the file instead of looking everywhere.
  - *Rules:*
    - A node appears once for a stored object.
- **Backend:** The storage that the controller keeps its files in: its type, its life (alive, dying, or dead), and the time of its last change. A backend of type `file` keeps the files on the controller's own file system; a backend of type `s3` keeps them in an S3-compatible store. The controller database holds one backend of type `file` from the start.
  - *Rules:*
    - A backend has one type, which is `file` or `s3`.
    - Only one backend of type `file` exists.
    - Only one alive backend exists.
    - Only one dying backend exists.
- **S3 config:** The configuration of an S3 backend: the bucket, the region, the endpoint, the access key, and the secret key.
  - *Rules:*
    - A backend has at most one S3 config.
    - The endpoint, the access key, and the secret key are present. The bucket and the region are optional.
- **Drain:** The switch of the controller's files from one backend to another: the phase it is in, the backend it moves the files from, and the backend it moves them to. The backend moved from is the dying backend; the backend moved to is the alive one.
  - *Rules:*
    - The phase is one of `unknown`, `draining`, `error`, and `completed`.
    - The backend moved from is optional; the backend moved to is present.
    - Only one drain that has not ended exists.
- **Agent binary, file store, and task output:** The records that name stored objects: an {ref}`agent version <agent-version>`'s binary, a charm resource's file store, and an {ref}`action <action>` task's output. Other pages own them.
- **Lookups:** The allowed values of the backend type and the drain phase come from the lookup tables `object_store_backend_type` and `object_store_drain_phase_type`. The life values come from the shared lookup table `life`.

(object-store-operations)=
## Object store operations

Object store operations are the actions on the files and the backends of the object store. No client API call reaches the object store directly: the workers of the controller and the {ref}`agents <agent>` of its models store and get files. The workers of the controller drain the files when the backend changes. The one caller outside Juju is the user the {ref}`controller machine agent <controller-agent>` runs as, on the controller machine, who sets the S3 configuration over the controller's control socket (a Unix socket named `control.socket` in the data directory of the controller machine agent, which accepts JSON `POST` requests). The `juju-controller` charm is the usual caller.

Every model of the controller uses the controller's alive backend. The controller creates the backend of type `file` when the controller database is created, so the file system is the backend until an S3 backend takes over.

(object-store-configuring-s3)=
### Configuring the S3 backend

Configuring the S3 backend sets an S3-compatible store as the backend that holds the controller's files. A `POST` request to the `/s3-config` endpoint of the control socket carries a JSON body with the fields `bucket`, `region`, `endpoint`, `access_key`, and `secret_key`. The operation marks the current file backend as dying, adds the S3 backend as the alive backend with its configuration, and starts a {ref}`drain <object-store-draining>` that moves the files.

The operation's required access, rules, and errors are:

- *Required access:* The user the controller machine agent runs as, on the controller machine. The request goes over the control socket, so the caller must be on that machine.
- *Rules:*
  - The endpoint, the access key, and the secret key are required.
  - A bucket name has at least 3 and at most 63 characters, only lowercase letters, digits, and hyphens; it begins and ends with a letter or a digit, starts with neither `xn--` nor `sthree-`, and does not end with `-s3alias`.
  - A request without a bucket uses the bucket `juju-<controller UUID>`.
  - Only a switch from the `file` backend is accepted. A switch from one S3 backend to another is refused.
  - A repeated request with the same configuration, while the drain it started is running, changes nothing.
  - The request body can be at most 1,048,576 bytes.
- *Errors:* The caller receives each with the HTTP status:
  - **`missing request body`** (400): *Trigger:* The request has no body. *Remediation:* Send the JSON fields.
  - **`request body is not valid JSON: <reason>`** (400): *Trigger:* The body is not valid JSON. *Remediation:* Send the JSON fields.
  - **`request body must not exceed 1048576 bytes`** (413): *Trigger:* The body is larger than the limit. *Remediation:* Send only the fields above.
  - **`request Content-Type must be application/json`** (415): *Trigger:* The request's Content-Type header is not `application/json`. *Remediation:* Send the header.
  - **`invalid S3 config: validating S3 credentials: <reason>`** (400): *Trigger:* A required field is missing, or the bucket name breaks the rule above. The reason names the field, for example `endpoint is required`. *Remediation:* Correct the field.
  - **`saving S3 config: transitioning backend to S3: draining already in progress`** (500): *Trigger:* A drain is running and the configuration differs from the one that started it. *Remediation:* Wait for the drain to finish, then send the request again.
  - **`saving S3 config: transitioning backend to S3: backend transition not supported`** (500): *Trigger:* The current backend is an S3 backend. *Remediation:* Removing an S3 configuration is not supported at this time.
  - **`removing S3 config is not supported at this time`** (501): *Trigger:* A `DELETE` request to the same endpoint. *Remediation:* None; the S3 configuration cannot be removed.

A successful request answers `updated S3 config` (200).

(object-store-draining)=
### Draining the object store

Draining the object store moves every file from a dying backend to the backend that replaced it. A drain starts when {ref}`configuring the S3 backend <object-store-configuring-s3>` succeeds. A worker of the controller carries it out.

The operation's required access, rules, and errors are:

- *Required access:* A worker of the controller. No caller can start a drain on its own.
- *Rules:*
  - The drain starts in the phase `draining`. It ends in the phase `completed` when every file has moved, and in the phase `error` when a step fails.
  - A drain in the phase `error` stays there; moving the rest of the files needs manual intervention.
  - While the drain runs, writes to the object store wait. Reads continue from the dying backend until the switch.
  - The worker moves the controller's agent binaries first, then the files of each model.
  - The worker retries a model's drain run up to 10 times, 30 seconds apart, and waits at most one hour for all the models together; the agent binaries get one hour of their own.
  - A completed drain marks the dying backend dead. The controller then has one alive backend, the one moved to.
- *Errors:* The worker records each by setting the phase to `error`:
  - **`draining agent binaries: <reason>`** and **`draining models: <reason>`**: *Trigger:* A drain run failed after its retries. *Remediation:* Find the reason in the controller's log, fix it, and switch again.
  - **`getting model namespaces: <reason>`** and **`completing draining: <reason>`**: *Trigger:* The controller database refused a step of the drain. *Remediation:* Find the reason in the controller's log and retry the switch.

(object-store-storing)=
### Storing files

Storing files writes a file into the controller's backend and records its stored object and path. The callers are the workers of the controller and the agents of its models: the controller stores the agent binaries that {ref}`uploading agent binaries <agent-version-uploading>` receives, a unit agent stores the files of the charm resources it downloads, and a controller worker stores the output of an action task.

The operation's required access, rules, and errors are:

- *Required access:* A worker of the controller or an agent of one of its models. No client API call stores a file directly.
- *Rules:*
  - The file is written to a temporary file first, and both hashes are computed while it is written. The temporary file is moved to its final place, a file named by the file's SHA-384 hash, only after the whole file is written.
  - Before the file is written or removed, the caller claims a {ref}`lease <lease>` on the file's hash, so that no two callers move the same file at once. The lease is held for 30 seconds and extended while the write runs.
  - A file that the hash already names is left as it is. A file with the same name but a different size is refused.
  - The stored object is recorded after the file is in place, with both hashes and the size. A stored object whose hash exists already is not added again: the new path names the existing stored object.
  - The path is recorded with the stored object. A path that exists with a different hash is refused.
  - The caller gives its own controller node as a placement for the file, so that other nodes can find it.
  - The hashes come as a pair: a stored object with one hash and not the other is refused.
  - Removing a path deletes the stored object's record for that path. The file itself is deleted only when no path names its hash any more.
- *Errors:* The operation returns each to its caller:
  - **`file locked`**: *Trigger:* Another caller holds the lease on the file's hash. *Remediation:* Retry; the lease lasts 30 seconds.
  - **`partially written data: written <n>, expected <m>`**: *Trigger:* The input ended before the announced size. *Remediation:* Send the whole file.
  - **`encoded as "<sha384>": object already exists`**: *Trigger:* A file with the same hash exists with a different size. *Remediation:* Remove the file, or send the file it was replaced with.
  - **`hash mismatch for "<path>": expected "<hash>", got "<hash>"`**: *Trigger:* The caller named the SHA-384 hash to check, but the file's hash differs. *Remediation:* Send the file that matches the hash.
  - **`missing hash256`** and **`missing hash384`**: *Trigger:* One hash is present and the other is not. *Remediation:* Send both hashes, or neither.
  - **`path already exists with different hash`**: *Trigger:* The path names a different stored object. *Remediation:* Remove the path first.

(object-store-getting)=
### Getting files

Getting files returns a stored file to the worker or agent that asked for it. The file is found by its path, by its SHA-256 hash, or by the first 7 to 64 characters of its SHA-256 hash.

The operation's required access, rules, and errors are:

- *Required access:* A worker of the controller or an agent of one of its models. A client reads a stored file through the operation that owns it, such as {ref}`downloading a charm resource <charm-resource-listing>`.
- *Rules:*
  - A hash prefix has at least 7 and at most 64 characters, each a digit or a letter from `a` to `f`.
  - A file that the controller node does not hold is fetched from another controller node. The placements of the stored object name the nodes to ask, in no fixed order, so that the load spreads over the nodes. With no placement, the controller asks the nodes it knows.
  - A file fetched from another node is checked against the stored object's size and SHA-384 hash. The file is then written to the local file system and placed on the node, so that the next read is local.
- *Errors:* The operation returns each to its caller:
  - **`size mismatch for "<path>": expected <n>, got <m>`** and **`hash mismatch for "<path>": expected "<hash>", got "<hash>"`**: *Trigger:* The file that another node sent does not match the stored object. *Remediation:* Remove the file from the node that holds it, so that it is fetched again.
  - **`minimum has prefix length is 7: minimum hash prefix length is 7`** and **`<prefix>: invalid hash prefix`**: *Trigger:* The prefix is shorter than 7, longer than 64, or has a character outside `a` to `f` and `0` to `9`. *Remediation:* Use a longer prefix, or fix the characters.

## Object store and model migration

A migration does not carry the model's stored-object, path, and placement records: the target drops them from the imported database. The charms, the agent binaries, and the resources transfer over the migration's file endpoints. The target stores each file again and records its stored object and path. A migrated model reads its files from the target controller's backend. When the migration activates the model, the controller checks the {ref}`agent binaries <agent-version>` at the target version in the catalogues of the controller and of the model: for every architecture that the model's running agents report, a binary must exist in the controller's catalogue or in the model's own. A missing binary does not fail the migration; the model keeps its current agent version. The controller warns. See {ref}`Migrating models <model-migrating>`.

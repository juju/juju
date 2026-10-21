---
myst:
  html_meta:
    description: "Juju charm resource reference: the file and OCI image content that a charm declares, the records the model keeps about it, how a unit fetches it, and the calls that list, add, upload, activate, download, poll, and remove resources."
---

(charm-resource)=
# Resource (charm)

```{ibnote}
See also: {ref}`Juju | Manage charm resources <manage-charm-resources>`
```

A **charm resource** is a named piece of content, a file or an [OCI image](https://opencontainers.org/) reference, that a {ref}`charm <charm>` declares and that the {ref}`units <unit>` of an {ref}`application <application>` of that charm fetch to run. The charm declares the resource. The application uses one revision of it, which comes from Charmhub or from a file that a user uploads.

(charm-resource-database-schema)=
## Charm resource database schema

```{ggarch}
:file: ../juju.ggarch
:view: Resource database schema
:caption: The model database keeps one record for each resource. A charm resource (declared by a charm) has many resources, and a resource belongs to one charm resource. An application resource links one resource to one application, and an application has many application resources. A pending resource links one resource to the name of an application that does not exist yet. A retrieved-by record belongs to one resource. A unit resource links a unit to a resource, a unit has many unit resources, and a resource has many unit resources. A file store belongs to one resource and points to one stored object, which many file stores can share. An image store belongs to one resource and points to one image metadata record, which many image stores can share. The application, the unit, the charm resource, and the stored object are described on other pages or in the text.
:alt: Twelve boxes of the same size inside a group named Model database, each with a filled table mark in the corner. Resource is in the middle. Charm resource is to its left (1 to m). Application resource is above it (1 to 1), with Application to the left of Application resource (1 to m) and Pending resource to its right (1 to 1). Retrieved by is to the right of Resource (1 to 1). Unit resource is below Resource (m to 1), with Unit to its left (1 to m). File store is to the right of Unit resource (1 to 1) and Stored object to the right of File store (m to 1). Image store is below File store (1 to 1) and Image metadata to the right of Image store (m to 1).
```

The charm resource database schema is the set of records that the {ref}`model database <database>` keeps about the resources of a model. A charm declares each resource. The model then holds each revision of it that an application uses or may use, the units that use a revision, and the stored content. The records, with the rules that apply to each, are:

- **Charm resource:** One resource that a charm declares: its name, its kind, the path of the file, and a description. The kind is one of:
  - **`file`** is the base case: content that a unit reads as a file, such as an archive, a database, or a media file.
  - **`oci-image`** is a reference to a container image, with optional credentials for the registry that holds it.
  - *Rules:*
    - A charm declares a resource name once.
    - A charm resource is never updated. It is added with its charm and deleted with its charm.
- **Resource:** One revision of a charm resource that Juju knows about: its revision number (empty when it has none), the time it was created, and the time Juju last asked Charmhub for newer revisions. Its origin is one of:
  - **`store`** is the base case: a revision that Charmhub holds.
  - **`upload`** is a file or an image reference that a user provided.

  Its state is one of:
  - **`available`** is the base case: the revision that an application's units use.
  - **`potential`** is a newer revision in Charmhub that a user can upgrade to.
  - *Rules:*
    - A resource refers to one charm resource.
    - The origin is `store` or `upload`. The state is `available` or `potential`.
- **Application resource:** The link that makes a resource the one that an application uses. The resource can belong to an earlier charm revision than the application has now, because it belongs to the charm revision that the application had when Juju acquired the resource.
  - *Rules:*
    - A resource is linked to one application.
- **Pending resource:** The link from a resource to the name of an application that does not exist yet. It lasts from the moment a client adds the resource to the moment the application is created or refreshed.
  - *Rules:*
    - A resource has one pending link.
- **Unit resource:** The link that records that a unit uses a resource, with the time it was added.
  - *Rules:*
    - A unit has one resource for each charm resource name. A newer link for the same name replaces the older one.
    - A unit resource refers to a resource of the same charm resource name.
- **Retrieved by:** Who put the content in the store: a `user` (the user who uploaded it), or a `unit` or an `application` (the one whose request made Juju download it from Charmhub).
  - *Rules:*
    - A resource has one retrieved-by record.
- **File store:** The link from a `file` resource to its content in the object store (the controller's store of blobs), with the content's size in bytes and SHA-384 fingerprint.
  - *Rules:*
    - A resource has one file store.
- **Image store:** The link from an `oci-image` resource to its image metadata, with the size and SHA-384 fingerprint of the metadata.
  - *Rules:*
    - A resource has one image store.
- **Image metadata:** The registry path of the image, with the username and password for the registry when it is private. Juju keeps the metadata and never pulls the image.
  - *Rules:*
    - A storage key appears once.

(charm-resource-change-streams)=
## Charm resource change streams

Charm resource change streams are the notifications that tell a unit agent that the resources of its application have changed. The stream carries one kind of change:

- **Application changes:** A unit agent calls `WatchApplication` to hear that its application changed, which includes a new charm URL and a higher charm modified version. Uploading a resource for an application raises its charm modified version (see {ref}`uploading resources <charm-resource-uploading>`). The unit agent then runs the {ref}`upgrade-charm <hook-upgrade-charm>` hook, which is the place for a charm to fetch its resources again.

(charm-resource-calls)=
## Charm resource calls

Charm resource calls are the calls that {ref}`clients <client>`, unit agents, and the controller take on charm resources. A **{ref}`unit agent <unit-agent>`** is the process that runs the charm of a unit. It logs in as its unit and acts only for that unit. An agent that logs in as an application acts for any unit of that application. No model access level or block applies to a unit agent. Clients list the resources of a charm and of an application, add, upload, and activate resources, and download them. Unit agents fetch the content that a hook asks for. The controller polls Charmhub for new revisions and removes resources that nothing uses.

(charm-resource-listing-charm-resources)=
### Listing charm resources

```{ibnote}
See also: {ref}`Juju | Manage charm resources <manage-charm-resources>`
```

Listing charm resources shows the resources that a charm on Charmhub declares, each with its latest revision. A client calls `ListCharmResources` on the `Charms` facade with one or more charm URLs and their origins.

The caller, the rules, and the errors are:

- *Caller:* A client, for a user with {ref}`read access <user-access-model-read>` to the model. No block applies.
- *Rules:*
  - The charm URL must be a Charmhub charm.
  - The channel, base, and architecture in the origin select the revision. The architecture defaults to the default architecture of the model.
- *Errors:* The client receives:
  - **`charm "<name>" not valid`** (code `not valid`): *Trigger:* The charm URL is not a Charmhub URL, for example a local charm. *Remediation:* Name a charm on Charmhub.
  - **`permission denied`** (code `unauthorized access`): *Trigger:* The caller lacks read access to the model. *Remediation:* Ask a model administrator for read access.

(charm-resource-adding)=
### Adding resources

```{ibnote}
See also: {ref}`Juju | Manage charm resources <manage-charm-resources>`
```

Adding resources records which revision or upload to use for each resource of a charm before an application is created or its charm changes. A client calls `AddPendingResources` on the `Resources` facade with an application tag, a charm URL, the charm's origin, and one entry for each resource. Each entry has a name, an origin, and for a `store` origin a revision. The answer lists one resource ID for each entry, in order. {ref}`Activating resources <charm-resource-activating>` uses the IDs.

The caller, the rules, and the errors are:

- *Caller:* A client, for a user who can log in to the model, which needs {ref}`read access <user-access-model-read>`. The facade checks no further access level and no block.
- *Rules:*
  - The application does not have to exist.
  - The charm and each named charm resource must already be in the model.
  - Juju resolves each entry with the origin `store` against Charmhub. A revision of -1 selects the revision that the charm's channel carries. A revision equal to that one takes its details. A different revision without a fingerprint is looked up in Charmhub. A different revision with a fingerprint is used as given.
  - An entry with the origin `upload` is kept as given. For a local charm, every entry becomes an `upload` entry.
  - A `store` entry has a revision. An `upload` entry has none.
  - Each entry becomes a resource in the state `available`, linked to the application name as a pending resource.
- *Errors:* The client receives each in the `Error` field of the result:
  - **`"<tag>" is not a valid application tag`** (code `bad request`): *Trigger:* The tag is not `application-<name>`. *Remediation:* Send the tag of the application.
  - **`application <name> not found`** (code `not found`): *Trigger:* The name belongs to a synthetic application (the local stand-in for an application offered by another model). *Remediation:* Use an application of this model.
  - **`parsing charm URL "<url>": <reason>`**: *Trigger:* The charm URL does not parse. *Remediation:* Send a valid charm URL.
  - **`bad resource info for "<name>": <reason>`**: *Trigger:* An entry has an invalid origin, type, or fingerprint. *Remediation:* Correct the entry.
  - **`revision is empty for store resource: argument not valid`** and **`revision is set for upload resource: argument not valid`**: *Trigger:* The revision does not match the origin. *Remediation:* Give a revision for `store` and none for `upload`.
  - **`resource name is empty: resource name not valid`**: *Trigger:* An entry has no name. *Remediation:* Name the resource.
  - **`charm or charm resource does not exist: charm resource not found`**: *Trigger:* The charm is not in the model, or it does not declare the resource. *Remediation:* Check the names against the charm.

(charm-resource-uploading)=
### Uploading resources

```{ibnote}
See also: {ref}`Juju | Manage charm resources <manage-charm-resources>`
```

Uploading resources stores the content of a resource that has the origin `upload`. A client sends `PUT /model/<uuid>/applications/<application>/resources/<name>` with the content as the body. The request is either for a resource that {ref}`adding resources <charm-resource-adding>` made, which it names with the query `pendingid=<resource ID>`, or for the resource that the application uses now, which it names with no pending ID.

The caller, the rules, and the errors are:

- *Caller:* A client, for a user with {ref}`write access <user-access-model-write>` to the model, or {ref}`superuser access <user-access-controller-superuser>` to the controller. The change block (see {ref}`restricting commands on a model <restrict-commands-on-a-model>`) must not be on.
- *Rules:*
  - The request has the content type `application/octet-stream`, the header `Content-Sha384` with the SHA-384 fingerprint of the content, a `Content-Disposition` header with a `filename` parameter, and the size in `Content-Length`.
  - The resource must have the origin `upload`.
  - For a `file` resource, the extension of the file name must equal the extension of the path that the charm declares.
  - Juju hashes the body. The size and the fingerprint must equal the ones in the request, or Juju refuses the upload and stores nothing.
  - For an `oci-image` resource, the body is a JSON or YAML document with the registry path (the key `ImageName` in JSON, `registrypath` in YAML), and optionally `username` and `password`. The document is at most 1 MiB. Juju derives the size and the fingerprint from the document and keeps its own values when they differ from the request.
  - With a pending ID, Juju stores the content for that resource.
  - With no pending ID, Juju first creates a new resource for the same charm resource, with the origin `upload`, the state `available`, and no revision. The new resource takes the place of the application's old one. Juju keeps the old resource for units that still use it and schedules it for {ref}`removal <charm-resource-removing>`. After storing the content, Juju raises the charm modified version of the application by one, so that its units hear of the change (see {ref}`change streams <charm-resource-change-streams>`).
  - The uploading user is recorded as the one who retrieved the content.
  - Storing content for a resource that already has different content is refused. If recording the content fails, Juju removes the stored content.
  - The answer is the resource: its details, ID, application name, uploader, and time.
- *Errors:* The client receives:
  - **`permission denied`** (code `unauthorized access`): *Trigger:* The caller has neither write access to the model nor superuser access. *Remediation:* Ask a model administrator for write access.
  - **The block message** (code `operation is blocked`): *Trigger:* The change block is on. *Remediation:* Remove the block.
  - **`unsupported content type "<type>"`**: *Trigger:* The content type is not `application/octet-stream`. *Remediation:* Send the body as raw bytes.
  - **`parsing fingerprint: <reason>`**: *Trigger:* The `Content-Sha384` header is missing or malformed. *Remediation:* Send the SHA-384 of the content in hexadecimal.
  - **`missing "filename" in resource upload request`** and **`badly formatted Content-Disposition: <reason>`**: *Trigger:* The `Content-Disposition` header lacks the file name or is malformed. *Remediation:* Send `Content-Disposition` with a `filename`.
  - **`application "<application>", resource "<name>" not found`** (code `not found`): *Trigger:* The request has no pending ID and the application does not use a resource of that name. *Remediation:* {ref}`List the resources <charm-resource-listing>` of the application.
  - **`resource <ID> is not of type upload`**: *Trigger:* The resource has the origin `store`. *Remediation:* Select a revision number for that resource.
  - **`incorrect extension on resource upload "<file name>", expected "<extension>"`**: *Trigger:* The file extension differs from the charm's declared path. *Remediation:* Upload a file with the declared extension.
  - **`downloading resource body: downloaded resource has unexpected hash: got "<x>", expected "<y>"`** and **`downloading resource body: downloaded resource has unexpected size: got <n>, expected <m>`**: *Trigger:* The body does not match the fingerprint or the size of the request. *Remediation:* Compute both from the file that is sent.
  - **`storing resource <name> of application <application>: <reason>`**: *Trigger:* Storing failed. The reason is `stored resource already exists` when different content is already stored, `container image resource exceeds maximum size of 1048576 bytes` for an image document over 1 MiB, or a parsing message when the document is neither JSON nor YAML. *Remediation:* Fix the cause that the reason names.

(charm-resource-activating)=
### Activating resources

Activating resources makes the pending resources of an application the ones it uses. A client does it when it deploys an application or refreshes its charm, by passing the resource IDs that {ref}`adding resources <charm-resource-adding>` returned.

The caller, the rules, and the errors are:

- *Caller:* A client, for a user with {ref}`write access <user-access-model-write>` to the model. The change block (see {ref}`restricting commands on a model <restrict-commands-on-a-model>`) must not be on, except for a refresh that forces its units.
- *Rules:*
  - A deployment names one resource ID for each resource that the charm declares.
  - Each ID must be a resource that is pending for the application, in the state `available`, with the charm resource name, the charm, and the kind of the destination charm.
  - A refresh names IDs only for the resources that change. A resource that the destination charm declares and that has no ID needs a resource that the application already uses.
  - A replaced resource takes the place of the old one in the application, and Juju schedules the old one for {ref}`removal <charm-resource-removing>`.
  - A resource that the destination charm no longer declares is detached from the application and scheduled for removal.
  - When a deployment fails, Juju deletes the pending resources it was given. It logs the cleanup and does not report it.
- *Errors:* The client receives each with no error code unless noted:
  - **`not all pending resources for charm provided`**: *Trigger:* A deployment names a different number of resources than the charm declares. *Remediation:* Add every declared resource first.
  - **`resource "<name>" is not defined by destination charm: invalid resource args`**: *Trigger:* A refresh names a resource that the new charm does not declare. *Remediation:* Use a name from the new charm.
  - **`resource "<name>" is not pending for the application: invalid resource args`**: *Trigger:* The ID is not pending for this application, or its state is not `available`. *Remediation:* Add the resource again for this application.
  - **`resource "<name>" does not match the destination charm: invalid resource args`**: *Trigger:* The pending resource is for another charm or has another kind. *Remediation:* Add the resource for the destination charm.
  - **`resource "<name>" is not resolved for the destination charm: invalid resource args`**: *Trigger:* The new charm declares a resource that the application does not use yet and the request names none. *Remediation:* Add the resource and pass its ID.
  - **The block message** (code `operation is blocked`): *Trigger:* The change block is on. *Remediation:* Remove the block.

(charm-resource-listing)=
### Listing resources

```{ibnote}
See also: {ref}`Juju | Manage charm resources <manage-charm-resources>`
```

Listing resources shows, for each application that a client names, its resources, the resources of each unit, and the newer revisions that Charmhub carries. A client calls `ListResources` on the `Resources` facade with application tags.

The caller, the rules, and the errors are:

- *Caller:* A client, for a user who can log in to the model, which needs {ref}`read access <user-access-model-read>`. The facade checks no further access level and no block.
- *Rules:*
  - The call takes a list of application tags. Each tag gets its own result.
  - A result holds the application's resources (the state `available`), the resources of each of its units, and its `potential` resources, which are the newer revisions in Charmhub.
  - Each resource has its name, kind, path, description, origin, revision, size, fingerprint, who retrieved the content, and the time it was added.
- *Errors:* The client receives each in the result for the tag:
  - **`"<tag>" is not a valid application tag`** (code `bad request`): *Trigger:* The tag is not `application-<name>`. *Remediation:* Send the tag of the application.
  - **`application <name> not found`** (code `not found`): *Trigger:* The name belongs to a synthetic application. *Remediation:* Use an application of this model.
  - **`application not found`**: *Trigger:* The model has no application of that name. The client receives it with no error code. *Remediation:* Check the name against the applications of the model.

(charm-resource-downloading)=
### Downloading resources

Downloading resources gives a client the stored content of an application's resource. A client sends `GET /model/<uuid>/applications/<application>/resources/<name>`.

The caller, the rules, and the errors are:

- *Caller:* A client, for a user with {ref}`read access <user-access-model-read>` to the model, or {ref}`superuser access <user-access-controller-superuser>` to the controller. A controller agent, a machine agent, or an application agent can also download. No block applies.
- *Rules:*
  - The resource is the one that the application uses for the name.
  - The answer is the content as `application/octet-stream`, with its size in `Content-Length`.
  - Juju sends only content that the controller already stores. It does not download from Charmhub for this request.
- *Errors:* The client receives:
  - **`application <name> not found`** (code `not found`): *Trigger:* The name belongs to a synthetic application. *Remediation:* Use an application of this model.
  - **`resource <name> of application <application> not found`** (code `not found`): *Trigger:* The application does not use a resource of that name, or the application does not exist. *Remediation:* {ref}`List the resources <charm-resource-listing>` of the application.
  - **`resource <name> of application <application> has no blob downloaded on controller`** (code `not found`): *Trigger:* The controller stores no content for the resource yet. *Remediation:* Upload the content, or wait until a unit fetches it.
  - **`permission denied`** (code `unauthorized access`): *Trigger:* The caller lacks read access to the model. *Remediation:* Ask a model administrator for read access.

(charm-resource-fetching)=
### Fetching resources

```{ibnote}
See also: {ref}`resource-get <hook-command-resource-get>`
```

```{ggarch}
:file: ../juju.ggarch
:view: Fetching a resource for a unit
:caption: Fetching a resource goes in this order. A hook runs `resource-get`, and the unit agent asks the controller for the resource by name. The controller finds the stored content of the application's resource. When no content is stored, the controller downloads the revision from Charmhub and stores it with the name of the unit that asked. The controller sends the content with its fingerprint to the unit agent, and then records that the unit uses the resource. The unit agent asks for the resource's details, checks the fingerprint, and writes the file. A failed recording is only logged. A failed download or a missing resource reaches the unit agent as an error.
:alt: A sequence diagram with three participants: the unit agent, the controller, and Charmhub. The unit agent asks the controller for a resource by name. The controller finds the stored content and downloads the revision from Charmhub, which returns the content. The controller stores the content and who retrieved it, and returns the content and its fingerprint to the unit agent. The controller then records that the unit uses the resource. The unit agent asks for the resource's details, then checks the fingerprint and writes the file.
```

Fetching resources gives a unit the content of its application's resource as a file. A hook runs the {ref}`resource-get <hook-command-resource-get>` hook command. The unit agent asks the controller for the content and for the details of the resource, then the command prints the path of the file. A charm can run the command in any {ref}`hook <hook>`. The {ref}`install <hook-install>` and {ref}`upgrade-charm <hook-upgrade-charm>` hooks are the usual places.

The caller, the rules, and the errors are:

- *Caller:* A unit agent, for its own unit. The content request is `GET /model/<uuid>/units/<unit>/resources/<name>`. An application agent can ask for any unit of its application. The details request is `GetResourceInfo` on the `ResourcesHookContext` facade.
- *Rules:*
  - Juju looks for the resource that the application uses for the name. When the application has none, it looks for the resource that the unit uses.
  - When the controller stores the content, it sends it.
  - For a `store` resource without stored content, the controller downloads the revision from Charmhub, stores it with the unit as the one who retrieved it, and sends it. A local charm has no repository to download from, so the request fails with `not found`.
  - For an `upload` resource without stored content, nothing can be sent. The answer is `not found`.
  - Controller configuration limits the downloads that run at once: `application-resource-download-limit` for each application and `controller-resource-download-limit` for all applications. Each is 0 (no limit) by default. A request waits for a free slot.
  - The answer has the headers `Content-Type: application/octet-stream`, `Content-Length`, and `Content-Sha384`.
  - After Juju sends the content, it records that the unit uses the resource. A failure is only logged.
  - `GetResourceInfo` takes resource names and answers each one with the resource, using the same lookup. A name with no resource gets a `not found` error in its own result.
  - The unit agent writes the file under the unit's resources directory, at the path of the charm resource. A file that already has the same fingerprint stays as it is. Otherwise the agent checks the fingerprint and the size of the content it writes.
  - The command takes one resource name.
- *Errors:* The hook command prints the first two. The controller returns the others:
  - **`could not download resource: <reason>`**: *Trigger:* The request failed or the content did not match its fingerprint. *Remediation:* Read the reason, then retry the hook.
  - **`missing required resource name`**: *Trigger:* The command ran with no name. *Remediation:* Run `resource-get <name>`.
  - **`permission denied`** (code `unauthorized access`): *Trigger:* The named unit is not the caller's unit, nor a unit of the application agent's application. *Remediation:* Ask for the caller's own unit.
  - **`missing resource name bad request`** (code `bad request`): *Trigger:* The name in the URL is empty. *Remediation:* Give a name.
  - **`resource "<name>" not found`** (code `not found`): *Trigger:* The application and the unit have no such resource, or an `upload` resource has no stored content, or a local charm's `store` resource has no stored content. *Remediation:* {ref}`List the resources <charm-resource-listing>` and upload the content.

(charm-resource-polling)=
### Polling Charmhub for resource revisions

Polling Charmhub for resource revisions lets Juju tell users that a newer revision of a resource exists. The controller does it for each application whose charm comes from Charmhub, in the pass in which it checks for new charm revisions. A client sees the result as the `potential` resources in {ref}`listing resources <charm-resource-listing>`.

The caller, the rules, and the errors are:

- *Caller:* The controller.
- *Rules:*
  - The pass runs about every 24 hours, with a random delay.
  - An application whose charm has no resources is skipped.
  - When a `potential` resource already has the polled revision for the same charm, Juju only updates its last-polled time. Otherwise it creates a new `potential` resource that takes the place of the old one in the application, and schedules the old one for {ref}`removal <charm-resource-removing>`.
  - The `available` resource of the application stays as it is.
- *Errors:* The controller logs each and goes on:
  - **`failed to get application UUID for "<name>": <reason>`** (warning): *Trigger:* The application was removed during the pass. *Remediation:* None is needed.
  - **`Resource not found for application <name> (<UUID>), missing: [<names>]`**: *Trigger:* Charmhub lists a resource for which the application has no `potential` resource. *Remediation:* None is needed. Juju skips that resource.

(charm-resource-removing)=
### Removing resources

Removing resources deletes a resource and its stored content once nothing uses it. The controller does it, as a removal job that an earlier operation schedules.

The caller, the rules, and the errors are:

- *Caller:* The controller.
- *Rules:*
  - Juju schedules a job when an upload, an activation, or polling replaces a resource, and when an activation detaches a resource.
  - The job deletes the resource only when no application resource, pending resource, or unit resource refers to it. It deletes the retrieved-by record and the store link with it. It deletes the stored content or the image metadata when nothing else uses it.
  - A resource that is already gone counts as removed.
  - Removing an application deletes its resources. Removing a unit deletes its unit resources.
- *Errors:* The controller logs the error and retries about every 10 seconds:
  - **`resource "<UUID>" is still in use`**: *Trigger:* A unit still uses the resource. *Remediation:* None is needed. The job completes after the last unit stops using the resource.

(charm-resource-and-model-migration)=
## Charm resources and model migration

A migration exports the resource records with the model's database: each application's resource and each unit's resource. The content of a resource that has a stored blob is uploaded to the target controller with a `POST` to `/migrate/resources`, which needs {ref}`superuser access <user-access-controller-superuser>` and works only while the target is importing the model (see {ref}`Migrating models <model-migrating>`). Resources without a blob and the unit resources are not uploaded, because they migrate with the database. A resource with the origin `upload` arrives with no revision number.

---
myst:
  html_meta:
    description: "Juju charm reference: what a charm is, the records the model keeps about it, the changes that follow it, and the operations that resolve, add, upload, list, show, download, and remove charms."
---

(charm)=
# Charm

```{ibnote}
See also: {ref}`manage-charms`
```

A **charm** is the packaged operations code of an {ref}`application <application>`: an archive with metadata, configuration options, actions, and a manifest, plus the code that deploys, configures, scales, and integrates the application on any {ref}`Juju-supported cloud <list-of-supported-clouds>`. A {ref}`model <model>` keeps one record for each revision of a charm. An {ref}`application <application>` or {ref}`unit <unit>` need not use the revision. Charms come from [Charmhub](https://charmhub.io/) or from a local archive. Juju removes a charm when no application and no unit uses it any more.

(charm-data-model)=
## Charm data model

```{ggarch}
:file: ../juju.ggarch
:view: Charm data model
:caption: The model database keeps one record for each charm revision. A charm has one metadata record. A charm has many relations, many manifest bases, many config options, many actions, many resources, and many containers, each as its own record that belongs to one charm. An application points at one charm, and a charm can serve many applications. The application has its own page and the figure shows it only for context. The storage, device, extra binding, hash, download information, term, tag, category, container mount, and storage property records of a charm, and the fixed lists of allowed values, are described in the text and are not boxes in the figure.
:alt: Nine boxes of the same size inside a group named Model database, each with a filled table mark in the corner. Charm is in the middle. Metadata is above it (1 to 1). Manifest base is below it (m to 1). Application is to its left (m to 1) and has an outlined arrow mark for a record that another page describes. Relation is to its right (m to 1). Config option is above and to the left of Charm and Action is above and to the right (each m to 1). Container is below and to the left and Resource is below and to the right (each m to 1).
```

(charm-anatomy)=
The charm data model is the set of records that the {ref}`model database <database>` keeps about the charms of a model. The records, with the rules that apply to each, are:

- **Charm:** One revision of a charm: where it comes from, its name and revision, the version string that its author set, whether its archive is available, and the place of that archive in the model's object store (the store in which Juju keeps files for the model). A charm record is either pending, with its archive not yet stored (available is false), or available.

  (charm-revision)=
  - **Revision:** A number that identifies one version of a charm. The revision of a Charmhub charm is the number that Charmhub assigned when the author uploaded that version. Juju numbers the revisions of local charms itself (see {ref}`uploading charms <charm-uploading>`).
  - **Source:** `local` for an archive that a client uploaded, `charmhub` for a charm from Charmhub, and `cmr` for the stand-in charm of a remote application in a {ref}`cross-model relation <cross-model-relation>`.
  - **Reference name:** The name under which Juju first received the charm. For a Charmhub charm it is the name in the store, which can differ from the name in the charm's own metadata.
  - *Rules:*
    - A source, reference name, and revision appear once together, so that a model has one record for each revision of a charm.
    - A reference name is not empty.
    - The architecture, when present, is one of `amd64`, `arm64`, `ppc64el`, `s390x`, and `riscv64`. The charm of a remote application has none.
    - The source is one of `local`, `charmhub`, and `cmr`.
- **Metadata:** What the author declared about the charm: its name, summary, description, minimum Juju version, the user it runs as (`default`, `root`, `sudoer`, or `non-root`), and its `assumes` expression (the conditions that the model must satisfy). Terms, tags, and categories are lists of free-form values in their own records.

  (charm-taxonomy-by-role)=
  Whether a charm is subordinate decides its kind. A charm is a principal charm or a subordinate charm:

  (principal-charm)=
  - **Principal charms** are the base case: charms that run on their own.

  (subordinate-charm)=
  - **Subordinate charms** are machine charms that the author declared subordinate. A subordinate charm augments the functionality of another charm, its principal, and runs on the same machine. See {ref}`subordinate relations <subordinate-relation>` for how its units come into being.
  - *Rules:*
    - A charm has one metadata record.
    - A metadata record has a name.
    - Subordinate is true or false, and false unless set.
    - The user is one of `default`, `root`, `sudoer`, and `non-root`.
- **Relation:** One endpoint that the charm declares, with its name, interface, role (`provider`, `requirer`, or `peer`), scope (`global` or `container`), whether it is optional, and its capacity. {ref}`Applications <application-endpoint>` pair these endpoints with their own, and {ref}`relations <relation>` join them.
  - *Rules:*
    - A charm has each relation name once.
    - The role is one of `provider`, `requirer`, and `peer`.
    - The scope is one of `global` and `container`.
- **Extra binding:** The name of a binding that the charm declares without a relation, so that an application can bind it to a {ref}`space <space>`.
- **Config option:** One setting of the charm, with its key, type (`string`, `int`, `float`, `boolean`, or `secret`), default value, and description. {ref}`Application configuration <application-configuration>` sets the values.
  - *Rules:*
    - A charm has each key once.
    - The type is one of `string`, `int`, `float`, `boolean`, and `secret`.
- **Action:** One {ref}`action <action>` that the charm declares, with its key, description, parameters, whether it can run in parallel, and its execution group.
  - *Rules:*
    - A charm has each action key once.
- **Resource:** One {ref}`charm resource <charm-resource>` that the charm declares, with its name, kind (`file` or `oci-image`), path, and description.
  - *Rules:*
    - A charm has each resource name once.
    - The kind is one of `file` and `oci-image`.
- **Storage:** One kind of {ref}`storage <storage>` that the charm asks for, with its name, description, kind (`block` or `filesystem`), whether it is shared or read-only, the minimum and maximum number of instances, its minimum size in MiB, its location, and a list of properties.
  - *Rules:*
    - A charm has each storage name once.
    - The kind is one of `block` and `filesystem`.
    - A storage has a minimum and a maximum count.
- **Device:** One device that the charm asks for, with its key, name, description, type, and minimum and maximum count.
  - *Rules:*
    - A charm has each device key once.
- **Container:** One workload container that the charm declares, with its key, the resource that holds its image, the user ID and group ID it runs with (`-1` when not set, because `0` is root), and its mounts. A mount names a storage of the charm and a location in the container.

  (charm-taxonomy-by-substrate)=
  The containers decide the substrate that a charm is designed for:

  (machine-charm)=
  - **Machine charms** are the base case: charms that declare no container. They are designed to run on a resource of a machine cloud: a bare metal machine, a virtual machine, or a system container.

  (kubernetes-charm)=
  - **Kubernetes charms** are charms that declare at least one container. A Kubernetes charm runs in a container of a pod on a Kubernetes cloud.

  (sidecar-charm)=
  - **Sidecar charms** are Kubernetes charms in the sense that Juju 4 uses: the charm runs in a container of the same pod as the workload, so that each unit has an operator beside the workload, and the charm manages the workload through [Pebble](https://documentation.ubuntu.com/pebble/).

  (infrastructure-agnostic-charm)=
  - **Infrastructure-agnostic charms** are charms that declare no container and do nothing that only makes sense on machines.
  - *Rules:*
    - A container key appears once for a charm.
    - A mount belongs to a container of the same charm.
- **Manifest base:** One base that the charm supports: an operating system (`ubuntu`), a channel of that base, and an architecture. A charm lists its bases in order. The channel of a base has the form `<track>/<risk>/<branch>`, such as `22.04/stable`.

  (charm-channel)=
  A **charm channel** is a charm release identifier with the same form, such as `3/stable` for the charm `kafka` on Charmhub. The application keeps the channel it follows (see {ref}`application <application>`). The parts of a channel are:

  (charm-channel-track)=
  - **Track:** A name that collects several supported releases of a charm under one name, such as a major version. A deployment with no track follows the track `latest`.

  (charm-channel-track-guardrail)=
  - **Track guardrail:** A pattern that [Charmhub](https://charmhub.io/) applies to new tracks of a charm, so that the tracks of one charm follow one naming pattern. Charmhub enforces it, and Juju does not.
  - **Risk:** One of `stable`, `candidate`, `beta`, and `edge`. When a channel names no risk, Juju uses `stable`.
  - **Branch:** An optional finer subdivision of a channel for short-lived releases that the author pushes for a fix or a trial. A channel that names a branch also names a risk.
  - *Rules:*
    - A base has a risk.
    - The operating system is `ubuntu`.
    - The architecture is one of `amd64`, `arm64`, `ppc64el`, `s390x`, and `riscv64`.
- **Hash:** The SHA-256 digest of the charm's archive.
  - *Rules:*
    - A charm has at most one hash.
    - The only kind of hash is `sha256`.
- **Download information:** How Juju fetches the archive of a Charmhub charm: the download URL, the size, the identifier that Charmhub gives the charm, and where the information came from (`download`, `migration`, `upload`, or `bootstrap`). Charms from Charmhub have it. A charm that arrives through a migration has it too. A charm uploaded from a local archive has none.
  - *Rules:*
    - A charm has at most one download information record.
    - A download URL and a size are present.
- **Application and unit:** The records that use a charm. Other pages own them: see {ref}`application <application>` and {ref}`unit <unit>`. Each of them points at one charm.

(charm-taxonomy)=
(charm-taxonomy-by-function)=
(charm-taxonomy-by-architecture)=
(charm-taxonomy-by-generation)=
Charms are also described by what they do for their workload and by how their authors wrote them. These descriptions are not stored, because nothing in the records decides them:

(workloadless-charm)=
- **Workloadless charms** run no workload locally. The Data Integrator is one.

(configurator-charm)=
- **Configurator charms** are workloadless charms that configure another charm once the two are integrated, such as Prometheus Scrape Config.

(proxy-charm)=
- **Proxy charms** are configurator charms whose configuration is about how to reach a workload that no charm manages, such as Parca Scrape Target.

(integrator-charm)=
- **Integrator charms** are proxy charms whose workload is functionality of a {ref}`cloud <cloud>`, such as the AWS Integrator and the VMware vSphere Integrator.

(podspec-charm)=
- **Podspec charms** created and managed Kubernetes resources that other charms or applications used. Juju 4 no longer supports them, and sidecar charms replace them.

(ops-charm)=
- **Ops charms** are written with the [Ops](https://ops.readthedocs.io/) framework, a Python library over the {ref}`hook commands <hook-command>`. Ops is the recommended way to write a charm.

(12-factor-app-charm)=
- **12-Factor app charms** are created from paired [Rockcraft](https://documentation.ubuntu.com/rockcraft/en/latest/index.html) and [Charmcraft](https://canonical-charmcraft.readthedocs-hosted.com/en/stable/) profiles. The profiles generate the rock (the container image) and the charm for one type of workload, such as a Flask application.

(reactive-charm)=
- **Reactive charms** are written with the [Reactive](https://charmsreactive.readthedocs.io/en/latest/) framework, which Ops superseded.

(bare-charm)=
- **Bare charms** use no framework and implement every {ref}`hook <hook>` by hand, so they are also called hooks-based or hooks-only charms. Ops superseded them.

A charm is a row in the table `charm`. Its metadata is a row in `charm_metadata`. Its relations are rows in `charm_relation`. The other records of a charm are rows in `charm_extra_binding`, `charm_config`, `charm_action`, `charm_resource`, `charm_storage` (with `charm_storage_property`), `charm_device`, `charm_container` (with `charm_container_mount`), `charm_manifest_base`, `charm_hash`, `charm_download_info`, `charm_term`, `charm_tag`, and `charm_category`. The allowed values come from the lookup tables `charm_source`, `charm_relation_role`, `charm_relation_scope`, `charm_config_type`, `charm_resource_kind`, `charm_storage_kind`, `charm_run_as_kind`, `charm_provenance`, `hash_kind`, `os`, and `architecture`. A database trigger refuses to change a row of every table above except `charm`, `charm_download_info`, `charm_tag`, `charm_category`, `charm_storage_property`, and the lookup tables, so that only an insertion or a deletion alters it. The view `v_application_charm_download_info` joins an application to the download information of its charm. {ref}`juju-db-repl` shows the definition of any table with `.ddl <name>`.

(charm-change-streams)=
## Charm change streams

Charm change streams are the notifications that tell the {ref}`workers <model-agent>` of a model that an application needs a charm that the archive of an application's charm is not yet stored. A stream is delivered through a watcher (an API object that tells its holder when something it follows has changed). The stream of applications with a pending charm carries the UUID of an application when a record of the application changes while the application's charm is not available and comes from Charmhub or from a local archive. Its first notification lists every application with a pending charm at that moment. Juju uses it to download charms (see {ref}`downloading pending charms <charm-downloading-pending>`).

(charm-operations)=
## Charm operations

Charm operations are the actions that find, add, upload, list, show, and download charms. They also include the background work that fetches, reserves, and removes charms. A {ref}`client <client>` calls resolving, listing charm resources, getting download information, adding, listing, and showing through the controller API (the set of calls that clients and {ref}`agents <agent>` make to the controller). A client calls uploading and downloading archives through two HTTP routes. A **unit agent** is the process that runs the unit's charm, logged in as its unit and acting only for that unit. A unit agent downloads archives and reads archive checksums. The workers of the {ref}`model agent <model-agent>` (the process that runs the workers of a model) perform the rest. An **origin** is the description of where a charm comes from: its source, channel, base (an operating system and its release), architecture, revision, Charmhub identifier, and hash.

(charm-resolving)=
### Resolving charms

```{ibnote}
See also: {ref}`Juju | Manage charms <manage-charms>`
```

Resolving charms finds, for each reference, the Charmhub revision that matches a requested channel, base, architecture, and revision.

The operation's required access, rules, and errors are:

- *Required access:* {ref}`Read access <user-access-model-read>` to the model. No block applies.
- *Rules:*
  - The call takes a list of references, each with an origin. Each reference in the list gets its own result.
  - A result holds the URL of the charm, its origin, and the bases that the charm supports.
  - Juju asks the Charmhub that the model's `charmhub-url` setting names.
  - A result with the architecture `all` names `amd64`.
  - Only Charmhub charms resolve.
- *Errors:* The client receives each in the result for the reference, with no error code:
  - **`cannot parse charm or bundle URL: "<url>"`**: *Trigger:* The reference is not a charm URL. *Remediation:* Use a charm name, optionally with a revision.
  - **`unknown schema for charm URL "<url>"`**: *Trigger:* The reference has a schema other than Charmhub, such as `local:`. *Remediation:* Use the name of a Charmhub charm.
  - **`resolving with preferred channel: <reason>`**: *Trigger:* Charmhub does not answer, or has no revision that matches. *Remediation:* Read the reason, then relax the channel, base, or architecture.

(charm-listing-resources)=
### Listing charm resources

```{ibnote}
See also: {ref}`Juju | Manage charms <manage-charms>`, {ref}`Juju | Manage charm resources <manage-charm-resources>`
```

Listing charm resources returns the resources that a Charmhub charm releases in a channel.

The operation's required access, rules, and errors are:

- *Required access:* {ref}`Read access <user-access-model-read>` to the model. No block applies.
- *Rules:*
  - The call takes a list of charm URLs with origins. Each entry in the list gets its own list of resources.
  - Only Charmhub charms are listed.
  - An origin with the base `all` is treated as one with no base. An origin with the architecture `all` or no architecture is treated as `amd64`.
  - The listing follows the channel and ignores the revision.
- *Errors:* Each fails the whole call:
  - **`permission denied`**: *Trigger:* The caller lacks read access to the model. The client receives it with the code `unauthorized access`. *Remediation:* Ask for read access.
  - **`charm "<name>" not valid`**: *Trigger:* The URL is not a Charmhub URL. The client receives it with the code `not valid`. *Remediation:* Use the name of a Charmhub charm.
  - **`unable to locate charm "<name>" with matching channel "<channel>"`**: *Trigger:* Charmhub has no revision in the channel. The client receives it with no error code. *Remediation:* Use a channel in which the charm is released.

(charm-getting-download-information)=
### Getting download information

```{ibnote}
See also: {ref}`Juju | Manage charms <manage-charms>`
```

Getting download information returns the URL from which the archive of a Charmhub charm can be fetched, with the charm's origin. The call fetches nothing.

The operation's required access, rules, and errors are:

- *Required access:* {ref}`Read access <user-access-model-read>` to the model. No block applies.
- *Rules:*
  - The call takes a list of charm URLs with origins. Each entry in the list gets its own URL and origin.
  - The origin is normalized as for {ref}`listing charm resources <charm-listing-resources>`.
  - The origin of the result holds the Charmhub identifier and the SHA-256 hash of the archive.
- *Errors:* Each fails the whole call:
  - **`permission denied`**: *Trigger:* The caller lacks read access to the model. The client receives it with the code `unauthorized access`. *Remediation:* Ask for read access.
  - **`cannot parse charm or bundle URL: "<url>"`**: *Trigger:* The entry is not a charm URL. The client receives it with no error code. *Remediation:* Use a charm name.
  - **`<code>: <message>`**: *Trigger:* Charmhub refused the request, where the code and message are Charmhub's. The client receives it with no error code. *Remediation:* Read the message and correct the channel, base, or revision.

(charm-adding)=
### Adding charms

```{ibnote}
See also: {ref}`Juju | Deploy a charm <deploy-a-charm>`
```

Adding charms stores a pending record for a Charmhub charm in the model, so that an application can use it. The archive follows later (see {ref}`downloading pending charms <charm-downloading-pending>`).

The operation's required access, rules, and errors are:

- *Required access:* {ref}`Superuser access <user-access-controller-superuser>` to the controller, or {ref}`write access <user-access-model-write>` to the model. No block applies.
- *Rules:*
  - The origin's source is `charm-hub`.
  - The origin's base has a name and a channel.
  - Juju asks Charmhub for the charm's essential metadata (the metadata, configuration, actions, and manifest) and not for the archive.
  - The record's revision is the revision that Charmhub resolved, or the revision in the URL when Charmhub gave none.
  - The record starts as pending, with the hash from Charmhub and download information with the origin `download`.
  - A charm with the same reference name and revision already stored is left as it is, whatever its source, and the call succeeds.
  - The charm's name is valid (it starts with a letter and has lowercase letters, digits, and single hyphens), its metadata is present, and its manifest lists at least one base. Every base has the operating system `ubuntu`.
  - The relation names of the charm are unique across the endpoints it provides, requires, and peers. Each endpoint has the role of its group. A name or an interface that Juju reserves (the name `juju` and names that start with `juju-`) is allowed on endpoints that a charm provides or peers on only when the charm's name starts with `juju-`. A charm may require an endpoint with a reserved interface, but not one with a reserved name. The container-scoped endpoints that a subordinate charm requires are the exception.
  - Juju adds an endpoint named `juju-info`, with the role `provider`, when the charm declares no endpoint of that name.
  - An architecture of a base that Juju does not support is skipped, and Juju logs the skip at info level.
  - The result is the resolved origin.
- *Errors:* The client receives each with no error code unless stated:
  - **`permission denied`**: *Trigger:* The caller has neither superuser access nor write access. The client receives it with the code `unauthorized access`. *Remediation:* Ask for write access.
  - **`unknown schema for charm URL "<url>"`**: *Trigger:* The origin's source is not `charm-hub`. *Remediation:* Add a Charmhub charm, or {ref}`upload <charm-uploading>` a local one.
  - **`base required for Charmhub charms`**: *Trigger:* The origin's base has no name or no channel. The client receives it with the code `bad request`. *Remediation:* Send both, such as `ubuntu` and `22.04`.
  - **`retrieving essential metadata for charm "<url>": <reason>`**: *Trigger:* Charmhub does not answer, or has no matching revision. *Remediation:* {ref}`Resolve the charm <charm-resolving>` first and use its result.
  - **`setting charm "<url>": <reason>`**: *Trigger:* The charm failed a rule above. The reason is one of `charm metadata not valid`, `charm name not valid`, `charm manifest not found`, `charm manifest not valid`, `charm base name not supported`, `charm relation name conflict`, `charm relation role not valid`, and `charm relation reserved name misuse`, with a detail. *Remediation:* Report the fault to the charm's author.

(charm-uploading)=
### Uploading charms

```{ibnote}
See also: {ref}`Juju | Deploy a charm <deploy-a-charm>`
```

Uploading charms stores the archive of a local charm in the model, with its records, in one request to `/model-<model UUID>/charms/<name>-<SHA-256 prefix>`, so that an application can use it.

The operation's required access, rules, and errors are:

- *Required access:* {ref}`Superuser access <user-access-controller-superuser>` to the controller, or {ref}`write access <user-access-model-write>` to the model. No block applies.
- *Rules:*
  - The request is an HTTP `PUT` with the content type `application/zip` and the header `Juju-Curl`, which holds the charm's URL.
  - The last part of the path, after the last hyphen, is a prefix of the archive's SHA-256 digest at least seven characters long. The text before it is the charm's name.
  - Juju reads at most 500 MiB.
  - An upload that takes longer than five minutes is cancelled.
  - The archive's digest must start with the prefix.
  - The record is available at once, with the source `local`. It has no download information.
  - Juju numbers the revision. The first revision of a name is 0. Each later upload of that name takes the next number.
  - The same rules as for {ref}`adding charms <charm-adding>` apply to the charm's metadata and manifest.
  - The answer is a JSON body with the charm's URL, which has the form `local:<architecture>/<name>-<revision>`, and the same URL in the header `Juju-Curl`.
- *Errors:* The body of the answer holds the message and the code:
  - **`permission denied`**: *Trigger:* The caller has neither superuser access nor write access. The route answers with the HTTP status 403 and a plain-text message that starts `authorization failed:`, with no code. *Remediation:* Ask for write access.
  - **`uploading charm: expected Content-Type: application/zip, got: <type>`**: *Trigger:* The request has another content type. The client receives it with the code `bad request` and the status 400. *Remediation:* Send `application/zip`.
  - **`uploading charm: "<path part>" is not a valid charm object path`** and **`uploading charm: invalid sha length: "<prefix>"`**: *Trigger:* The path has no hyphen, or the prefix is shorter than seven characters. The client receives them with the code `bad request`. *Remediation:* Use `<name>-<prefix>` with at least seven characters of the digest.
  - **`uploading charm: missing "Juju-Curl" header`** and **`uploading charm: "<url>" is not a valid charm url`**: *Trigger:* The header is absent or does not parse. The client receives them with the code `bad request`. *Remediation:* Send a URL such as `local:mycharm`.
  - **`uploading charm: unsupported charm source "<schema>"`**: *Trigger:* The URL's schema is neither `local` nor `ch`. The client receives it with the code `bad request`. *Remediation:* Use `local:`.
  - **`uploading charm: non-local charms may only be uploaded during model migration import`**: *Trigger:* The URL's schema is `ch`. The client receives it with the code `bad request`. *Remediation:* {ref}`Add the charm <charm-adding>` instead.
  - **`uploading charm: resolving uploaded charm: charm hash mismatch`**: *Trigger:* The archive's digest does not start with the prefix in the path. The client receives it with the code `bad request`. *Remediation:* Compute the digest again and use its first characters.
  - **`uploading charm: resolving uploaded charm: <reason>`** and **`uploading charm: setting charm: <reason>`**: *Trigger:* The archive is not a valid charm. The client receives each with the code `bad request`. *Remediation:* Build the archive again.

(charm-listing)=
### Listing charms

```{ibnote}
See also: {ref}`Juju | Manage charms <manage-charms>`
```

Listing charms returns the URLs of the charms that the model has stored.

The operation's required access, rules, and errors are:

- *Required access:* {ref}`Read access <user-access-model-read>` to the model. No block applies.
- *Rules:*
  - A request can name charms. Without names the answer holds every charm. With names it holds only the charms whose reference name is one of them.
  - The answer holds Charmhub and local charms, pending ones included. The stand-in charms of remote applications are left out.
  - A Charmhub URL has the form `ch:<architecture>/<name>-<revision>`.
  - The order of the answer is not defined.
- *Errors:* The client receives each with no error code unless stated:
  - **`permission denied`**: *Trigger:* The caller lacks read access to the model. The client receives it with the code `unauthorized access`. *Remediation:* Ask for read access.
  - **`listing charms: <reason>`**: *Trigger:* The model database could not be read. *Remediation:* Try again, then read the reason in the controller log.

(charm-showing)=
### Showing charms

```{ibnote}
See also: {ref}`Juju | Manage charms <manage-charms>`
```

Showing charms returns what a stored charm declares: its metadata, configuration options, actions, and manifest, with its revision and version.

The operation's required access, rules, and errors are:

- *Required access:* {ref}`Read access <user-access-model-read>` to the model. No block applies.
- *Rules:*
  - The request names the charm by URL. The URL has a revision.
  - Configuration options, actions, and the manifest are left out of the answer when the charm declares none.
- *Errors:* The client receives each in the answer:
  - **`permission denied`**: *Trigger:* The caller lacks read access to the model. The client receives it with the code `unauthorized access`. *Remediation:* Ask for read access.
  - **`charm "<url>" not found`**: *Trigger:* The model has no charm with that source, name, and revision, or the URL has no revision. The client receives it with the code `not found`. *Remediation:* {ref}`List the charms <charm-listing>` and use a URL from the answer.

(charm-downloading)=
### Downloading charm archives

Downloading charm archives streams the archive of a stored charm out of the model's object store, in a request to `/model-<model UUID>/charms/<name>-<SHA-256 prefix>`. The unit agent of a unit uses it to install the unit's charm.

The operation's required access, rules, and errors are:

- *Required access:* Any caller that has authenticated. The route has no further check.
- *Rules:*
  - The request is an HTTP `GET`. The archive is the one whose SHA-256 digest starts with the prefix, which is at least seven characters long. The name before the prefix is not used to find it.
  - The answer holds the header `x-amz-checksum-sha256` with the digest, base64-encoded, so that the caller can verify the archive.
- *Errors:* The body of the answer holds the message and the code:
  - **`retrieving charm: charm not found`**: *Trigger:* No stored archive has a digest that starts with the prefix. The client receives it with the code `not found` and the HTTP status 404. *Remediation:* {ref}`Read the hash <charm-reading-checksums>` of the charm and use its first characters.
  - **`retrieving charm: "<path part>" is not a valid charm object path`** and **`retrieving charm: invalid sha length: "<prefix>"`**: *Trigger:* The path has no hyphen, or the prefix is shorter than seven characters. The client receives them with the code `bad request` and the status 400. *Remediation:* Use `<name>-<prefix>` with at least seven characters of the digest.

(charm-reading-checksums)=
### Reading charm archive checksums

Reading charm archive checksums returns the SHA-256 digest of the archive of each charm that is available. A unit agent reads it before it downloads the archive.

The operation's required access, rules, and errors are:

- *Required access:* A unit agent, for its own unit.
- *Rules:*
  - The call takes a list of charm URLs. Each URL in the list gets its own digest or error.
  - A charm that is still pending has no digest.
- *Errors:* The unit agent receives each in the result for the URL:
  - **`charm "<url>" not found`**: *Trigger:* The model has no charm with that source, name, and revision. The unit agent receives it with the code `not found`. *Remediation:* Use the URL that {ref}`showing charms <charm-showing>` gives.
  - **`charm "<url>" not available`**: *Trigger:* The charm is pending. The unit agent receives it with the code `not yet available; try again later`. *Remediation:* Try again, because the download of the charm is still under way.

(charm-downloading-pending)=
### Downloading pending charms

Downloading pending charms fetches the archive of every pending Charmhub charm that an application uses, and makes the charm available. The workers of the {ref}`model agent <model-agent>` perform it, one for each application, when the application with a pending charm appears in the change stream.

The operation's required access, rules, and errors are:

- *Required access:* The workers of the model agent.
- *Rules:*
  - A worker reads the download URL and the hash from the charm's records. It downloads the archive to a temporary file, with up to three attempts 20 seconds apart and five minutes for each.
  - The download is accepted when its SHA-256 digest equals the stored hash.
  - Juju puts the archive in the model's object store and then sets the charm's archive path and object store entry and makes it available. A charm becomes available once and does not return to pending.
  - A charm that is already available is left as it is.
  - The temporary file is deleted afterward.
- *Errors:* The worker logs each and retries the download up to three times, 20 seconds apart. The charm stays pending until a download succeeds:
  - **`invalid origin hash: "<expected>", got "<actual>"`**: *Trigger:* The digest of the download differs from the hash that Charmhub gave. *Remediation:* Wait for the next attempt, then {ref}`resolve the charm <charm-resolving>` again if it keeps failing.
  - **`charm hash mismatch`** and **`charm not resolved`**: *Trigger:* The charm changed during the download. *Remediation:* None: the worker tries again.
  - **`charm already exists with different size`**: *Trigger:* The object store holds an archive with the same hash and another size. *Remediation:* Report it to Juju developers.

(charm-reserving-revisions)=
### Reserving charm revisions

Reserving charm revisions adds a pending record for each newer revision that Charmhub has released for the charm of an application. The workers of the model agent perform it about every 24 hours.

The operation's required access, rules, and errors are:

- *Required access:* The workers of the model agent.
- *Rules:*
  - A worker asks Charmhub about every Charmhub application of the model, with the application's revision, channel, base, and architecture.
  - The record is pending. Its source is `charmhub`. Its revision, hash, and download information come from Charmhub's answer, and the download information has the origin `download`.
  - A revision that the model already has is left as it is.
  - The worker also stores the latest revision of the resources of the charm.
  - Status shows the newest pending revision of a charm, one that no application uses yet, as a revision that an application of that charm can upgrade to.
  - The interval is 24 hours and an agent setting can change it.
- *Errors:* The worker logs each and tries again at the next interval:
  - **`sending metrics failed`**: *Trigger:* Charmhub did not accept the usage metrics. *Remediation:* None: the worker tries again.
  - **`failed to fetch revisions: <reason>`** and **`failed to store revisions: <reason>`**: *Trigger:* Charmhub did not answer, or the model database could not be written. *Remediation:* None: the worker tries again at the next interval.

(charm-removing)=
### Removing charms

Removing charms deletes a charm's records when nothing uses it. It happens when the removal of an application or a unit finishes.

The operation's required access and rules are:

- *Required access:* The workers of the model agent, which remove the application or unit.
- *Rules:*
  - Juju deletes the charm only when no application and no unit points at it. Otherwise it keeps the charm and logs the number of uses.
  - The deletion removes the records about the charm, then the charm itself.
  - The charm's archive is deleted from the object store when no other charm and no resource file uses it.

(charm-and-model-migration)=
## Charms and model migration

A migration does not move the charm hashes. It moves the other charm records, then uploads each charm's archive to the target controller, in ascending order of charm URL, so that revisions match. The source sends each archive to `/migrate/charms/<name>-<SHA-256 prefix>` with the header `Juju-Curl`, which needs {ref}`superuser access <user-access-controller-superuser>` to the target controller. The target finds the charm record that the migration already created, then stores the archive, computes the hash, and makes the charm available, with download information that has the origin `migration`. The target accepts a charm URL from Juju 3, which has no architecture. See {ref}`Migrating models <model-migrating>`.

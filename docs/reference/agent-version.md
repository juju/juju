---
myst:
  html_meta:
    description: "Juju agent version reference: the target version and stream a model's agents are told to run, the catalogue of stored agent binaries, and the calls that upload the binaries and hand them to the agents."
---

(agent-version)=
# Agent version

```{ibnote}
See also: {ref}`Juju | Upgrade a model <upgrade-a-model>`
```

The **agent version** of a {ref}`model <model>` is the version of Juju that the model's {ref}`agents <agent>` are told to run, together with the agent stream they take it from. The model database keeps one record, which also names the newest version the controller has seen. The binaries themselves are gzipped tarballs in the {ref}`object store <object-store>` of the controller or of the model whose catalogue lists them; the controller database and each model database keep a catalogue of the binaries they hold. A client sets the target version by {ref}`upgrading models <model-upgrading>`.

(agent-version-database-schema)=
## Agent version database schema

```{ggarch}
:file: ../juju.ggarch
:view: Agent version database schema
:caption: The model database keeps one agent version record, which names one agent stream. The controller database and each model database keep one record for each agent binary they hold: its version, its architecture, and the stored object it is (a stored object is one binary). A binary belongs to one stored object and names one architecture; the catalogue of the controller can be read by every model. The stored object and the architecture are described elsewhere and the figure shows them only for context: all boxes are records, and the two with a second, outlined arrow mark in the corner are the records that another page describes or that belong to no page.
:alt: Two groups of boxes of the same size, each box with a filled table mark in the corner. The group named Model database holds Agent version and Agent stream in a row, joined 1 to 1. The group named Controller database and model databases holds Agent binary, Architecture, and Stored object in a row; Agent binary joins Architecture (m to 1) and Stored object (m to 1). Architecture and Stored object have an outlined arrow mark for records described elsewhere.
```

The agent version database schema is the set of records that the databases keep about the versions the agents run. The records, with the rules that apply to each, are:

- **Agent version:** The target version (the version the model's agents should run) and the latest version (the newest one the controller has seen on its metadata source), with the stream the agents take their binaries from.
  - *Rules:*
    - A model keeps one agent version record.
- **Agent stream:** The stream the agents of a model take their binaries from: `released`, `proposed`, `testing`, or `devel`.
  - *Rules:*
    - A stream name appears once.
- **Agent binary:** One stored agent binary: its version and its architecture.
  - *Rules:*
    - A catalogue holds each version and architecture once.
- **Architecture and the stored object:** What a binary is built for and where it is stored. The stored object is described on the {ref}`object store <object-store>` page; the architectures are the shared names of the platforms Juju builds for.

(agent-version-change-streams)=
## Agent version change streams

Agent version change streams are the notifications that tell the agents of a model that the target version has changed. A machine agent calls `WatchAPIVersion` for its machine; a {ref}`unit agent <unit-agent>` calls it for its unit. When the watcher fires, the agent asks for the binaries of the new target version and swaps its own binary for one of them.

(agent-version-calls)=
## Agent version calls

Agent version calls are the calls that put agent binaries on the controller and hand them to the agents. A client, for a user with admin access, uploads binaries; an agent asks for the binaries of its target version. Setting the target version itself is part of {ref}`upgrading models <model-upgrading>`, which also states the checks a version must pass.

(agent-version-uploading)=
### Uploading agent binaries

Uploading agent binaries puts one binary on the controller, as a gzipped tarball, for one version and one architecture. A binary that the controller already holds with the same hash is left as it is.

The caller, the rules, and the errors are:

- *Caller:* A client, for a user with {ref}`superuser access <user-access-controller-superuser>` to the controller, or with {ref}`admin access <user-access-model-admin>` to the model the request names. A request naming the controller model needs superuser access. A change block on that model refuses the upload.
- *Rules:*
  - The request is a `POST` of the tarball with the content type `application/x-tar-gz`, and names the binary version in the query parameter `binaryVersion`, in the form `<version>-<series>-<architecture>`, such as `4.5.0-noble-amd64`.
  - The handler keeps the number and the architecture of the version; the series in between is ignored.
  - The tarball is stored in the object store of the model the request names, and added to that model's catalogue. A request naming the controller model stores the binary in the controller's object store and catalogue, which every model can use.
  - A binary that the catalogue already holds with the same hash is left as it is.
- *Errors:* The client receives each with the HTTP status 400:
  - **`expected binaryVersion argument`**: *Trigger:* The request names no binary version. *Remediation:* Name the version in the query parameter `binaryVersion`.
  - **`invalid agent binary version "<version>"`**: *Trigger:* The named version cannot be read as a version, a series, and an architecture. *Remediation:* Send the version in the form `<version>-<series>-<architecture>`.
  - **`agent binary version "<version>" is not valid`**: *Trigger:* The version has a zero number, or an architecture Juju does not know. *Remediation:* Send a supported version and architecture.
  - **`unsupported architecture "<architecture>"`**: *Trigger:* The architecture is not one Juju knows. *Remediation:* Use a supported architecture.
  - **`no agent binaries uploaded`**: *Trigger:* The request body is empty. *Remediation:* Send the tarball as the body.
  - **`expected Content-Type: application/x-tar-gz, got: <type>`**: *Trigger:* The request has another content type. *Remediation:* Send the tarball with the content type `application/x-tar-gz`.

(agent-version-finding)=
### Finding agent binaries

Finding agent binaries returns to an agent the binaries it should run: those of the target version of its machine or unit, for its architecture, with the URL to download each from. A machine agent asks for its machine; a unit agent for its unit.

The caller, the rules, and the errors are:

- *Caller:* A machine agent, for its own machine, or a unit agent, for its own unit.
- *Rules:*
  - Every URL points at one of the controller's API servers. A binary that no catalogue holds is fetched from the metadata source when the agent downloads it.
- *Errors:* The agent receives each in the result for its entity:
  - **`permission denied`**: *Trigger:* The entity tag cannot be read, or the caller is not the agent of the entity it names. The client receives it with the code `unauthorized access`. *Remediation:* Send the tag of the agent's own machine or unit.
  - **`"<machine or unit>" not found`** and **`target agent version for <machine or unit> not found`**: *Trigger:* No such machine or unit exists, or it has no target version recorded. The client receives them with the code `not found`. *Remediation:* Send the tag of the agent's own machine or unit; set the model's target agent version first for the second.

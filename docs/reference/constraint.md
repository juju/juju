---
myst:
  html_meta:
    description: "Juju constraint reference: what a constraint is, the records that keep it, the constraint keys, and how constraints combine, and the operations that set and read the constraints of models, applications, and machines."
---

(constraint)=
# Constraint

```{ibnote}
See also: {ref}`Juju | Manage constraints for a model <manage-constraints-for-a-model>`, {ref}`Juju | Manage constraints for an application <manage-constraints-for-an-application>`
```

A **constraint** is a set of requirements on the {ref}`compute resources <resource-compute>` that Juju requests from a {ref}`cloud <cloud>`: the architecture, the cores, the memory, the root disk, and the other keys listed below. A {ref}`model <model>`, an {ref}`application <application>`, and a {ref}`machine <machine>` can each hold one set, and the model database keeps all of them. Each call that sets a holder's requirements replaces the whole set.

(constraint-database-schema)=
## Constraint database schema

```{ggarch}
:file: ../juju.ggarch
:view: Constraint database schema
:caption: The model database keeps one record for each constraint. A constraint has many tag entries, space entries, and zone entries. Each entry belongs to one constraint. A space entry names one space. A model, an application, and a machine each hold at most one constraint. The model, the application, the machine, and the space are described on their own pages and the figure shows them only for context: all boxes are records, and the four with a second, outlined arrow mark in the corner are the records that other pages describe.
:alt: Eight boxes of the same size inside a group named Model database, each with a filled table mark in the corner. Constraint is in the middle. Application constraint is above it (1 to 1), Model constraints is above and to its left (1 to 1), and Machine constraint is above and to its right (1 to 1); all three have an outlined arrow mark. Space entry is below Constraint (1 to m), Tag entry is below and to its left (1 to m), and Zone entry is below and to its right (1 to m). Space is below Space entry (m to 1) and has an outlined arrow mark.
```

The constraint database schema is the set of records that the {ref}`model database <database>` keeps about constraints. The records, with the rules that apply to each, are:

- **Constraint:** One holder's set of requirements, with one column for each key below. Every column is optional: a key that the holder did not set is empty.
  - *Rules:*
    - The container type is one of the values of the lookup table `container_type`.
- **Tag entry:** One machine tag that the holder named, either as an attribute the machine must have, or, when the entry is marked as excluded, as one it must not have.
  - *Rules:*
    - A constraint has each tag once.
- **Space entry:** One {ref}`network space <space>` that the holder named. The entry is positive or excluded, as for a tag entry.
  - *Rules:*
    - A constraint has each space once.
    - A space entry names a space that exists in the model.
- **Zone entry:** One availability {ref}`zone <zone>` that the holder named.
  - *Rules:*
    - A constraint has each zone once.
- **Model, application, and machine:** The holders, each with at most one constraint. Other pages own them: see {ref}`model <model>`, {ref}`application <application>`, and {ref}`machine <machine>`.

(list-of-constraints)=
## List of constraints

A client writes a constraint as `key=value` pairs separated by spaces, such as `mem=4G cores=2`, and the controller API takes the same keys. A key appears once in a constraint. An empty value clears the key. The keys, with the value each takes, are listed below. A cloud accepts only some of the keys and only some of the values; the page of each {ref}`cloud <list-of-supported-clouds>` records what it accepts. On Kubernetes the keys map onto the pod: `mem` and `cpu-power` become the pod's resource requests, `arch` its node selector, and `tags` its affinity rules.

(constraint-allocate-public-ip)=
### `allocate-public-ip`

Whether the machine gets an address outside the cloud's virtual network. The value is `true` or `false`.

(constraint-arch)=
### `arch`

The machine architecture. Valid values: `amd64`, `arm64`, `ppc64el`, `s390x`, `riscv64`.

(constraint-container)=
### `container`

The container type the machine must be. Valid values: `lxd` and `none`. Kubernetes does not accept this key.

(constraint-cores)=
### `cores`

The number of CPU cores. The value is an integer. The key has the alias `cpu-cores`; setting both the key and the alias is refused.

(constraint-cpu-power)=
### `cpu-power`

An abstract measure of CPU power. The value is an integer; on Kubernetes a unit is a millicore. Not every cloud accepts the key, so use `cores` where a charm must run anywhere.

(constraint-image-id)=
### `image-id`

The image the machine must use. The value is the cloud's identifier of an image, so it is cloud-specific.

(constraint-instance-role)=
### `instance-role`

The cloud role the machine must run with: an EC2 instance profile, an Azure managed identity.

(constraint-instance-type)=
### `instance-type`

The cloud's name of an instance type. Which names exist is cloud-specific, so use the `cores`, `mem`, and `root-disk` keys where a charm must run anywhere.

(constraint-ip-family)=
### `ip-family`

The IP address family for the machine's network interfaces. Valid values: `ipv4`, `ipv6`, `dual`. Azure accepts `ipv4` and `dual`.

(constraint-mem)=
### `mem`

The memory, in MiB. A suffix `M`, `G`, `T`, or `P` multiplies the value by 1, 1024, 1024 squared, or 1024 cubed.

(constraint-root-disk)=
### `root-disk`

The space on the root drive, in MiB, with the same suffixes as `mem`. Storage attached to the machine later does not count toward it.

(constraint-root-disk-source)=
### `root-disk-source`

The storage pool or location that the root disk comes from. The value and its effect are cloud-specific.

(constraint-spaces)=
### `spaces`

The network spaces the machine must reach, as a list. A name with the prefix `^` excludes the space. A space must exist in the model. Kubernetes does not accept this key.

(constraint-tags)=
### `tags`

The tags of the machine, as a list. A tag with the prefix `^` excludes it.

(constraint-virt-type)=
### `virt-type`

The virtualization type. Which values exist is cloud-specific.

(constraint-zones)=
### `zones`

The availability zones, as a list; the machine is created in one of them.

(constraint-merging)=
## Constraint merging

Constraint merging combines the constraints that apply to one machine into one set, at the moment Juju creates the machine:

- For a unit, Juju starts with the model's constraints and lets the application's constraints replace them key by key. When the result has no `arch`, Juju sets `amd64`.
- For a machine that a client adds, Juju starts with the model's constraints and lets the machine's constraints replace them key by key. The `container` key is dropped from both.
- The cloud can register keys that conflict, such as `instance-type` with `arch`, `mem`, `cores`, and `cpu-power` on EC2. A key from the more specific holder removes the conflicting keys of the model.
- A machine keeps the merged result. Changing a constraint afterwards affects only machines that Juju creates later.

(constraint-operations)=
## Constraint operations

Constraint operations are the actions a {ref}`client <client>` can take on constraints through the controller API (the set of calls that clients and {ref}`agents <agent>` make to the controller): set the constraints of a model, get them, set those of an application, get them, give those of a machine, and read them. Each call acts on one holder. Deploying an application with constraints is described on the {ref}`application <application>` page.

(constraint-model-setting)=
### Setting model constraints

```{ibnote}
See also: {ref}`Juju | Manage constraints for a model <manage-constraints-for-a-model>`
```

Setting model constraints stores the constraints of a model, replacing the whole set. A client that bootstraps a controller sets the constraints of the controller model with the bootstrap constraints and those of the controller application with the bootstrap machine constraints.

The operation's required access, rules, and errors are:

- *Required access:* {ref}`Write access <user-access-model-write>` to the model. The change block (see {ref}`restricting commands on a model <restrict-commands-on-a-model>`) must not be on.
- *Rules:*
  - The old tags, spaces, and zones are removed and the request's are stored.
  - A space the request names must exist in the model.
  - The container type must be `lxd` or `none`.
  - The cloud's list of unsupported or conflicting keys is not applied here, so a key that the cloud refuses surfaces when Juju merges.
- *Errors:*
  - **`space not found for model constraint "<model uuid>": <reason>`**: *Trigger:* A named space does not exist. The client receives it with the code `not found`. *Remediation:* {ref}`Add the space <add-a-space>` first.
  - **`invalid container type for model constraint "<model uuid>"`**: *Trigger:* The container type is not `lxd` or `none`. The client receives it with the code `not valid`. *Remediation:* Use `lxd` or `none`.
  - **`model "<model uuid>" not found`**: *Trigger:* The model does not exist. The client receives it with the code `model not found`. *Remediation:* Use an existing model.
  - **`permission denied`**: *Trigger:* The caller lacks write access. The client receives it with the code `unauthorized access`. *Remediation:* Ask a model administrator.
  - **`the operation has been blocked`** (or the message that the block carries): *Trigger:* The change block is on. The client receives it with the code `operation is blocked`. *Remediation:* Ask the operator to lift the block.

(constraint-model-getting)=
### Getting model constraints

Getting model constraints returns the constraints of a model. A model without constraints gets an empty result.

The operation's required access, rules, and errors are:

- *Required access:* {ref}`Read access <user-access-model-read>` to the model, or {ref}`admin access <user-access-model-admin>`, or {ref}`superuser access <user-access-controller-superuser>` to the controller. No block applies.
- *Errors:*
  - **`model "<model uuid>" not found`**: *Trigger:* The model does not exist. The client receives it with the code `model not found`. *Remediation:* Use an existing model.
  - **`permission denied`**: *Trigger:* The caller lacks the access above. The client receives it with the code `unauthorized access`. *Remediation:* Ask a model administrator.

(constraint-application-setting)=
### Setting application constraints

```{ibnote}
See also: {ref}`Juju | Manage constraints for an application <manage-constraints-for-an-application>`
```

Setting application constraints stores the constraints of one application, replacing the whole set: the tags, spaces, and zones of the old set are removed, and a key left unset loses its value.

The operation's required access, rules, and errors are:

- *Required access:* {ref}`Write access <user-access-model-write>` to the model. The change block (see {ref}`restricting commands on a model <restrict-commands-on-a-model>`) must not be on.
- *Rules:*
  - The application must exist.
  - The container type must be `lxd` or `none`. A space the request names must exist in the model.
  - The cloud checks the values and the conflicts between keys. A key that the cloud does not support is logged and stored, not refused.
  - A subordinate application takes no constraints at deployment.
- *Errors:*
  - **`invalid application constraints`**: *Trigger:* The container type is not known, or a named space does not exist. The client receives it with no error code. *Remediation:* Use `lxd` or `none`, or {ref}`add the space <add-a-space>` first.
  - **`invalid constraint value: <key>=<value>` followed by `valid values are: <values>`** and **`ambiguous constraints: "<key>" overlaps with "<key>"`**: *Trigger:* The cloud refuses a value, or two keys conflict. The client receives them with no error code. *Remediation:* Use a listed value, or drop one of the two keys.
  - **`application <name> not found`**: *Trigger:* The application does not exist. The client receives it with the code `not found`. *Remediation:* Use an existing application.
  - **`application is dead`**: *Trigger:* The application is dead. The client receives it with no error code. *Remediation:* Use an existing application.
  - **`permission denied`**: *Trigger:* The caller lacks write access. The client receives it with the code `unauthorized access`. *Remediation:* Ask a model administrator.
  - **`the operation has been blocked`** (or the message that the block carries): *Trigger:* The change block is on. The client receives it with the code `operation is blocked`. *Remediation:* Ask the operator to lift the block.

(constraint-application-getting)=
### Getting application constraints

Getting application constraints returns the constraints of one or more applications. An application without constraints gets an empty result.

The operation's required access, rules, and errors are:

- *Required access:* {ref}`Read access <user-access-model-read>` to the model. No block applies.
- *Rules:*
  - The call takes a list of applications. Each application in the list gets its own result.
- *Errors:* A refused read fails the whole call with `permission denied` (code `unauthorized access`). The client receives the others in the result for the application:
  - **`application <name> not found`**: *Trigger:* The application does not exist. The client receives it with the code `not found`. *Remediation:* Use an existing application.
  - **`"<tag>" is not a valid tag`**: *Trigger:* The request's tag is malformed. The client receives it with no error code. *Remediation:* Tag an application.
  - **`unexpected tag type, expected application, got <kind>`**: *Trigger:* The tag is of another kind. The client receives it with no error code. *Remediation:* Tag an application.

(constraint-machine-setting)=
### Setting machine constraints

Setting machine constraints gives a machine its constraints when a client adds the machine (see {ref}`adding machines <machine-adding>`). No call changes them afterwards.

The operation's required access, rules, and errors are:

- *Required access:* {ref}`Write access <user-access-model-write>` to the model. The change block must not be on.
- *Rules:*
  - Juju merges the request's constraints with the model's (see {ref}`constraint merging <constraint-merging>`) and stores the result with the machine.
  - The cloud then prechecks the merged constraints.
- *Errors:* The client receives each in the result for the machine:
  - **`merging machine and model constraints: <reason>`**: *Trigger:* The cloud refuses a value, or two keys conflict. The client receives it with no error code. *Remediation:* Use a value that the cloud lists, or drop one of the conflicting keys.
  - **`prechecking instance for create machine: <reason>`**: *Trigger:* The cloud refuses the merged constraints. The client receives it with no error code. *Remediation:* Follow the reason.

(constraint-machine-getting)=
### Getting machine constraints

Getting machine constraints shows the constraints of each machine in the model's status.

The operation's required access, rules, and errors are:

- *Required access:* {ref}`Read access <user-access-model-read>` to the model, or superuser access to the controller. A machine agent or the controller reads the constraints of a machine to provision it.

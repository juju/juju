---
myst:
  html_meta:
    description: "Juju constraint reference: what a constraint is, the records that keep it, the constraint keys, and the operations that set and read the constraints of an application."
---

(constraint)=
# Constraint

```{ibnote}
See also: {ref}`Juju | Manage constraints for a model <manage-constraints-for-a-model>`, {ref}`Juju | Manage constraints for an application <manage-constraints-for-an-application>`
```

A **constraint** is a set of requirements on the {ref}`compute resources <resource-compute>` that Juju requests from a {ref}`cloud <cloud>`: the architecture, the cores, the memory, the root disk, and the other keys listed below. A {ref}`model <model>`, an {ref}`application <application>`, and a {ref}`machine <machine>` can each hold one set, and the model database keeps all of them. A client sets the requirements and replaces them whole.

(constraint-data-model)=
## Constraint data model

```{ggarch}
:file: ../juju.ggarch
:view: Constraint data model
:caption: The model database keeps one record for each constraint. A constraint has many tag entries, space entries, and zone entries. Each entry belongs to one constraint. A space entry names one space. A model, an application, and a machine each hold at most one constraint. The model, the application, the machine, and the space are described on their own pages and the figure shows them only for context: all boxes are records, and the four with a second, outlined arrow mark in the corner are the records that other pages describe.
:alt: Eight boxes of the same size inside a group named Model database, each with a filled table mark in the corner. Constraint is in the middle. Application constraint is above it (1 to 1), Model constraint is above and to its left (1 to 1), and Machine constraint is above and to its right (1 to 1); all three have an outlined arrow mark. Space entry is below Constraint (1 to m), Tag entry is below and to its left (1 to m), and Zone entry is below and to its right (1 to m). Space is below Space entry (m to 1) and has an outlined arrow mark.
```

The constraint data model is the set of records that the {ref}`model database <database>` keeps about constraints. The records, with the rules that apply to each, are:

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

A constraint is a row in the table `constraint`. Its tags, spaces, and zones are rows in `constraint_tag`, `constraint_space`, and `constraint_zone`. The view `v_constraint` shows each constraint with the name of its container type. Each holder's constraint is a row in `model_constraint`, `application_constraint`, or `machine_constraint`. {ref}`juju-db-repl` shows the definition of any of them with `.ddl <name>`.

(list-of-constraints)=
## List of constraints

The keys of a constraint, with the value each takes, are listed below. A cloud accepts only some of the keys and only some of the values; the page of each {ref}`cloud <list-of-supported-clouds>` records what it accepts. On Kubernetes the keys map onto the pod: `mem` and `cpu-power` become the container's resource request and limit, `arch` its node selector, and `tags` its affinity rules.

(constraint-allocate-public-ip)=
### `allocate-public-ip`

Whether the machine gets an address outside the cloud's virtual network. The value is `true` or `false`.

(constraint-arch)=
### `arch`

The machine architecture. Valid values: `amd64`, `arm64`, `ppc64el`, `s390x`, `riscv64`.

(constraint-container)=
### `container`

The container type the machine must be. Valid value: `lxd`. Kubernetes does not accept this key.

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

(constraint-operations)=
## Constraint operations

Constraint operations are the actions a {ref}`client <client>` can take on constraints through the controller API (the set of calls that clients and {ref}`agents <agent>` make to the controller): setting the constraints of an application and reading them. The constraints of a model are set and read as part of its configuration (see {ref}`configuring models <model-configuring>`). The constraints of a machine are given when the machine is added (see {ref}`adding machines <machine-adding>`). When Juju places a unit or creates a machine, it merges the holder's constraints with the model's, and the holder's values win.

(constraint-application-setting)=
### Setting application constraints

```{ibnote}
See also: {ref}`Juju | Manage constraints for an application <manage-constraints-for-an-application>`
```

Setting application constraints stores the constraints of one application, replacing the whole set: the tags, spaces, and zones of the old set are removed. A key left unset loses its value.

The operation's required access, rules, and errors are:

- *Required access:* {ref}`Write access <user-access-model-write>` to the model. The change block (see {ref}`restricting commands on a model <restrict-commands-on-a-model>`) must not be on.
- *Rules:*
  - The application must exist.
  - The container type must be one of the lookup table's values. A space the request names must exist in the model.
  - A key that the model's cloud does not support is logged and stored, not refused.
- *Errors:*
  - **`invalid application constraints`**: *Trigger:* The container type is not one Juju knows, or a named space does not exist in the model. The client receives it with no error code. *Remediation:* Use the container type `lxd`, or {ref}`add the space <add-a-space>` first.
  - **`application "<name>" not found`**: *Trigger:* The application does not exist. The client receives it with the code `not found`. *Remediation:* Use an existing application.
  - **`application is dead`**: *Trigger:* The application is dead, after its removal finished. The client receives it with no error code. *Remediation:* The application is gone; use an existing one.
  - **`permission denied`**: *Trigger:* The caller lacks the access above. The client receives it with the code `unauthorized access`. *Remediation:* Ask a model administrator.
  - **`the operation has been blocked`** (or the message that the block carries): *Trigger:* The change block is on. The client receives it with the code `operation is blocked`. *Remediation:* Remove the block.

(constraint-application-getting)=
### Getting application constraints

Getting application constraints returns the constraints of one or more applications. An application without constraints gets an empty result.

The operation's required access, rules, and errors are:

- *Required access:* {ref}`Read access <user-access-model-read>` to the model. No block applies.
- *Rules:*
  - The call takes a list of applications. Each application in the list gets its own result.
- *Errors:* The client receives each in the result for the application:
  - **`application "<name>" not found`**: *Trigger:* The application does not exist. The client receives it with the code `not found`. *Remediation:* Use an existing application.
  - **`application is dead`**: *Trigger:* The application is dead, after its removal finished. The client receives it with no error code. *Remediation:* The application is gone; use an existing one.
  - **`"<tag>" is not a valid tag`**: *Trigger:* The request's application tag is not a valid tag. The client receives it with no error code. *Remediation:* Tag an application.
  - **`unexpected tag type, expected application, got <kind>`**: *Trigger:* The request's tag is of another kind. The client receives it with no error code. *Remediation:* Tag an application.

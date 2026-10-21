---
myst:
  html_meta:
    description: "Juju model defaults reference: the default model configuration that a cloud and each of its regions carry, the records that keep it, and how a new model takes its values from them."
---

(model-defaults)=
# Model defaults

```{ibnote}
See also: {ref}`Juju | Configure a model <configure-a-model>`
```

A **model default** is the value of a {ref}`model configuration <model-configuration>` key that a new {ref}`model <model>` starts with, set for every model of a {ref}`cloud <cloud>` or for the models of one of its regions. The {ref}`controller database <database>` keeps them, one key and value per cloud and per region. A {ref}`superuser <user-access-controller-superuser>` sets them, as part of {ref}`configuring models <model-configuring>`.

(model-defaults-data-model)=
## Model defaults data model

```{ggarch}
:file: ../juju.ggarch
:view: Model defaults data model
:caption: The controller database keeps one default record for each key of a cloud, and one for each key of one of its regions. A cloud has many defaults, and a default belongs to one cloud. A cloud has many regions, and a region belongs to one cloud; a region has many defaults, and a default belongs to one region. The cloud and the region are described on their own pages and the figure shows them only for context: all boxes are records, and the two with a second, outlined arrow mark in the corner are the records that other pages describe.
:alt: Four boxes of the same size in a row inside a group named Controller database, each with a filled table mark in the corner. Cloud default is on the left, Cloud is next to it (m to 1), then Region (1 to m), and Region default is on the right (m to 1). Cloud and Region have an outlined arrow mark for records that other pages describe.
```

The model defaults data model is the set of records that the controller database keeps about the default {ref}`model configuration <model-configuration>` of a {ref}`cloud <cloud>` and of its {ref}`regions <cloud-data-model>`. The records, with the rules that apply to each, are:

- **Cloud default:** One key of the model configuration and the value that every new model of the cloud starts with for that key.
  - *Rules:*
    - A cloud has each key once.
    - A key is not empty.
- **Region default:** One key of the model configuration and the value that every new model of one region of the cloud starts with for that key.
  - *Rules:*
    - A region has each key once.
    - A key is not empty.
- **Cloud and cloud region:** The records at the far ends of the defaults. Other pages own them: see {ref}`cloud <cloud>` and the {ref}`regions <cloud-data-model>` of a cloud.

A cloud default is a row in the table `cloud_defaults`. A region default is a row in `cloud_region_defaults`. {ref}`juju-db-repl` shows the definition of either with `.ddl <name>`.

(model-defaults-operations)=
## Model default operations

Model default operations are part of {ref}`configuring models <model-configuring>`: a client reads the defaults of a cloud, sets them, and removes them, for a {ref}`cloud <cloud>` or for one of its {ref}`regions <cloud-data-model>`.

When a {ref}`model <model>` is created, its {ref}`model configuration <model-configuration>` takes each key's value from the most specific source that sets it: the defaults built into this version of Juju come first, then the defaults of the cloud's provider (the part of Juju that talks to the cloud's API), the cloud's defaults, the region's defaults, and finally the model's fixed metadata, which no one can change. A new model takes its values from the defaults at the moment it is created; a change later on does not reach models that exist ({ref}`adding a model <add-a-model>`). The default values of each key are listed with the key in the {ref}`list of model configuration keys <list-of-model-configuration-keys>`.

(model-defaults-reading)=
### Reading model defaults

Reading model defaults lists the default value of each {ref}`model configuration <model-configuration>` key that a {ref}`cloud <cloud>` carries, together with the value that one of its {ref}`regions <cloud-data-model>` carries for the same key.

The operation's required access, rules, and errors are:

- *Required access:* {ref}`Superuser access <user-access-controller-superuser>` to the controller.
- *Rules:*
  - Each key's result carries the cloud's default value and, for each region that has one, the region's value for the key.
- *Errors:*
  - **`permission denied`**: *Trigger:* The caller is not a superuser. The {ref}`client <client>` receives it with the code `unauthorized access`. *Remediation:* Ask a superuser.
  - **`cloud "<name>" not found`**: *Trigger:* The request names a cloud that does not exist. The {ref}`client <client>` receives it with the code for a missing entity. *Remediation:* Use the name of a cloud that exists.

(model-defaults-setting)=
### Setting model defaults

Setting model defaults writes the value of one or more {ref}`model configuration <model-configuration>` keys for a {ref}`cloud <cloud>` or for one of its {ref}`regions <cloud-data-model>`.

The operation's required access, rules, and errors are:

- *Required access:* {ref}`Superuser access <user-access-controller-superuser>` to the controller.
- *Rules:*
  - A change block on the controller model (see {ref}`restricting commands on a model <restrict-commands-on-a-model>`) refuses a set.
  - Setting a key that the cloud or the region already carries replaces its value.
- *Errors:*
  - **`permission denied`**: *Trigger:* The caller is not a superuser. The {ref}`client <client>` receives it with the code `unauthorized access`. *Remediation:* Ask a superuser.
  - **`cloud "<name>" not found`**: *Trigger:* The request names a cloud that does not exist. The {ref}`client <client>` receives it with the code for a missing entity. *Remediation:* Use the name of a cloud that exists.
  - **`cloud "<name>" region "<name>" not found`**: *Trigger:* The request names a region that the cloud does not have. The {ref}`client <client>` receives it with the code for a missing entity. *Remediation:* Use the name of a region of the cloud.

(model-defaults-removing)=
### Removing model defaults

Removing model defaults removes the default value of one or more {ref}`model configuration <model-configuration>` keys from a {ref}`cloud <cloud>` or from one of its {ref}`regions <cloud-data-model>`.

The operation's required access, rules, and errors are:

- *Required access:* {ref}`Superuser access <user-access-controller-superuser>` to the controller.
- *Rules:*
  - A change block on the controller model (see {ref}`restricting commands on a model <restrict-commands-on-a-model>`) refuses a removal.
- *Errors:*
  - **`permission denied`**: *Trigger:* The caller is not a superuser. The {ref}`client <client>` receives it with the code `unauthorized access`. *Remediation:* Ask a superuser.
  - **`cloud "<name>" not found`**: *Trigger:* The request names a cloud that does not exist. The {ref}`client <client>` receives it with the code for a missing entity. *Remediation:* Use the name of a cloud that exists.
  - **`cloud "<name>" region "<name>" not found`**: *Trigger:* The request names a region that the cloud does not have. The {ref}`client <client>` receives it with the code for a missing entity. *Remediation:* Use the name of a region of the cloud.

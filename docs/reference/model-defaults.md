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

The model defaults data model is the set of records that the controller database keeps about the default model configuration of a cloud and of its regions. The records, with the rules that apply to each, are:

- **Cloud default:** One key of the model configuration and the value that every new model of the cloud starts with for that key.
  - *Rules:*
    - A cloud has each key once.
    - A key is not empty.
- **Region default:** One key of the model configuration and the value that every new model of one region of the cloud starts with for that key. A region's value takes the place of the cloud's value for the same key.
  - *Rules:*
    - A region has each key once.
    - A key is not empty.
- **Cloud and cloud region:** The records at the far ends of the defaults. Other pages own them: see {ref}`cloud <cloud>` and the {ref}`regions <cloud-data-model>` of a cloud.

A cloud default is a row in the table `cloud_defaults`. A region default is a row in `cloud_region_defaults`. {ref}`juju-db-repl` shows the definition of either with `.ddl <name>`.

When a model is created, its configuration takes the value of each key from the first of these that has one: the defaults built into this version of Juju, the defaults of the cloud's provider, the cloud's defaults, then the region's defaults. A region's value wins over the cloud's value for the same key. The default values of each key are listed with the key in the {ref}`list of model configuration keys <list-of-model-configuration-keys>`.

(model-defaults-operations)=
## Model default operations

Model default operations are part of {ref}`configuring models <model-configuring>`: a client reads the defaults of a cloud, sets them, and resets them, for a cloud or for one of its regions. Setting and reading them needs {ref}`superuser access <user-access-controller-superuser>` to the controller. A change block on the controller model refuses a change. A new model takes its values from the defaults at the moment it is created; a change later on does not reach models that exist ({ref}`adding a model <add-a-model>`).

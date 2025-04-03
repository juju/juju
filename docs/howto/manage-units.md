(manage-units)=
# How to manage units

> See also: {ref}`unit`

This document demonstrates various operations that you can perform on a unit.

```{important}
Units are also relevant when adding storage or scaling an application. See {ref}`manage-storage` and {ref}`manage-applications`.
```

(add-a-unit)=
## Add a unit

To add a unit, use the `add-unit` command followed by the application name:

```{important}
This is only true for machine deployments. For Kubernetes, see [How to control the number of units <5891md`.
```

```text
juju add-unit mysql
```

By using various command options, you can also specify the number of units, the model, the kind of storage, the target machine (e.g., if you want to collocate multiple units of the same or of different applications on the same machine -- though watch out for potentials configuration clashes!), etc.


> See more: {ref}`command-juju-add-unit`

(control-the-number-of-units)=
## Control the number of units

The procedure depends on whether you are on machines or rather Kubernetes.

**Machines.** To control the number of an application's units in a machine deployment, add or remove units in the amount required to obtain the desired number.

> See more: {ref}`add-a-unit`, {ref}`remove-a-unit`

**Kubernetes.** To control the number of an application's units in a Kubernetes deployment, run the `scale-application` command followed by the number of desired units (which can be both higher and lower than the current number).

```text
juju scale-application mediawiki 3
```

> See more: {ref}`command-juju-scale-application`

(view-details-about-a-unit)=
## View details about a unit

To see more details about a unit, use the `show-unit` command followed by the unit name:

```text
juju show-unit mysql/0
```

By using various options you can also choose to get just a subset of the output, a different output format, etc.

> See more: {ref}`command-juju-show-unit`


## List a unit's resources

To see the resources for a unit, use the `resources` command followed by the unit name. For example:

```text
juju resources mysql/0
```

> See more: {ref}`command-juju-resources`

## Show the status of a unit

To see the status of a unit, use the `status` command:

```text
juju status
```

This will show information about the model, along with its machines, applications and units. For example:

```text
Model           Controller           Cloud/Region        Version  SLA          Timestamp
tutorial-model  tutorial-controller  microk8s/localhost  2.9.34   unsupported  12:10:16+02:00

App             Version                         Status  Scale  Charm           Channel  Rev  Address         Exposed  Message
mattermost-k8s  .../mattermost:v6.6.0-20.04...  active      1  mattermost-k8s  stable    21  10.152.183.185  no
postgresql-k8s  .../postgresql@ed0e37f          active      1  postgresql-k8s  stable     4                  no       Pod configured

Unit               Workload  Agent  Address       Ports     Message
mattermost-k8s/0*  active    idle   10.1.179.151  8065/TCP
postgresql-k8s/0*  active    idle   10.1.179.149  5432/TCP  Pod configured
```

> See more: {ref}`command-juju-status`, [Unit status](https://juju.is/docs/juju/status#heading--unit-status)


## Set the meter status on a unit

To set the meter status on a unit, use the `set-meter-status` command followed by the unit name. For example:

```text
juju set-meter-status myapp/0
```

> See more: {ref}`command-juju-set-meter-status`

(mark-unit-errors-as-resolved)=
## Mark unit errors as resolved

To mark unit errors as resolved, use the `resolved` command followed by the unit name or a list of space-separated unit names. For example:

```text
juju resolved myapp/0
```

> See more: {ref}`command-juju-resolved`

(remove-a-unit)=
## Remove a unit

To remove individual units instead of the entire application (i.e. all the units), use the `remove-unit` command followed by the unit name. For example, the code below removes unit 2 of the PostgreSQL charm. For example:

```{important}
While this can be used for both machine and Kubernetes deployments, unless you care about which unit you're removing specifically, in Kubernetes you may also just run `juju scale-application <n>`, where `n` is less than the current number of units. See {ref}`control-the-number-of-units`.
```


```text
juju remove-unit postgresql/2
```

```{important}
In the case that the removed unit is the only one running, the corresponding machine will also be removed, unless any of the following is true for that machine: <br>

- it was created with `juju add-machine` <br>
- it is not being used as the only controller <br>
- it is not hosting Juju-managed containers (KVM guests or LXD containers)

```


It is also possible to remove multiple units at a time by passing instead a space-separated list of unit names:

```text
juju remove-unit mediawiki/1 mediawiki/3 mediawiki/5 mysql/2
```

<!--Why is this necessary? Doesn't removing a unit automatically destroy the storage?-->
To also destroy the storage attached to the units, add the `--destroy-storage` option.

<!--As a last resort in case of what...?-->
As a last resort, use the `--force` option (in `v.2.6.1`).

> See more: {ref}`command-juju-remove-unit`, {ref}`removing-things`


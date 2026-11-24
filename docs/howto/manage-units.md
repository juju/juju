---
myst:
  html_meta:
    description: "Learn how to add, remove, debug, and manage units in Juju applications. Complete guide to unit-level operations."
---

(manage-units)=
# How to manage units

```{ibnote}
See also: {ref}`unit`
```

This document demonstrates various operations that you can perform on a unit.

```{important}
Units are also relevant when adding storage or scaling an application. See {ref}`manage-storage` and {ref}`manage-applications`.
```

(add-a-unit)=
## Add a unit

On a machine or Kubernetes model, use `juju add-unit <application>` to add one unit to an existing application. Use `-n` to specify how many units to add, not the desired total. For example, to add two units:

```text
juju add-unit my-app -n 2
```

On machine models, you can also use `--to` to choose a target machine or `--attach-storage` to attach an existing storage volume.

```{ibnote}
See more: {ref}`command-juju-add-unit`
```

(control-the-number-of-units)=
## Control the number of units

The procedure depends on whether you are on machines or rather Kubernetes.

**Machines.** To control the number of an application's units in a machine deployment, add or remove units in the amount required to obtain the desired number.

```{ibnote}
See more: {ref}`add-a-unit`, {ref}`remove-a-unit`
```

**Kubernetes.** Use `juju scale-application <application> <scale>` to set the desired total, whether it is higher or lower than the current count. This command is only available on Kubernetes models.

```text
juju scale-application mediawiki 3
```

To change the count by a specific amount instead, see {ref}`add-a-unit` or {ref}`remove-a-unit`.

```{ibnote}
See more: {ref}`command-juju-scale-application`
```

(view-details-about-a-unit)=
## View details about a unit

To see more details about a unit, use the `show-unit` command followed by the unit name:

```text
juju show-unit mysql/0
```

By using various options you can also choose to get just a subset of the output, a different output format, etc.

```{ibnote}
See more: {ref}`command-juju-show-unit`
```

## List a unit's resources

To see the resources for a unit, use the `resources` command followed by the unit name. For example:

```text
juju resources mysql/0
```

```{ibnote}
See more: {ref}`command-juju-resources`
```

## Show the status of a unit

To see the status of a unit, use the `status` command:

```text
juju status
```

This will show information about the model, along with its machines, applications and units. For example:

```text
Model           Controller           Cloud/Region        Version  SLA          Timestamp
tutorial-model  tutorial-controller  microk8s/localhost  4.0.15   unsupported  12:10:16+02:00

App             Version                         Status  Scale  Charm           Channel  Rev  Address         Exposed  Message
mattermost-k8s  .../mattermost:v6.6.0-20.04...  active      1  mattermost-k8s  stable    21  10.152.183.185  no
postgresql-k8s  .../postgresql@ed0e37f          active      1  postgresql-k8s  stable     4                  no       Pod configured

Unit               Workload  Agent  Address       Ports     Message
mattermost-k8s/0*  active    idle   10.1.179.151  8065/TCP
postgresql-k8s/0*  active    idle   10.1.179.149  5432/TCP  Pod configured
```

```{ibnote}
See more: {ref}`command-juju-status`, {ref}`unit-status`
```

(mark-unit-errors-as-resolved)=
## Mark unit errors as resolved

To mark unit errors as resolved, use the `resolved` command followed by the unit name or a list of space-separated unit names. For example:

```text
juju resolved myapp/0
```

```{ibnote}
See more: {ref}`command-juju-resolved`
```

(remove-a-unit)=
## Remove a unit

**Machines.** Use `juju remove-unit` with the name of the unit you want to remove. For example, to remove unit 2 of `postgresql`:

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

To also destroy the storage attached to the units, add the `--destroy-storage` option.

As a last resort, use the `--force` option.

**Kubernetes.** You cannot remove a unit by name. Specify the application and use `--num-units` to set how many units to remove. For example, to remove one unit:

```text
juju remove-unit my-app --num-units 1
```

To set the desired total instead, see {ref}`control-the-number-of-units`.

```{ibnote}
See more: {ref}`command-juju-remove-unit`, {ref}`removing-things`
```


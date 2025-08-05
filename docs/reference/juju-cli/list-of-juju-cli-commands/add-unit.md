> See also: [remove-unit](#remove-unit)

## Summary
Adds one or more units to a deployed application.

## Usage
```juju add-unit [options] <application name>```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `--attach-storage` |  | Existing storage to attach to the deployed unit (not available on k8s models) |
| `-m`, `--model` |  | Model to operate in. Accepts [&lt;controller name&gt;:]&lt;model name&gt;&#x7c;&lt;model UUID&gt; |
| `-n`, `--num-units` | 1 | Number of units to add |
| `--to` |  | The machine and/or container to deploy the unit in (bypasses constraints) |

## Examples

Add five units of mysql on five new machines:

    juju add-unit mysql -n 5

Add a unit of mysql to machine 23 (which already exists):

    juju add-unit mysql --to 23

Add two units of mysql to existing machines 3 and 4:

   juju add-unit mysql -n 2 --to 3,4

Add three units of mysql, one to machine 3 and the others to new
machines:

    juju add-unit mysql -n 3 --to 3

Add a unit of mysql into a new LXD container on machine 7:

    juju add-unit mysql --to lxd:7

Add two units of mysql into two new LXD containers on machine 7:

    juju add-unit mysql -n 2 --to lxd:7,lxd:7

Add three units of mysql, one to a new LXD container on machine 7,
and the others to new machines:

    juju add-unit mysql -n 3 --to lxd:7

Add a unit of mysql to LXD container number 3 on machine 24:

    juju add-unit mysql --to 24/lxd/3

Add a unit of mysql to LXD container on a new machine:

    juju add-unit mysql --to lxd


## Details
The add-unit is used to scale out an application for improved performance or
availability.

The usage of this command differs depending on whether it is being used on a
k8s or cloud model.

Many charms will seamlessly support horizontal scaling while others may need
an additional application support (e.g. a separate load balancer). See the
documentation for specific charms to check how scale-out is supported.

For k8s models the only valid argument is -n, --num-units.
Anything additional will result in an error.

Example:

Add five units of mysql:
    juju add-unit mysql --num-units 5


For cloud models, by default, units are deployed to newly provisioned machines
in accordance with any application or model constraints.

This command also supports the placement directive ("--to") for targeting
specific machines or containers, which will bypass application and model
constraints. --to accepts a comma-separated list of placement specifications
(see examples below). If the length of this list is less than the number of
units being added, the remaining units will be added in the default way (i.e.
to new machines).





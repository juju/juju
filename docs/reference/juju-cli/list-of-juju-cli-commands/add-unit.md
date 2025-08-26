(command-juju-add-unit)=
# `juju add-unit`
> See also: [remove-unit](#remove-unit)

## Summary
Adds one or more units to a deployed application.

## Usage
```juju add-unit [options] <application name>```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `--attach-storage` |  | (Machine models only) Specify an existing storage volume to attach to the deployed unit. |
| `-m`, `--model` |  | Model to operate in. Accepts [&lt;controller name&gt;:]&lt;model name&gt;&#x7c;&lt;model UUID&gt; |
| `-n`, `--num-units` | 1 | Specify the number of units to add. |
| `--to` |  | (Machine models only) Specify a comma-separated list of placement directives. If the length of this list is less than `-n`, the remaining units will be added in the default way (i.e., to new machines). |

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

The `add-unit`command is used to scale out an application for improved performance or
availability.

Note: Some charms will seamlessly support horizontal scaling while others may need
an additional application support (e.g. a separate load balancer). See the
documentation for specific charms to check how scale-out is supported.

Further reading:

- https://documentation.ubuntu.com/juju/3.6/reference/unit/
- https://documentation.ubuntu.com/juju/3.6/reference/placement-directive/
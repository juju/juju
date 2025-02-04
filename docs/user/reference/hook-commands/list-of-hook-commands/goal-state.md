(hook-command-goal-state)=
# `goal-state`
## Summary
Print the status of the charm's peers and related units.

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `--format` | yaml | Specify output format (json&#x7c;yaml) |
| `-o`, `--output` |  | Specify an output file |

## Examples

    goal-state


## Details

'goal-state' command will list the charm units and relations, specifying their status and
their relations to other units in different charms.
goal-state queries information about charm deployment and returns it as structured data.

goal-state provides:
    - the details of other peer units have been deployed and their status
    - the details of remote units on the other end of each endpoint and their status

The output will be a subset of that produced by the juju status. There will be output
for sibling (peer) units and relation state per unit.

The unit status values are the workload status of the (sibling) peer units. We also use
a unit status value of dying when the unit’s life becomes dying. Thus unit status is one of:
    - allocating
    - active
    - waiting
    - blocked
    - error
    - dying

The relation status values are determined per unit and depend on whether the unit has entered
or left scope. The possible values are:
    - joining : a relation has been created, but no units are available. This occurs when the
      application on the other side of the relation is added to a model, but the machine hosting
      the first unit has not yet been provisioned. Calling relation-set will work correctly as
      that data will be passed through to the unit when it comes online, but relation-get will
      not provide any data.
    - joined : the relation is active. A unit has entered scope and is accessible to this one.
    - broken : unit has left, or is preparing to leave scope. Calling relation-get is not advised
      as the data will quickly out of date when the unit leaves.
    - suspended : parent cross model relation is suspended
    - error: an external error has been detected

By reporting error state, the charm has a chance to determine that goal state may not be reached
due to some external cause. As with status, we will report the time since the status changed to
allow the charm to empirically guess that a peer may have become stuck if it has not yet reached
active state.
(command-juju-remove-unit)=
# `juju remove-unit`

```
Usage: juju remove-unit [options] <unit> [...] | <application>

Summary:
Remove application units from the model.

Global Options:
--debug  (= false)
    equivalent to --show-log --logging-config=<root>=DEBUG
-h, --help  (= false)
    Show help on a command or other topic.
--logging-config (= "")
    specify log levels for modules
--quiet  (= false)
    show no informational output
--show-log  (= false)
    if set, write the log file to stderr
--verbose  (= false)
    show more verbose output

Command Options:
-B, --no-browser-login  (= false)
    Do not use web browser for authentication
--destroy-storage  (= false)
    Destroy storage attached to the unit
--force  (= false)
    Completely remove an unit and all its dependencies
-m, --model (= "")
    Model to operate in. Accepts [<controller name>:]<model name>|<model UUID>
--no-prompt  (= false)
    Does nothing. Option present for forward compatibility with Juju 3
--no-wait  (= false)
    Rush through unit removal without waiting for each individual step to complete
--num-units  (= 0)
    Number of units to remove (k8s models only)

Details:
Remove application units from the model.

The usage of this command differs depending on whether it is being used on a
k8s or cloud model.

Removing all units of a application is not equivalent to removing the
application itself; for that, the `juju remove-application` command
is used.

For k8s models only a single application can be supplied and only the
--num-units argument supported.
Specific units cannot be targeted for removal as that is handled by k8s,
instead the total number of units to be removed is specified.

Examples:
    juju remove-unit wordpress --num-units 2

For cloud models specific units can be targeted for removal.
Units of a application are numbered in sequence upon creation. For example, the
fourth unit of wordpress will be designated "wordpress/3". These identifiers
can be supplied in a space delimited list to remove unwanted units from the
model.

Juju will also remove the machine if the removed unit was the only unit left
on that machine (including units in containers).

Sometimes, the removal of the unit may fail as Juju encounters errors
and failures that need to be dealt with before a unit can be removed.
For example, Juju will not remove a unit if there are hook failures.
However, at times, there is a need to remove a unit ignoring
all operational errors. In these rare cases, use --force option but note
that --force will remove a unit and, potentially, its machine without
given them the opportunity to shutdown cleanly.

Unit removal is a multi-step process. Under normal circumstances, Juju will not
proceed to the next step until the current step has finished.
However, when using --force, users can also specify --no-wait to progress through steps
without delay waiting for each step to complete.

Examples:

    juju remove-unit wordpress/2 wordpress/3 wordpress/4

    juju remove-unit wordpress/2 --destroy-storage

    juju remove-unit wordpress/2 --force

    juju remove-unit wordpress/2 --force --no-wait

See also:
    remove-application
    scale-application
```
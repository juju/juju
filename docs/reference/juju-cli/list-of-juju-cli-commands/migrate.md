(command-juju-migrate)=
# `juju migrate`

```
Usage: juju migrate [options] <model-name> <target-controller-name>

Summary:
Migrate a hosted model to another controller.

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

Details:
migrate begins the migration of a model from its current controller to
a new controller. This is useful for load balancing when a controller
is too busy, or as a way to upgrade a model's controller to a newer
Juju version. Once complete, the model's machine and and unit agents
will be connected to the new controller. The model will no longer be
available at the source controller.

Note that only hosted models can be migrated. Controller models can
not be migrated.

If the migration fails for some reason, the model be returned to its
original state with the model being managed by the original
controller.

In order to start a migration, the target controller must be in the
juju client's local configuration cache. See the juju "login" command
for details of how to do this.

This command only starts a model migration - it does not wait for its
completion. The progress of a migration can be tracked using the
"status" command and by consulting the logs.

See also:
    login
    controllers
    status
```
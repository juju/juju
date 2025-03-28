(command-juju-upgrade-series)=
# `juju upgrade-series`

```
Usage: juju upgrade-series [options] <machine> <command> [args]

Summary:
Upgrade the Ubuntu series of a machine.

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
--force  (= false)
    Upgrade even if the series is not supported by the charm and/or related subordinate charms.
-m, --model (= "")
    Model to operate in. Accepts [<controller name>:]<model name>|<model UUID>
-y, --yes  (= false)
    Agree that the operation cannot be reverted or canceled once started without being prompted.

Details:
Upgrade a machine's operating system series.

upgrade-series allows users to perform a managed upgrade of the operating system
series of a machine. This command is performed in two steps; prepare and complete.

The "prepare" step notifies Juju that a series upgrade is taking place for a given
machine and as such Juju guards that machine against operations that would
interfere with the upgrade process.

The "complete" step notifies juju that the managed upgrade has been successfully completed.

It should be noted that once the prepare command is issued there is no way to
cancel or abort the process. Once you commit to prepare you must complete the
process or you will end up with an unusable machine!

The requested series must be explicitly supported by all charms deployed to
the specified machine. To override this constraint the --force option may be used.

The --force option should be used with caution since using a charm on a machine
running an unsupported series may cause unexpected behavior. Alternately, if the
requested series is supported in later revisions of the charm, upgrade-charm can
run beforehand.

Examples:

Prepare machine 3 for upgrade to series "bionic"":

	juju upgrade-series 3 prepare bionic

Prepare machine 4 for upgrade to series "focal" even if there are applications
running units that do not support the target series:

	juju upgrade-series 4 prepare focal --force

Complete upgrade of machine 5, indicating that all automatic and any
necessary manual upgrade steps have completed successfully:

	juju upgrade-series 5 complete

See also:
    machines
    status
    upgrade-charm
    set-series

Aliases: upgrade-machine
```
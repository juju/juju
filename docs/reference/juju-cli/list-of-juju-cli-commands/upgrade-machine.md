(command-juju-upgrade-machine)=
# `juju upgrade-machine`
> See also: [machines](#machines), [status](#status), [refresh](#refresh), [set-application-base](#set-application-base)

## Summary
Upgrade the Ubuntu base of a machine.

## Usage
```juju upgrade-machine [options] <machine> <command> [args]```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `--force` | false | Upgrade even if the base is not supported by the charm and/or related subordinate charms. |
| `-m`, `--model` |  | Model to operate in. Accepts [&lt;controller name&gt;:]&lt;model name&gt;&#x7c;&lt;model UUID&gt; |
| `-y`, `--yes` | false | Agree that the operation cannot be reverted or canceled once started without being prompted. |

## Examples

Prepare machine 3 for upgrade to base "ubuntu@18.04"":

	juju upgrade-machine 3 prepare ubuntu@18.04

Prepare machine 4 for upgrade to base "ubuntu@20.04" even if there are 
applications running units that do not support the target base:

	juju upgrade-machine 4 prepare ubuntu@20.04 --force

Complete upgrade of machine 5, indicating that all automatic and any
necessary manual upgrade steps have completed successfully:

	juju upgrade-machine 5 complete


## Details

Upgrade a machine's operating system release.

upgrade-machine allows users to perform a managed upgrade of the operating system
release of a machine using a base. This command is performed in two steps; 
prepare and complete.

The "prepare" step notifies Juju that a base upgrade is taking place for a given
machine and as such Juju guards that machine against operations that would
interfere with the upgrade process. A base can be specified using the OS name
and the version of the OS, separated by @.

The "complete" step notifies juju that the managed upgrade has been successfully 
completed.

It should be noted that once the prepare command is issued there is no way to
cancel or abort the process. Once you commit to prepare you must complete the
process or you will end up with an unusable machine!

The requested base must be explicitly supported by all charms deployed to
the specified machine. To override this constraint the --force option may be used.

The --force option should be used with caution since using a charm on a machine
running an unsupported base may cause unexpected behavior. Alternately, if the
requested base is supported in later revisions of the charm, upgrade-charm can
run beforehand.
(hook-command-juju-reboot)=
# `juju-reboot`
## Summary
Reboot the host machine.

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `--now` | false | reboot immediately, killing the invoking process |

## Examples

    # immediately reboot
    juju-reboot --now

    # Reboot after current hook exits
    juju-reboot


## Details

juju-reboot causes the host machine to reboot, after stopping all containers
hosted on the machine.

An invocation without arguments will allow the current hook to complete, and
will only cause a reboot if the hook completes successfully.

If the --now flag is passed, the current hook will terminate immediately, and
be restarted from scratch after reboot. This allows charm authors to write
hooks that need to reboot more than once in the course of installing software.

The --now flag cannot terminate a debug-hooks session; hooks using --now should
be sure to terminate on unexpected errors, so as to guarantee expected behaviour
in all situations.

juju-reboot is not supported when running actions.
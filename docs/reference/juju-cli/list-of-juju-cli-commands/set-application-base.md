(command-juju-set-application-base)=
# `juju set-application-base`
> See also: [status](#status), [refresh](#refresh), [upgrade-machine](#upgrade-machine)

## Summary
Sets an application's base.

## Usage
```juju set-application-base [options] <application> <base>```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Specifies whether to skip web browser for authentication. |
| `-m`, `--model` |  | Specifies the model to operate in. Accepts `[<controller name>:]<model name>|<model UUID>`. |

## Examples

Set the base for the `ubuntu` application to `ubuntu@20.04`:

	juju set-application-base ubuntu ubuntu@20.04


## Details

Sets the specified application's base value within Juju. Any subordinates
of the application will also have their base set to the provided value. A base
can be specified using the OS name and the version of the OS, separated by `@`.

This will not change the base of any existing units, rather new units will use
the new base when deployed.

It is recommended to only do this after upgrade-machine has been run for
machine containing all existing units of the application.

To ensure correct binaries, run `juju refresh` before running `juju add-unit`.
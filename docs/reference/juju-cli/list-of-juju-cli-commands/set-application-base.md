(command-juju-set-application-base)=
# `juju set-application-base`
> See also: [status](#status), [refresh](#refresh), [upgrade-machine](#upgrade-machine)

## Summary
Set an application's base.

## Usage
```juju set-application-base [options] <application> <base>```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `-m`, `--model` |  | Model to operate in. Accepts [&lt;controller name&gt;:]&lt;model name&gt;&#x7c;&lt;model UUID&gt; |

## Examples

Set the base for the `ubuntu` application to `ubuntu@20.04`:

	juju set-application-base ubuntu ubuntu@20.04


## Details

The specified application's base value will be set within juju. Any subordinates
of the application will also have their base set to the provided value. A base
can be specified using the OS name and the version of the OS, separated by `@`.

This will not change the base of any existing units, rather new units will use
the new base when deployed.

It is recommended to only do this after upgrade-machine has been run for
machine containing all existing units of the application.

To ensure correct binaries, run `juju refresh` before running `juju add-unit`.
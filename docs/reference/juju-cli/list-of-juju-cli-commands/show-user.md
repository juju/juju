(command-juju-show-user)=
# `juju show-user`
> See also: [add-user](#command-juju-add-user), [register](#command-juju-register), [users](#command-juju-users)

## Summary
Show information about a user.

## Usage
```text
juju show-user [options] [<user name>]
```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-c`, `--controller` |  | Controller to operate in |
| `--exact-time` | false | Use full timestamps for local user records on the controller |
| `--format` | yaml | Specify output format (json&#x7c;yaml) |
| `-o`, `--output` |  | Specify an output file |

## Examples

Look up your account on the current controller:

    juju show-user

Look up jsmith on the current controller as a controller superuser:

    juju show-user jsmith

Print your account on the current controller as JSON:

    juju show-user --format json


## Details
By default, the CLI prints one YAML record for the account saved on the
selected controller. Give a username to look up that user. A controller
superuser can look up another user, including a disabled user.

Further reading:

- https://canonical.com/juju/docs/juju-cli/latest/reference/user/
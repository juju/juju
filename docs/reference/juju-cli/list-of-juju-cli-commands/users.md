(command-juju-users)=
# `juju users`
> See also: [add-user](#command-juju-add-user), [register](#command-juju-register), [show-user](#command-juju-show-user), [disable-user](#command-juju-disable-user), [enable-user](#command-juju-enable-user)

**Aliases:** list-users

## Summary
Lists Juju users allowed to connect to a controller or model.

## Usage
```text
juju users [options] [model-name]
```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `--all` | false | Include disabled users (on controller only) |
| `-c`, `--controller` |  | Controller to operate in |
| `--exact-time` | false | Use full timestamps for local user records on the controller |
| `--format` | tabular | Specify output format (json&#x7c;tabular&#x7c;yaml) |
| `-o`, `--output` |  | Specify an output file |

## Examples

List the users visible to you on the current controller:

    juju users

Include disabled users on the operations controller as a controller superuser:

    juju users -c operations --all

List the users visible to you on the mymodel model:

    juju users mymodel


## Details

To list users on the selected controller, omit the model name. A controller
superuser sees enabled users by default and can include disabled users with
`--all`. Other users see their own record.

To list visible direct grants on a model, provide its name. A model admin
or controller superuser sees the model roster. Other users who can read the
model see their own direct entry. If the CLI reports `user not found on model`,
ask a model admin to check the roster. The model listing uses relative
connection times and shows enabled users regardless of `--exact-time`
and `--all`

Further reading:

- https://canonical.com/juju/docs/juju-cli/latest/reference/user/
(command-juju-resources)=
# `juju resources`
> See also: [attach-resource](#attach-resource), [charm-resources](#charm-resources)

**Aliases:** list-resources

## Summary
Shows the resources for an application or unit.

## Usage
```juju resources [options] <application or unit>```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Specifies whether to skip web browser for authentication. |
| `--details` | false | Shows detailed information about the resources used by each unit. |
| `--format` | tabular | Specify output format (json&#x7c;tabular&#x7c;yaml) |
| `-m`, `--model` |  | Specifies the model to operate in. Accepts `[<controller name>:]<model name>|<model UUID>`. |
| `-o`, `--output` |  | Specify an output file |

## Examples

To list resources for an application:

	juju resources mysql

To list resources for a unit:

	juju resources mysql/0

To show detailed information about resources used by a unit:

	juju resources mysql/0 --details


## Details

Shows the resources required by and those in use by an existing
application or unit in your model.  When run for an application, it will also show any
updates available for resources from a store.
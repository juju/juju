> See also: [attach-resource](#attach-resource), [charm-resources](#charm-resources)

**Aliases:** list-resources

## Summary
Show the resources for an application or unit.

## Usage
```juju resources [options] <application or unit>```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `--details` | false | show detailed information about resources used by each unit. |
| `--format` | tabular | Specify output format (json&#x7c;tabular&#x7c;yaml) |
| `-m`, `--model` |  | Model to operate in. Accepts [&lt;controller name&gt;:]&lt;model name&gt;&#x7c;&lt;model UUID&gt; |
| `-o`, `--output` |  | Specify an output file |

## Examples

To list resources for an application:

	juju resources mysql

To list resources for a unit:

	juju resources mysql/0

To show detailed information about resources used by a unit:

	juju resources mysql/0 --details


## Details

This command shows the resources required by and those in use by an existing
application or unit in your model.  When run for an application, it will also show any
updates available for resources from a store.




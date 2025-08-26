(command-juju-charm-resources)=
# `juju charm-resources`
> See also: [resources](#resources), [attach-resource](#attach-resource)

**Aliases:** list-charm-resources

## Summary
Display the resources for a charm in a repository.

## Usage
```juju charm-resources [options] <charm>```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `--channel` | stable | the channel of the charm |
| `--format` | tabular | Specify output format (json&#x7c;tabular&#x7c;yaml) |
| `-m`, `--model` |  | Model to operate in. Accepts [&lt;controller name&gt;:]&lt;model name&gt;&#x7c;&lt;model UUID&gt; |
| `-o`, `--output` |  | Specify an output file |

## Examples

Display charm resources for the `postgresql` charm:

    juju charm-resources postgresql

Display charm resources for `mycharm` in the `2.0/edge` channel:

    juju charm-resources mycharm --channel 2.0/edge



## Details

This command will report the resources and the current revision of each
resource for a charm in a repository.

Channel can be specified with `--channel`.  If not provided, `stable` is used.
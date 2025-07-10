(command-juju-get-annotations)=
# `juju get-annotations`
## Summary
Get annotations.

## Usage
```juju get-annotations [options] <resource tag> [<resource tag 2>...]```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `--format` | tabular | Specify output format (json&#x7c;tabular&#x7c;yaml) |
| `-m`, `--model` |  | Model to operate in. Accepts [&lt;controller name&gt;:]&lt;model name&gt;&#x7c;&lt;model UUID&gt; |
| `-o`, `--output` |  | Specify an output file |

## Examples

	juju get-annotations model-<modelUUID1> model-<modelUUID2>
	juju get-annotations applicationoffer-<offerUUID>


## Details

Get annotations for an entity.
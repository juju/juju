(command-juju-set-annotations)=
# `juju set-annotations`
## Summary
Set annotations.

## Usage
```juju set-annotations [options] <resource tag> [key=value...]```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `-m`, `--model` |  | Model to operate in. Accepts [&lt;controller name&gt;:]&lt;model name&gt;&#x7c;&lt;model UUID&gt; |

## Examples

	juju set-annotations model-<modelUUID> owner=alice
	juju set-annotations applicationoffer-<offerUUID> stage=staging owner=eve


## Details

Set annotations for an entity with a list of key values.
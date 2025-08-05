> See also: [add-space](#add-space), [spaces](#spaces), [reload-spaces](#reload-spaces), [remove-space](#remove-space), [show-space](#show-space)

## Summary
Rename a network space.

## Usage
```juju rename-space [options] <old-name> <new-name>```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `-m`, `--model` |  | Model to operate in. Accepts [&lt;controller name&gt;:]&lt;model name&gt;&#x7c;&lt;model UUID&gt; |
| `--rename` |  | the new name for the network space |

## Examples

Rename a space from db to fe:

	juju rename-space db fe


## Details
Renames an existing space from "old-name" to "new-name". Does not change the
associated subnets and "new-name" must not match another existing space.



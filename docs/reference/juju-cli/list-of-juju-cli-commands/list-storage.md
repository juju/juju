> See also: [show-storage](#show-storage), [add-storage](#add-storage), [remove-storage](#remove-storage)

**Aliases:** list-storage

## Summary
Lists storage details.

## Usage
```juju storage [options] <filesystem|volume> ...```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `--filesystem` | false | List filesystem storage(deprecated) |
| `--format` | tabular | Specify output format (json&#x7c;tabular&#x7c;yaml) |
| `-m`, `--model` |  | Model to operate in. Accepts [&lt;controller name&gt;:]&lt;model name&gt;&#x7c;&lt;model UUID&gt; |
| `-o`, `--output` |  | Specify an output file |
| `--volume` | false | List volume storage(deprecated) |

## Examples

List all storage:

    juju storage

List only filesystem storage:

    juju storage --filesystem

List only volume storage:

    juju storage --volume


## Details

List information about storage.




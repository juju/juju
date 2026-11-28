(command-juju-storage)=
# `juju storage`
> See also: [show-storage](#command-juju-show-storage), [add-storage](#command-juju-add-storage), [remove-storage](#command-juju-remove-storage)

**Aliases:** list-storage

## Summary
Lists storage details.

## Usage
```text
juju storage [options] [<machine-id> ...]
```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `--filesystem` | false | List filesystem storage (deprecated) |
| `--format` | tabular | Specify output format (json&#x7c;tabular&#x7c;yaml) |
| `-m`, `--model` |  | Model to operate in. Accepts [&lt;controller name&gt;:]&lt;model name&gt;&#x7c;&lt;model UUID&gt; |
| `-o`, `--output` |  | Specify an output file |
| `--volume` | false | List volume storage (deprecated) |

## Examples

List all storage:

    juju storage

List only filesystem storage:

    juju storage --filesystem

List only volume storage:

    juju storage --volume


## Details

List storage instances in the selected model, including detached instances.
Use `juju show-storage` for a specific instance ID. The deprecated
`--filesystem` and `--volume` options list backing resources. Use one at a time,
optionally followed by machine IDs. An empty result applies to that selection;
empty JSON or YAML output has no document.

Further reading:

- https://canonical.com/juju/docs/juju-cli/latest/reference/storage/
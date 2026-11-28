(command-juju-show-storage)=
# `juju show-storage`
> See also: [storage](#command-juju-storage), [attach-storage](#command-juju-attach-storage), [detach-storage](#command-juju-detach-storage), [remove-storage](#command-juju-remove-storage)

## Summary
Shows storage instance information.

## Usage
```text
juju show-storage [options] <storage ID> [...]
```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `--format` | yaml | Specify output format (json&#x7c;yaml) |
| `-m`, `--model` |  | Model to operate in. Accepts [&lt;controller name&gt;:]&lt;model name&gt;&#x7c;&lt;model UUID&gt; |
| `-o`, `--output` |  | Specify an output file |

## Examples

Inspect an instance listed by juju storage:

    juju show-storage osd-devices/2


## Details

Inspect storage instances by their Juju IDs in the selected model. Pass
each ID as a separate argument. Check the attachment and location for
the intended unit.

Further reading:

- https://canonical.com/juju/docs/juju-cli/latest/reference/storage/
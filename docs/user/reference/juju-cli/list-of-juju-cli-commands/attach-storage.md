(command-juju-attach-storage)=
# `juju attach-storage`
## Summary
Attaches existing storage to a unit.

## Usage
```juju attach-storage [options] <unit> <storage> [<storage> ...]```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `-m`, `--model` |  | Model to operate in. Accepts [&lt;controller name&gt;:]&lt;model name&gt;&#x7c;&lt;model UUID&gt; |

## Examples

    juju attach-storage postgresql/1 pgdata/0



## Details

Attach existing storage to a unit. Specify a unit
and one or more storage IDs to attach to it.
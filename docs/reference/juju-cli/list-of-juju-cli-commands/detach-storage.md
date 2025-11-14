(command-juju-detach-storage)=
# `juju detach-storage`
> See also: [storage](#storage), [attach-storage](#attach-storage)

## Summary
Detaches storage from units.

## Usage
```juju detach-storage [options] <storage> [<storage> ...]```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `--force` | false | Forcefully detach storage |
| `-m`, `--model` |  | Model to operate in. Accepts [&lt;controller name&gt;:]&lt;model name&gt;&#x7c;&lt;model UUID&gt; |

## Examples

    juju detach-storage pgdata/0
    juju detach-storage --force pgdata/0



## Details

Detaches storage from units. Specify one or more storage IDs (storage_name/id),
as output by `juju storage`. The storage will remain in the model
until it is removed by an operator. The storage being detached will be removed
from all units that are using it.

Detaching storage may fail but under some circumstances, Juju user may need
to force storage detachment despite operational errors. Storage detachments are
not performed as a single operation, so when detaching multiple storage IDs it
may be that some detachments succeed while others fail. In this case the command
can be executed again to retry the failed detachments.
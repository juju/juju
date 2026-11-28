(command-juju-detach-storage)=
# `juju detach-storage`
> See also: [storage](#command-juju-storage), [attach-storage](#command-juju-attach-storage)

## Summary
Detaches storage from units.

## Usage
```text
juju detach-storage [options] <storage> [<storage> ...]
```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `--force` | false | Forcefully detach storage |
| `-m`, `--model` |  | Model to operate in. Accepts [&lt;controller name&gt;:]&lt;model name&gt;&#x7c;&lt;model UUID&gt; |

## Examples

Detach osd-devices/2 from all its units after stopping workload use and
checking each unit can spare the device:

    juju detach-storage osd-devices/2


## Details

Detach each specified instance from all units using it. Supply Juju storage
IDs from juju storage. Stop workload use first. If the request fails,
inspect each instance with show-storage to find which units still have
attachments before retrying.

Further reading:

- https://canonical.com/juju/docs/juju-cli/latest/reference/storage/
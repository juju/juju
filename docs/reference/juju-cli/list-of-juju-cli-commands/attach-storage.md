(command-juju-attach-storage)=
# `juju attach-storage`
## Summary
Attaches existing storage to a unit.

## Usage
```text
juju attach-storage [options] <unit> <storage> [<storage> ...]
```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `-m`, `--model` |  | Model to operate in. Accepts [&lt;controller name&gt;:]&lt;model name&gt;&#x7c;&lt;model UUID&gt; |

## Examples

Attach the detached OSD device osd-devices/2 to ceph-osd/1 after checking its placement and capacity:

    juju attach-storage ceph-osd/1 osd-devices/2


## Details

Attach one or more detached Juju storage instances to an existing machine
unit. Pass the instance IDs from juju storage as separate arguments.
Check the target unit's charm storage declaration and machine placement first.
Use show-storage to check each attachment after the request.

Further reading:

- https://canonical.com/juju/docs/juju-cli/latest/reference/storage/
(hook-command-juju-log)=
# `juju-log`
## Summary
Writes a message to Juju logs.

## Usage
```text
juju-log [options] <message>
```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `--debug` | false | Sends message at debug level. |
| `--format` |  | (DEPRECATED) Specifies the message format. |
| `-l`, `--log-level` | INFO | Sends message at the given level. |

## Examples

    juju-log -l 'WARN' Something has transpired
(command-juju-sync-agent-binary)=
# `juju sync-agent-binary`
> See also: [upgrade-controller](#upgrade-controller)

## Summary
Copy agent binaries from the official agent store into a local controller.

## Usage
```juju sync-agent-binary [options] ```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Specifies whether to skip web browser for authentication. |
| `--agent-version` |  | Copies a specific `major[.minor]` version. |
| `--dry-run` | false | Specifies whether to merely simulate the copy operation. |
| `--local-dir` |  | Specifies the local destination directory. |
| `-m`, `--model` |  | Specifies the model to operate in. Accepts `[<controller name>:]<model name>|<model UUID>`. |
| `--public` | false | Specifies whether to generate mirrors information for a public cloud. |
| `--source` |  | Specifies the local source directory. |
| `--stream` |  | Specifies the simplestreams stream for which to sync metadata. |

## Examples

    juju sync-agent-binary --debug --agent-version 2.0
    juju sync-agent-binary --debug --agent-version 2.0 --local-dir=/home/ubuntu/sync-agent-binary


## Details

This copies the Juju agent software from the official agent binaries store
(located at https://streams.canonical.com/juju) into the controller.
It is generally done when the controller is without internet access.

Instead of the above site, a local directory can be specified as source.
The online store will, of course, need to be contacted at some point to get
the software.
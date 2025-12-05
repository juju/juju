(command-juju-sync-agent-binary)=
# `juju sync-agent-binary`
> See also: [upgrade-controller](#upgrade-controller)

## Summary
Copies agent binaries from the official agent store into a local controller.

## Usage
```juju sync-agent-binary [options] ```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Disables web browser for authentication. |
| `--agent-version` |  | Specifies the major[.minor] version to copy. |
| `--dry-run` | false | Simulates the copy without making changes. |
| `--local-dir` |  | Specifies a local destination directory. |
| `-m`, `--model` |  | Model to operate in. Accepts [&lt;controller name&gt;:]&lt;model name&gt;&#x7c;&lt;model UUID&gt; |
| `--public` | false | Generates mirrors information for a public cloud. |
| `--source` |  | Specifies a local source directory. |
| `--stream` |  | Specifies the simplestreams stream for which to sync metadata. |

## Examples

    juju sync-agent-binary --debug --agent-version 2.0
    juju sync-agent-binary --debug --agent-version 2.0 --local-dir=/home/ubuntu/sync-agent-binary


## Details

Copies the Juju agent software from the official agent binaries store
(located at https://streams.canonical.com/juju) into the controller.
It is generally done when the controller is without internet access.

Instead of the above site, a local directory can be specified as source.
The online store will, of course, need to be contacted at some point to get
the software.
(command-juju-trust)=
# `juju trust`
> See also: [config](#config)

## Summary
Sets the trust status of a deployed application to true.

## Usage
```juju trust [options] <application name>```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `-m`, `--model` |  | Model to operate in. Accepts [&lt;controller name&gt;:]&lt;model name&gt;&#x7c;&lt;model UUID&gt; |
| `--remove` | false | Remove trusted access from a trusted application |
| `--scope` |  | (Kubernetes models only) Needs to be set to `cluster` |

## Examples

    juju trust media-wiki
    juju trust metallb --scope=cluster


## Details
Sets the trust configuration value to true.

On Kubernetes models, the `trust` operation currently grants the charm full access to the cluster.
Until the permissions model is refined to grant more granular role-based access, the use of
`--scope=cluster` is required to confirm this choice.
(command-juju-trust)=
# `juju trust`
> See also: [config](#command-juju-config)

## Summary
Grants or removes trust for a deployed application.

## Usage
```text
juju trust [options] <application name>
```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `-m`, `--model` |  | Model to operate in. Accepts [&lt;controller name&gt;:]&lt;model name&gt;&#x7c;&lt;model UUID&gt; |
| `--remove` | false | Remove trusted access from a trusted application |
| `--scope` |  | Set to `cluster` to confirm a Kubernetes trust grant |

## Examples

Grant trust to kubernetes-control-plane in a machine model:

    juju trust kubernetes-control-plane

Grant cluster-wide trust to traefik in a Kubernetes model:

    juju trust traefik --scope=cluster

Remove trust from traefik:

    juju trust traefik --remove


## Details
A trusted charm can request cloud connection details with the
`credential-get` hook command. Check what access the charm needs
before granting trust.

On Kubernetes, granting trust can give the charm cluster-wide access.
A grant requires `--scope=cluster`. Use `--remove`
to remove trust.

Further reading:

- https://canonical.com/juju/docs/juju-cli/latest/reference/application/
(command-juju-reprovision-machine)=
# `juju reprovision-machine`
## Summary
Reprovision a machine whose cloud instance has been lost.

## Usage
```juju reprovision-machine [options] <machine>```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `-m`, `--model` |  | Model to operate in. Accepts [&lt;controller name&gt;:]&lt;model name&gt;&#x7c;&lt;model UUID&gt; |

## Examples

    juju reprovision-machine 3


## Details

Reprovision a machine whose backing cloud instance is operator-declared lost.
This preserves the Juju machine identity and unit assignment and creates a
replacement cloud instance through the normal provisioning path.

Root disk, ephemeral disk, charm-local state, and machine-scoped storage
data are NOT recovered. The replacement instance will have empty storage.

This command is only supported for top-level, non-controller, IaaS
provider-backed machines without child container machines or attached
model-scoped storage.
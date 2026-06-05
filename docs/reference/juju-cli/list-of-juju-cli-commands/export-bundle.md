(command-juju-export-bundle)=
# `juju export-bundle`
## Summary
Exports the current model configuration as a reusable bundle.

## Usage
```juju export-bundle [options] ```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `--filename` |  | Bundle file |
| `--include-charm-defaults` | false | Whether to include charm config default values in the exported bundle |
| `--include-series` | false | Compatibility option. Set to include series in the bundle alongside bases |
| `-m`, `--model` |  | Model to operate in. Accepts [&lt;controller name&gt;:]&lt;model name&gt;&#x7c;&lt;model UUID&gt; |

## Examples

    juju export-bundle
    juju export-bundle --filename mymodel.yaml
    juju export-bundle --include-charm-defaults
    juju export-bundle --include-series


## Details

Exports the current model's applications and relations as a reusable bundle.

The bundle does not mirror the model configuration. It is a self-contained
definition derived from the applications currently deployed in the model, so
that the same set of applications and relations can be reproduced in another
model.

Juju may optimise how information is represented in the exported bundle.
For example, if all applications share the same base, Juju may set a
bundle-level default-base. 

Exposure rules and application offers may
also be captured in an overlay as a second YAML document within the same
file (for example, using exposed-endpoints and offers entries). These
optimisations change only how the bundle is expressed and do not affect
the resulting deployment.

If `--filename` is not used, the configuration is printed to `stdout`.
` --filename` specifies an output file.

If `--include-series` is used, the exported bundle will include the OS series
 alongside bases. This should be used as a compatibility option for older
 versions of Juju before bases were added.
(command-juju-export-bundle)=
# `juju export-bundle`

## Summary
Exports the current model configuration as a reusable bundle.

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `--filename` |  | Bundle file |
| `--include-charm-defaults` | false | Whether to include charm config default values in the exported bundle |
| `--include-series` | false | Comaptibility option. Set to include series in the bundle alongside bases |
| `-m`, `--model` |  | Model to operate in. Accepts [&lt;controller name&gt;:]&lt;model name&gt;&#x7c;&lt;model UUID&gt; |

## Examples

    juju export-bundle
    juju export-bundle --filename mymodel.yaml
    juju export-bundle --include-charm-defaults
    juju export-bundle --include-series


## Details

Exports the current model configuration as a reusable bundle.

If --filename is not used, the configuration is printed to stdout.
 --filename specifies an output file.

If --include-series is used, the exported bundle will include the OS series
 alongside bases. This should be used as a compatibility option for older
 versions of Juju before bases were added.
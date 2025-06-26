(command-juju-find-offers)=
# `juju find-offers`
> See also: [show-offer](#show-offer)

## Summary
Find offered application endpoints.

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `--format` | tabular | Specify output format (json&#x7c;tabular&#x7c;yaml) |
| `--interface` |  | return results matching the interface name |
| `-m`, `--model` |  | Model to operate in. Accepts [&lt;controller name&gt;:]&lt;model name&gt;&#x7c;&lt;model UUID&gt; |
| `-o`, `--output` |  | Specify an output file |
| `--offer` |  | return results matching the offer name |
| `--url` |  | return results matching the offer URL |

## Examples

    juju find-offers
    juju find-offers mycontroller:
    juju find-offers staging/mymodel
    juju find-offers --interface mysql
    juju find-offers --url staging/mymodel.db2
    juju find-offers --offer db2
   


## Details

Find which offered application endpoints are available to the current user.

This command is aimed for a user who wants to discover what endpoints are available to them.
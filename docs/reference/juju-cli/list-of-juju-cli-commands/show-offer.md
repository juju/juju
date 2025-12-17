(command-juju-show-offer)=
# `juju show-offer`
> See also: [find-offers](#find-offers)

## Summary
Shows extended information about the offered application.

## Usage
```juju show-offer [options] [<controller>:]<offer url>```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Specifies whether to skip web browser for authentication. |
| `--format` | tabular | Specify output format (json&#x7c;tabular&#x7c;yaml) |
| `-m`, `--model` |  | Specifies the model to operate in. Accepts `[<controller name>:]<model name>|<model UUID>`. |
| `-o`, `--output` |  | Specify an output file |

## Examples

To show the extended information for the application `prod` offered
from the model `default` on the same Juju controller:

     juju show-offer default.prod

The supplied URL can also include a username where offers require them.
This will be given as part of the URL retrieved from the
`juju find-offers` command. To show information for the application
'prod' from the model 'default' from the user 'admin':

    juju show-offer admin/default.prod

To show the information regarding the application `prod` offered from
the model `default` on an accessible controller named `controller`:

    juju show-offer controller:default.prod



## Details

Shows extended information about the application offered from a particular URL.

In addition to the URL of
the offer, additional information is provided from the README file of the
charm being offered.
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
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `--format` | tabular | Specify output format (json&#x7c;tabular&#x7c;yaml) |
| `-m`, `--model` |  | Model to operate in. Accepts [&lt;controller name&gt;:]&lt;model name&gt;&#x7c;&lt;model UUID&gt; |
| `-o`, `--output` |  | Specify an output file |

## Examples

To show the extended information for the application 'prod' offered
from the model 'default' on the same Juju controller:

     juju show-offer default.prod

The supplied URL can also include a username where offers require them. 
This will be given as part of the URL retrieved from the
'juju find-offers' command. To show information for the application
'prod' from the model 'default' from the user 'admin':

    juju show-offer admin/default.prod

To show the information regarding the application 'prod' offered from
the model 'default' on an accessible controller named 'controller':

    juju show-offer controller:default.prod



## Details

This command is intended to enable users to learn more about the
application offered from a particular URL. In addition to the URL of
the offer, extra information is provided from the readme file of the
charm being offered.
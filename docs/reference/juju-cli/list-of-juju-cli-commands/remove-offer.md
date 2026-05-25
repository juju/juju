(command-juju-remove-offer)=
# `juju remove-offer`
> See also: [find-offers](#find-offers), [offer](#offer)

## Summary
Removes one or more offers specified by their URL.

## Usage
```juju remove-offer [options] <offer-url> ...```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `-c`, `--controller` |  | Controller to operate in |
| `--force` | false | Force remove the offer |
| `--no-prompt` | false | Do not prompt for confirmation |

## Examples

    juju remove-offer staging/mymodel.hosted-mysql
    juju remove-offer hosted-mysql


## Details

Remove one or more application offers.

If an offer has active connections, Juju will ask for confirmation before
removing the offer and the relations to it unless --no-prompt is used.

Use --force to request forced offer removal from the controller.

Offers to remove are normally specified by their URL.
It's also possible to specify just the offer name, in which case
the offer is considered to reside in the current model.
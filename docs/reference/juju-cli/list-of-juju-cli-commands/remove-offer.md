(command-juju-remove-offer)=
# `juju remove-offer`
> See also: [find-offers](#command-juju-find-offers), [offer](#command-juju-offer)

## Summary
Remove one or more offers.

## Usage
```text
juju remove-offer [options] <offer-name-or-url> [<offer-name-or-url>...]
```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `-c`, `--controller` |  | Controller to operate in |
| `--force` | false | Remove the offer as well as any relations to the offer |
| `-y`, `--yes` | false | Skip the confirmation prompt for forced removal |

## Examples

    juju remove-offer staging/mymodel.hosted-mysql
    juju remove-offer staging/mymodel.hosted-mysql --force
    juju remove-offer hosted-mysql


## Details

Give an offer URL without an endpoint, or just its name in the current
model. To remove offers from different controllers, run the command once
per controller.

Use `--force` to remove an offer with connections. Only forced removal
prompts for confirmation; the prompt includes the offers and warns that
their relations will also be removed.

Further reading:

- https://canonical.com/juju/docs/juju-cli/latest/reference/offer/
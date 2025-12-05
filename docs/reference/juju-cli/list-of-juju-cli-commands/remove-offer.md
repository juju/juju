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
| `-B`, `--no-browser-login` | false | Disables web browser for authentication. |
| `-c`, `--controller` |  | Performs the operation in the specified controller. |
| `--force` | false | Removes the offer as well as any relations to the offer. |
| `-y`, `--yes` | false | Skips confirmation prompt. |

## Examples

    juju remove-offer prod.model/hosted-mysql
    juju remove-offer prod.model/hosted-mysql --force
    juju remove-offer hosted-mysql


## Details

Removes one or more application offers.

If the `--force` option is specified, any existing relations to the
offer will also be removed.

Offers to remove are normally specified by their URL.
It's also possible to specify just the offer name, in which case
the offer is considered to reside in the current model.
(command-juju-agreements)=
# `juju agreements`
> See also: [agree](#agree)

**Aliases:** list-agreements

## Summary
Lists user's agreements.

## Usage
```juju agreements [options] ```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Disables web browser for authentication. |
| `-c`, `--controller` |  | Performs the operation in the specified controller. |
| `--format` | tabular | Specify output format (json&#x7c;tabular&#x7c;yaml) |
| `-o`, `--output` |  | Specify an output file |

## Examples

    juju agreements


## Details

Lists the terms that the user has agreed to.

Charms may require a user to accept its terms in order for it to be deployed.
In other words, some applications may only be installed if a user agrees to 
accept some terms defined by the charm.
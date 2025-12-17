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
| `-B`, `--no-browser-login` | false | Specifies whether to skip web browser for authentication. |
| `-c`, `--controller` |  | Specifies the controller to operate in. |
| `--format` | tabular | Specify output format (json&#x7c;tabular&#x7c;yaml) |
| `-o`, `--output` |  | Specify an output file |

## Examples

    juju agreements


## Details

Charms may require a user to accept its terms in order for it to be deployed.
In other words, some applications may only be installed if a user agrees to 
accept some terms defined by the charm. 

This command lists the terms that the user has agreed to.
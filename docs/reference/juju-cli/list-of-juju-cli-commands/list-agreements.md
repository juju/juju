(command-juju-list-agreements)=
# `juju list-agreements`
> See also: [agree](#agree)

**Aliases:** list-agreements

## Summary
List user's agreements.

## Usage
```juju agreements [options] ```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `-c`, `--controller` |  | Controller to operate in |
| `--format` | tabular | Specify output format (json&#x7c;tabular&#x7c;yaml) |
| `-o`, `--output` |  | Specify an output file |

## Examples

    juju agreements


## Details

Charms may require a user to accept its terms in order for it to be deployed.
In other words, some applications may only be installed if a user agrees to 
accept some terms defined by the charm. 

This command lists the terms that the user has agreed to.
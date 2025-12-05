(command-juju-show-controller)=
# `juju show-controller`
> See also: [controllers](#controllers)

## Summary
Shows detailed information about a controller.

## Usage
```juju show-controller [options] [<controller name> ...]```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Disables web browser for authentication. |
| `--format` | yaml | Specify output format (json&#x7c;yaml) |
| `-o`, `--output` |  | Specify an output file |
| `--show-password` | false | Shows the password for the logged in user. |

## Examples

    juju show-controller
    juju show-controller aws google


## Details
Shows extended information about a controller(s) as well as related models
and user login details.
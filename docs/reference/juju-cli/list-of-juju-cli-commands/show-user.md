(command-juju-show-user)=
# `juju show-user`
> See also: [add-user](#add-user), [register](#register), [users](#users)

## Summary
Shows information about a user.

## Usage
```juju show-user [options] [<user name>]```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-c`, `--controller` |  | Performs the operation in the specified controller. |
| `--exact-time` | false | Uses full timestamp for connection times. |
| `--format` | yaml | Specify output format (json&#x7c;yaml) |
| `-o`, `--output` |  | Specify an output file |

## Examples

    juju show-user
    juju show-user jsmith
    juju show-user --format json
    juju show-user --format yaml


## Details

By default, the `YAML`format is used and the user name is the current
user.
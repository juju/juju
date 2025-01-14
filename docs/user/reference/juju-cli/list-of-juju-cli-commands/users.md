(command-juju-users)=
# `juju users`
> See also: [add-user](#add-user), [register](#register), [show-user](#show-user), [disable-user](#disable-user), [enable-user](#enable-user)

**Aliases:** list-users

## Summary
Lists Juju users allowed to connect to a controller or model.

## Usage
```juju users [options] [model-name]```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `--all` | false | Include disabled users (on controller only) |
| `-c`, `--controller` |  | Controller to operate in |
| `--exact-time` | false | Use full timestamp for connection times |
| `--format` | tabular | Specify output format (json&#x7c;tabular&#x7c;yaml) |
| `-o`, `--output` |  | Specify an output file |

## Examples

Print the users relevant to the current controller:

    juju users
    
Print the users relevant to the controller "another":

    juju users -c another

Print the users relevant to the model "mymodel":

    juju users mymodel


## Details
When used without a model name argument, users relevant to a controller are printed.
When used with a model name, users relevant to the specified model are printed.
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
| `--all` | false | Includes disabled users. |
| `-c`, `--controller` |  | Performs the operation in the specified controller. |
| `--exact-time` | false | Uses full timestamp for connection times. |
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
Prints users relevant to a controller when used without a model name argument.
Prints users relevant to the specified model when used with a model name.
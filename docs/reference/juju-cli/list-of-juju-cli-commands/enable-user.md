(command-juju-enable-user)=
# `juju enable-user`
> See also: [users](#users), [disable-user](#disable-user), [login](#login)

## Summary
Re-enable a previously disabled Juju user.

## Usage
```juju enable-user [options] <user name>```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Disables web browser for authentication. |
| `-c`, `--controller` |  | Performs the operation in the specified controller. |

## Examples

    juju enable-user bob


## Details
An enabled Juju user is one that can log in to a controller.
(command-juju-update-public-clouds)=
# `juju update-public-clouds`
> See also: [clouds](#clouds)

## Summary
Updates the public cloud information available to Juju.

## Usage
```juju update-public-clouds [options] ```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Disables web browser for authentication. |
| `-c`, `--controller` |  | Performs the operation in the specified controller. |
| `--client` | false | Performs the operation on the local client. |

## Examples

    juju update-public-clouds
    juju update-public-clouds --client
    juju update-public-clouds --controller mycontroller


## Details

If any new information for public clouds (such as regions and connection
endpoints) are available this command will update Juju accordingly. It is
suggested to run this command periodically.

The `--controller` option can be used to update public cloud(s) on a controller. The command
will only update the clouds that a controller knows about.

The `--client` option can be used to update a definition of public cloud(s) on this client.
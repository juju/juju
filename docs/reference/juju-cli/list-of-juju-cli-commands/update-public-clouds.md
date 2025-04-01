(command-juju-update-public-clouds)=
# `juju update-public-clouds`
> See also: [clouds](#clouds)

## Summary
Updates public cloud information available to Juju.

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `-c`, `--controller` |  | Controller to operate in |
| `--client` | false | Client operation |

## Examples

    juju update-public-clouds
    juju update-public-clouds --client
    juju update-public-clouds --controller mycontroller


## Details

If any new information for public clouds (such as regions and connection
endpoints) are available this command will update Juju accordingly. It is
suggested to run this command periodically.

Use --controller option to update public cloud(s) on a controller. The command
will only update the clouds that a controller knows about. 

Use --client to update a definition of public cloud(s) on this client.
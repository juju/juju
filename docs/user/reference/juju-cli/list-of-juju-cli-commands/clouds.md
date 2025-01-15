(command-juju-clouds)=
# `juju clouds`
> See also: [add-cloud](#add-cloud), [credentials](#credentials), [controllers](#controllers), [regions](#regions), [default-credential](#default-credential), [default-region](#default-region), [show-cloud](#show-cloud), [update-cloud](#update-cloud), [update-public-clouds](#update-public-clouds)

**Aliases:** list-clouds

## Summary
Lists all clouds available to Juju.

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `--all` | false | Show all available clouds |
| `-c`, `--controller` |  | Controller to operate in |
| `--client` | false | Client operation |
| `--format` | tabular | Specify output format (json&#x7c;tabular&#x7c;yaml) |
| `-o`, `--output` |  | Specify an output file |

## Examples

    juju clouds
    juju clouds --format yaml
    juju clouds --controller mycontroller 
    juju clouds --controller mycontroller --client
    juju clouds --client


## Details
Display the fundamental properties for each cloud known to Juju:
name, number of regions, number of registered credentials, default region, type, etc...

Clouds known to this client are the clouds known to Juju out of the box 
along with any which have been added with `add-cloud --client`. These clouds can be
used to create a controller and can be displayed using --client option.

Clouds may be listed that are co-hosted with the Juju client.  When the LXD hypervisor
is detected, the 'localhost' cloud is made available.  When a microk8s installation is
detected, the 'microk8s' cloud is displayed.

Use --controller option to list clouds from a controller. 
Use --client option to list clouds from this client. 
This command's default output format is 'tabular'. Use 'json' and 'yaml' for
machine-readable output.

Cloud metadata sometimes changes, e.g. providers add regions. Use the `update-public-clouds`
command to update public clouds or `update-cloud` to update other clouds.

Use the `regions` command to list a cloud's regions.

Use the `show-cloud` command to get more detail, such as regions and endpoints.

Further reading:
 
    Documentation:   https://juju.is/docs/olm/manage-clouds
    microk8s:        https://microk8s.io/docs
    LXD hypervisor:  https://documentation.ubuntu.com/lxd
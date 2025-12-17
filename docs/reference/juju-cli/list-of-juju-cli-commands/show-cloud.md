(command-juju-show-cloud)=
# `juju show-cloud`
> See also: [clouds](#clouds), [add-cloud](#add-cloud), [update-cloud](#update-cloud)

## Summary
Shows detailed information for a cloud.

## Usage
```juju show-cloud [options] <cloud name>```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Specifies whether to skip web browser for authentication. |
| `-c`, `--controller` |  | Specifies the controller to operate in. |
| `--client` | false | Specifies whether to perform the operation on the local client. |
| `--format` | display | Specify output format (display&#x7c;json&#x7c;yaml) |
| `--include-config` | false | Specifies whether to print available config option details specific to the specified cloud. |
| `-o`, `--output` |  | Specify an output file |

## Examples

    juju show-cloud google
    juju show-cloud azure-china --output ~/azure_cloud_details.txt
    juju show-cloud myopenstack --controller mycontroller
    juju show-cloud myopenstack --client
    juju show-cloud myopenstack --client --controller mycontroller


## Details

Provided information includes `defined` (public, built-in), `type`,
`auth-type`, `regions`, `endpoints`, and cloud specific configuration
options.

If `--include-config` is used, additional configuration (key, type, and
description) specific to the cloud are displayed if available.

Use the `--controller` option to show a cloud from a controller.

Use the `--client` option to show a cloud known on this client.
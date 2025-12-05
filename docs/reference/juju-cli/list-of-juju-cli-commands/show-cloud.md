(command-juju-show-cloud)=
# `juju show-cloud`
> See also: [clouds](#clouds), [add-cloud](#add-cloud), [update-cloud](#update-cloud)

## Summary
View detailed information about a cloud.

## Usage
```juju show-cloud [options] <cloud name>```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Disables web browser for authentication. |
| `-c`, `--controller` |  | Performs the operation in the specified controller. |
| `--client` | false | Performs the operation on the local client. |
| `--format` | display | Specify output format (display&#x7c;json&#x7c;yaml) |
| `--include-config` | false | Prints available config option details specific to the specified cloud. |
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

The `--controller` option can be used to show a cloud from a controller.

The `--client` option can be used to show a cloud known on this client.
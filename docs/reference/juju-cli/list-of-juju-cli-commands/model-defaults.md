(command-juju-model-defaults)=
# `juju model-defaults`
> See also: [models](#models), [model-config](#model-config)

**Aliases:** model-default

## Summary
Displays or sets default configuration settings for new models.

## Usage
```juju model-defaults [options] [<model-key>[<=value>] ...]```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `-c`, `--controller` |  | Controller to operate in |
| `--cloud` |  | The cloud to target |
| `--color` | false | Use ANSI color codes in output |
| `--file` |  | Path to yaml-formatted configuration file |
| `--format` | tabular | Specify output format (json&#x7c;tabular&#x7c;yaml) |
| `--ignore-read-only-fields` | false | Ignore read only fields that might cause errors to be emitted while processing yaml documents |
| `--no-color` | false | Disable ANSI color codes in tabular output |
| `-o`, `--output` |  | Specify an output file |
| `--region` |  | The region or cloud/region to target |
| `--reset` |  | Reset the provided comma delimited keys |

## Examples

Display all model config default values:

    juju model-defaults

Display the value of http-proxy model config default:

    juju model-defaults http-proxy

Display the value of http-proxy model config default for the aws cloud:

    juju model-defaults --cloud=aws http-proxy

Display the value of http-proxy model config default for the aws cloud
and us-east-1 region:

    juju model-defaults --region=aws/us-east-1 http-proxy

Display the value of http-proxy model config default for the us-east-1 region:

    juju model-defaults --region=us-east-1 http-proxy

Set the value of ftp-proxy model config default to 10.0.0.1:8000:

    juju model-defaults ftp-proxy=10.0.0.1:8000

Set the value of ftp-proxy model config default to 10.0.0.1:8000 in the
us-east-1 region:

    juju model-defaults --region=us-east-1 ftp-proxy=10.0.0.1:8000

Set model default values for the aws cloud as defined in path/to/file.yaml:

    juju model-defaults --cloud=aws --file path/to/file.yaml

Reset the value of default-base and test-mode to default:

    juju model-defaults --reset default-base,test-mode

Reset the value of http-proxy for the us-east-1 region to default:

    juju model-defaults --region us-east-1 --reset http-proxy


## Details

To view all model default values for the current controller:

    juju model-defaults

You can target a specific controller using the `-c` flag:

    juju model-defaults -c <controller>

By default, the output will be printed in a tabular format. You can instead
print it in json or yaml format using the `--format` flag:

    juju model-defaults --format json
    juju model-defaults --format yaml

To view the value of a single model default:

    juju model-defaults key

To set default model config values:

    juju model-defaults key1=val1 key2=val2 ...

You can also reset default keys to their original values:

    juju model-defaults --reset key1
    juju model-defaults --reset key1,key2,key3

You may simultaneously set some keys and reset others:

    juju model-defaults key1=val1 key2=val2 --reset key3,key4

Default values can be imported from a yaml file using the `--file` flag:

    juju model-defaults --file=path/to/cfg.yaml

This allows you to e.g. save a controller's model defaults to a file:

    juju model-defaults --format=yaml > cfg.yaml

and then import these later. Note that the output of `model-defaults` may

include read-only values, which will cause an error when importing later.
To prevent the error, use the `--ignore-read-only-fields` flag:

    juju model-defaults --file=cfg.yaml --ignore-read-only-fields

You can also read from `stdin` using `-`, which allows you to pipe default model
values from one controller to another:

    juju model-defaults -c c1 --format=yaml \
      | juju model-defaults -c c2 --file=- --ignore-read-only-fields

You can simultaneously read config from a yaml file and set config keys
as above. The command-line args will override any values specified in the file.

Model default configuration settings are specific to the cloud on which the
model is deployed. If the controller hosts more than one cloud, the cloud
(and optionally region) must be specified using the `--cloud` flag. This flag
accepts arguments in the following forms:

    --cloud=<cloud>                    (specified cloud, all regions)
    --region=<region>               (default cloud, specified region)
    --region=<cloud>/<region>            (specified cloud and region)
    --cloud=<cloud> --region=<region>    (specified cloud and region)
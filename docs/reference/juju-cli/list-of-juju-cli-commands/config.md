(command-juju-config)=
# `juju config`
> See also: [deploy](#deploy), [status](#status), [model-config](#model-config), [controller-config](#controller-config)

## Summary
Get, set, or reset configuration for a deployed application.

## Usage
```juju config [options] <application name> [--reset <key[,key]>] [<attribute-key>][=<value>] ...]```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `--color` | false | Use ANSI color codes in output |
| `--file` |  | path to yaml-formatted configuration file |
| `--format` | yaml | Specify output format (json&#x7c;yaml) |
| `-m`, `--model` |  | Model to operate in. Accepts [&lt;controller name&gt;:]&lt;model name&gt;&#x7c;&lt;model UUID&gt; |
| `--no-color` | false | Disable ANSI color codes in tabular output |
| `-o`, `--output` |  | Specify an output file |
| `--reset` |  | Reset the provided comma delimited keys |

## Examples

To view all configuration values for an application, run

    juju config mysql --format json

To set a configuration value for an application, run

    juju config mysql foo=bar

To set some keys and reset others:

    juju config mysql key1=val1 key2=val2 --reset key3,key4

To set a configuration value for an application from a file:

    juju config mysql --file=path/to/cfg.yaml


## Details

To view all configuration values for an application:

    juju config <app>

By default, the config will be printed in a `yaml` format. You can instead print it
in a `json` format using the `--format` flag:

    juju config <app> --format json

To view the value of a single config key, run

    juju config <app> key

To set config values, run

    juju config <app> key1=val1 key2=val2 ...

This sets "key1" to "val1", etc. Using the `@` directive, you can set a config
key's value to the contents of a file:

    juju config <app> key=@/tmp/configvalue

You can also reset config keys to their default values:

    juju config <app> --reset key1
    juju config <app> --reset key1,key2,key3

You may simultaneously set some keys and reset others:

    juju config <app> key1=val1 key2=val2 --reset key3,key4

Config values can be imported from a yaml file using the --file flag:

    juju config <app> --file=path/to/cfg.yaml

The `yaml` file should be in the following format:

    apache2:                        # application name
      servername: "example.com"     # key1: val1
      lb_balancer_timeout: 60       # key2: val2
      ...

This allows you to, e.g., save an app's config to a file:

    juju config app1 > cfg.yaml

and then import the config later. You can also read from stdin using `-`,
which allows you to pipe config values from one app to another:

    juju config app1 | juju config app2 --file -

You can simultaneously read config from a yaml file and set/reset config keys
as above. The command-line args will override any values specified in the file.

Rather than specifying each setting name/value inline, the `--file` flag option
may be used to provide a list of settings to be updated as a yaml file. The
yaml file contents must include a single top-level key with the application's
name followed by a dictionary of key/value pairs that correspond to the names
and values of the settings to be set. For instance, to configure apache2,
the following yaml file can be used:

    apache2:
      servername: "example.com"
      lb_balancer_timeout: 60

If the above `yaml` document is stored in a file called `config.yaml`, the
following command can be used to apply the config changes:

    juju config apache2 --file config.yaml

Finally, the `--reset` flag can be used to revert one or more configuration
settings back to their default value as defined in the charm metadata:

    juju config apache2 --reset servername
    juju config apache2 --reset servername,lb_balancer_timeout
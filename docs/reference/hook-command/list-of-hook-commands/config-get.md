(hook-command-config-get)=
# `config-get`
## Summary
Print application configuration.

## Usage
``` config-get [options] [<key>]```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-a`, `--all` | false | print all keys |
| `--format` | smart | Specify output format (json&#x7c;smart&#x7c;yaml) |
| `-o`, `--output` |  | Specify an output file |

## Examples

    INTERVAL=$(config-get interval)

    config-get --all


## Details

config-get returns information about the application configuration
(as defined by config.yaml). If called without arguments, it returns
a dictionary containing all config settings that are either explicitly
set, or which have a non-nil default value. If the --all flag is passed,
it returns a dictionary containing all defined config settings including
nil values (for those without defaults). If called with a single argument,
it returns the value of that config key. Missing config keys are reported
as nulls, and do not return an error.

&lt;key&gt; and --all are mutually exclusive.
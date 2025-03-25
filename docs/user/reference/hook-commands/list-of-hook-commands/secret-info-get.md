(hook-command-secret-info-get)=
# `secret-info-get`
## Summary
Get a secret's metadata info.

## Usage
``` secret-info-get [options] <ID>```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `--format` | yaml | Specify output format (json&#x7c;yaml) |
| `--label` |  | a label used to identify the secret |
| `-o`, `--output` |  | Specify an output file |

## Examples

    secret-info-get secret:9m4e2mr0ui3e8a215n4g
    secret-info-get --label db-password


## Details

Get the metadata of a secret with a given secret ID.
Either the ID or label can be used to identify the secret.
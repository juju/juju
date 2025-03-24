(hook-command-secret-get)=
# `secret-get`
## Summary
Get the content of a secret.

## Usage
``` secret-get [options] <ID> [key[#base64]]```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `--format` | yaml | Specify output format (json&#x7c;yaml) |
| `--label` |  | a label used to identify the secret in hooks |
| `-o`, `--output` |  | Specify an output file |
| `--peek` | false | get the latest revision just this time |
| `--refresh` | false | get the latest revision and also get this same revision for subsequent calls |

## Examples

    secret-get secret:9m4e2mr0ui3e8a215n4g
    secret-get secret:9m4e2mr0ui3e8a215n4g token
    secret-get secret:9m4e2mr0ui3e8a215n4g token#base64
    secret-get secret:9m4e2mr0ui3e8a215n4g --format json
    secret-get secret:9m4e2mr0ui3e8a215n4g --peek
    secret-get secret:9m4e2mr0ui3e8a215n4g --refresh
    secret-get secret:9m4e2mr0ui3e8a215n4g --label db-password


## Details

Get the content of a secret with a given secret ID.
The first time the value is fetched, the latest revision is used.
Subsequent calls will always return this same revision unless
--peek or --refresh are used.
Using --peek will fetch the latest revision just this time.
Using --refresh will fetch the latest revision and continue to
return the same revision next time unless --peek or --refresh is used.

Either the ID or label can be used to identify the secret.
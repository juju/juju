(hook-command-secret-add)=
# `secret-add`

## Summary
Add a new secret.

## Usage
``` secret-add [options] [key[#base64|#file]=value...]```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `--description` |  | the secret description |
| `--expire` |  | either a duration or time when the secret should expire |
| `--file` |  | a YAML file containing secret key values |
| `--label` |  | a label used to identify the secret in hooks |
| `--owner` | application | the owner of the secret, either the application or unit |
| `--rotate` |  | the secret rotation policy |

## Examples

    secret-add token=34ae35facd4
    secret-add key#base64=AA==
    secret-add key#file=/path/to/file another-key=s3cret
    secret-add --owner unit token=s3cret 
    secret-add --rotate monthly token=s3cret 
    secret-add --expire 24h token=s3cret 
    secret-add --expire 2025-01-01T06:06:06 token=s3cret 
    secret-add --label db-password \
        --description "my database password" \
        data#base64=s3cret== 
    secret-add --label db-password \
        --description "my database password" \
        --file=/path/to/file


## Details

Add a secret with a list of key values.

If a key has the '#base64' suffix, the value is already in base64 format and no
encoding will be performed, otherwise the value will be base64 encoded
prior to being stored.

If a key has the '#file' suffix, the value is read from the corresponding file.

By default, a secret is owned by the application, meaning only the unit
leader can manage it. Use "--owner unit" to create a secret owned by the
specific unit which created it.
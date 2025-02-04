(hook-command-secret-set)=
# `secret-set`

## Summary
Update an existing secret.

## Usage
``` secret-set [options] <ID> [key[#base64]=value...]```

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

    secret-set secret:9m4e2mr0ui3e8a215n4g token=34ae35facd4
    secret-set secret:9m4e2mr0ui3e8a215n4g key#base64 AA==
    secret-set secret:9m4e2mr0ui3e8a215n4g --rotate monthly token=s3cret 
    secret-set secret:9m4e2mr0ui3e8a215n4g --expire 24h
    secret-set secret:9m4e2mr0ui3e8a215n4g --expire 24h token=s3cret 
    secret-set secret:9m4e2mr0ui3e8a215n4g --expire 2025-01-01T06:06:06 token=s3cret 
    secret-set secret:9m4e2mr0ui3e8a215n4g --label db-password \
        --description "my database password" \
        data#base64 s3cret== 
    secret-set secret:9m4e2mr0ui3e8a215n4g --label db-password \
        --description "my database password"
    secret-set secret:9m4e2mr0ui3e8a215n4g --label db-password \
        --description "my database password" \
        --file=/path/to/file


## Details

Update a secret with a list of key values, or set new metadata.
If a value has the '#base64' suffix, it is already in base64 format and no
encoding will be performed, otherwise the value will be base64 encoded
prior to being stored.
To just update selected metadata like rotate policy, do not specify any secret value.
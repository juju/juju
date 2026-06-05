(command-juju-update-secret)=
# `juju update-secret`
## Summary
Update an existing secret.

## Usage
```juju update-secret [options] <ID>|<name> [key[#base64|#file]=value...]```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `--auto-prune` | nil | Used to allow Juju to automatically remove revisions which are no longer being tracked by any observers |
| `--file` |  | A YAML file containing secret key values |
| `--info` |  | The secret description |
| `-m`, `--model` |  | Model to operate in. Accepts [&lt;controller name&gt;:]&lt;model name&gt;&#x7c;&lt;model UUID&gt; |
| `--name` |  | The new secret name |

## Examples

    juju update-secret secret:9m4e2mr0ui3e8a215n4g token=34ae35facd4
    juju update-secret secret:9m4e2mr0ui3e8a215n4g key#base64 AA==
    juju update-secret secret:9m4e2mr0ui3e8a215n4g token=34ae35facd4 --auto-prune
    juju update-secret secret:9m4e2mr0ui3e8a215n4g --name db-password \
        --info "my database password" \
        data#base64 s3cret==
    juju update-secret db-pass --name db-password \
        --info "my database password"
    juju update-secret secret:9m4e2mr0ui3e8a215n4g --name db-password \
        --info "my database password" \
        --file=/path/to/file


## Details

Update a secret with a list of key values, or info.

If a value has the `#base64` suffix, it is already in `base64` format and no
encoding will be performed, otherwise the value will be base64 encoded
prior to being stored.

The `--auto-prune` option is used to allow Juju to automatically remove revisions
which are no longer being tracked by any observers (see Rotation and Expiry).
This is configured per revision. This feature is opt-in because Juju
automatically removing secret content might result in data loss.
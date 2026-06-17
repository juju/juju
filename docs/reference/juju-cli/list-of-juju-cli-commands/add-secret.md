(command-juju-add-secret)=
# `juju add-secret`
## Summary
Add a new secret.

## Usage
```juju add-secret [options] <name> [key[#base64|#file]=value...]```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `--file` |  | A YAML file containing secret key values |
| `--info` |  | The secret description |
| `-m`, `--model` |  | Model to operate in. Accepts [&lt;controller name&gt;:]&lt;model name&gt;&#x7c;&lt;model UUID&gt; |

## Examples

    juju add-secret my-apitoken token=34ae35facd4
    juju add-secret my-secret key#base64=AA==
    juju add-secret my-secret key#file=/path/to/file another-key=s3cret
    juju add-secret db-password \
        --info "my database password" \
        data#base64=s3cret==
    juju add-secret db-password \
        --info "my database password" \
        --file=/path/to/file


## Details

Add a secret with a list of key values.

If a key has the `#base64` suffix, the value is already in base64 format and no
encoding will be performed, otherwise the value will be base64 encoded
prior to being stored.

If a key has the `#file` suffix, the value is read from the corresponding file.

A secret is owned by the model, meaning only the model admin
can manage it, ie grant/revoke access, update, remove etc.
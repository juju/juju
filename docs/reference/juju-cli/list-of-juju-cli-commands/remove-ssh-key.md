(command-juju-remove-ssh-key)=
# `juju remove-ssh-key`
> See also: [ssh-keys](#ssh-keys), [add-ssh-key](#add-ssh-key), [import-ssh-key](#import-ssh-key)

## Summary
Removes a public SSH key (or keys) from a model.

## Usage
```juju remove-ssh-key [options] <ssh key id> ...```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `-m`, `--model` |  | Model to operate in. Accepts [&lt;controller name&gt;:]&lt;model name&gt;&#x7c;&lt;model UUID&gt; |

## Examples

    juju remove-ssh-key ubuntu@ubuntu
    juju remove-ssh-key 45:7f:33:2c:10:4e:6c:14:e3:a1:a4:c8:b2:e1:34:b4
    juju remove-ssh-key bob@ubuntu carol@ubuntu


## Details
Juju maintains a per-model cache of public SSH keys which it copies to
each unit. This command will remove a specified key (or space-separated
list of keys) from the model cache and all current units deployed in that
model. The keys to be removed may be specified by the key's fingerprint,
or by the text label associated with them. Invalid keys in the model cache
can be removed by specifying the key verbatim.
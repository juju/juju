(command-juju-ssh-keys)=
# `juju ssh-keys`
> See also: [add-ssh-key](#add-ssh-key), [remove-ssh-key](#remove-ssh-key)

**Aliases:** list-ssh-keys

## Summary
Lists the currently known SSH keys for the current (or specified) model.

## Usage
```juju ssh-keys [options] ```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `--full` | false | Show full key instead of just the fingerprint |
| `-m`, `--model` |  | Model to operate in. Accepts [&lt;controller name&gt;:]&lt;model name&gt;&#x7c;&lt;model UUID&gt; |

## Examples

    juju ssh-keys

To examine the full information for each key:

    juju ssh-keys -m jujutest --full


## Details

Juju maintains a per-model cache of SSH keys which it copies to each newly
created unit.

This command will display a list of all the keys currently used by Juju in
the current model (or the model specified, if the `-m` option is used).

By default a minimal list is returned, showing only the fingerprint of
each key and its text identifier. By using the `--full`option, the entire
key may be displayed.
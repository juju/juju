(command-juju-ssh-keys)=
# `juju ssh-keys`
> See also: [add-ssh-key](#command-juju-add-ssh-key), [remove-ssh-key](#command-juju-remove-ssh-key)

**Aliases:** list-ssh-keys

## Summary
Lists your authorized SSH keys in a model.

## Usage
```text
juju ssh-keys [options]
```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `--full` | false | Show full key instead of just the fingerprint |
| `-m`, `--model` |  | Model to operate in. Accepts [&lt;controller name&gt;:]&lt;model name&gt;&#x7c;&lt;model UUID&gt; |

## Examples

List the fingerprints of your registered SSH keys in the current model:

    juju ssh-keys

List your full public SSH keys in the `jujutest` model:

    juju ssh-keys -m jujutest --full


## Details
The command prints the model name followed by one fingerprint per line.
To print public keys instead of fingerprints, use `--full`.
If the list is empty, the command reports that there are no keys to display.

Further reading:

- https://canonical.com/juju/docs/juju-cli/latest/reference/ssh-key/
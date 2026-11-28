(command-juju-remove-ssh-key)=
# `juju remove-ssh-key`
> See also: [ssh-keys](#command-juju-ssh-keys), [add-ssh-key](#command-juju-add-ssh-key), [import-ssh-key](#command-juju-import-ssh-key)

## Summary
Removes a public SSH key (or keys) from a model.

## Usage
```text
juju remove-ssh-key [options] <ssh key id> ...
```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `-m`, `--model` |  | Model to operate in. Accepts [&lt;controller name&gt;:]&lt;model name&gt;&#x7c;&lt;model UUID&gt; |

## Examples

Remove the key that matches the contents of a public key file:

    juju remove-ssh-key "$(cat ~/.ssh/id_ed25519.pub)"


## Details
Pass each fingerprint, key comment, or full public key as a separate argument.
Quote arguments containing spaces. To select one key, use its fingerprint
from the output of juju ssh-keys.
Check standard error for key errors even if the command exits successfully.

Further reading:

- https://canonical.com/juju/docs/juju-cli/latest/reference/ssh-key/
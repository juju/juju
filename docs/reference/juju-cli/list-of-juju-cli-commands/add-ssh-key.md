(command-juju-add-ssh-key)=
# `juju add-ssh-key`
> See also: [ssh-keys](#command-juju-ssh-keys), [remove-ssh-key](#command-juju-remove-ssh-key), [import-ssh-key](#command-juju-import-ssh-key)

## Summary
Adds a public SSH key to a model.

## Usage
```text
juju add-ssh-key [options] <ssh key> ...
```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `-m`, `--model` |  | Model to operate in. Accepts [&lt;controller name&gt;:]&lt;model name&gt;&#x7c;&lt;model UUID&gt; |

## Examples

Add the public key stored in a file:

    juju add-ssh-key "$(cat ~/.ssh/id_ed25519.pub)"



## Details
Pass the text of each public key as a separate quoted argument, not a filename.
Check standard error: the command can report a key error without a nonzero exit status.

Further reading:

- https://canonical.com/juju/docs/juju-cli/latest/reference/ssh-key/
(command-juju-import-ssh-key)=
# `juju import-ssh-key`
> See also: [add-ssh-key](#command-juju-add-ssh-key), [ssh-keys](#command-juju-ssh-keys)

## Summary
Imports public SSH keys from GitHub or Launchpad into a model.

## Usage
```text
juju import-ssh-key [options] <lp|gh>:<user identity> ...
```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `-m`, `--model` |  | Model to operate in. Accepts [&lt;controller name&gt;:]&lt;model name&gt;&#x7c;&lt;model UUID&gt; |

## Examples

With GITHUB_USER set to the GitHub account whose keys you want to authorize:

    juju import-ssh-key "gh:${GITHUB_USER:?set GITHUB_USER first}"


## Details
Specify each external account as gh:USERNAME for GitHub or lp:USERNAME for
Launchpad. The command requests the import for the Juju account named admin.
Check standard error for individual source errors even if the command exits successfully.

Further reading:

- https://canonical.com/juju/docs/juju-cli/latest/reference/ssh-key/
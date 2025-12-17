(command-juju-import-ssh-key)=
# `juju import-ssh-key`
> See also: [add-ssh-key](#add-ssh-key), [ssh-keys](#ssh-keys)

## Summary
Adds a public SSH key from a trusted identity source to a model.

## Usage
```juju import-ssh-key [options] <lp|gh>:<user identity> ...```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Specifies whether to skip web browser for authentication. |
| `-m`, `--model` |  | Specifies the model to operate in. Accepts `[<controller name>:]<model name>|<model UUID>`. |

## Examples

Import all public keys associated with user account 'phamilton' on the
GitHub service:

    juju import-ssh-key gh:phamilton

Multiple identities may be specified in a space delimited list:

    juju import-ssh-key gh:rheinlein lp:iasmiov gh:hharrison


## Details
Juju can add SSH keys to its cache from reliable public sources (currently
Launchpad and GitHub), allowing those users SSH access to Juju machines.

The user identity supplied is the username on the respective service given by
'lp:' or 'gh:'.

If the user has multiple keys on the service, all the keys will be added.

Once the keys are imported, they can be viewed with the `juju ssh-keys`
command, where comments will indicate which ones were imported in
this way.

An alternative to this command is the more manual `juju add-ssh-key`.
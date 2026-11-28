(command-juju-remove-secret-backend)=
# `juju remove-secret-backend`
> See also: [add-secret-backend](#command-juju-add-secret-backend), [secret-backends](#command-juju-secret-backends), [show-secret-backend](#command-juju-show-secret-backend), [update-secret-backend](#command-juju-update-secret-backend)

## Summary
Removes a secret backend from the controller.

## Usage
```text
juju remove-secret-backend [options] <backend-name>
```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-c`, `--controller` |  | Controller to operate in |
| `--force` | false | Remove a user-added backend even if revisions still reference it |

## Examples

Once no secret revisions reference `myvault`, remove its registration from the controller:

    juju remove-secret-backend myvault


## Details

Removes the named registration without a confirmation prompt. For the built-in
backends (internal and kubernetes), the message that the backend still contains
secret content means removal is not allowed, even when the backend is empty.
Leave these backends in place. For a user-added backend, the same message means
revisions still reference it. Check those references before retrying.

With `--force`, Juju can lose access to user and charm secret revisions without
moving or erasing the remote values. Force cannot remove a built-in. On success
this command prints nothing; models selecting the removed backend revert to auto.

Further reading:

- https://canonical.com/juju/docs/juju-cli/latest/reference/secret-backend/
(command-juju-remove-credential)=
# `juju remove-credential`
> See also: [add-credential](#command-juju-add-credential), [autoload-credentials](#command-juju-autoload-credentials), [credentials](#command-juju-credentials), [default-credential](#command-juju-default-credential), [set-credential](#command-juju-set-credential), [update-credential](#command-juju-update-credential)

## Summary
Removes Juju credentials for a cloud.

## Usage
```text
juju remove-credential [options] <cloud name> <credential name>
```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `-c`, `--controller` |  | Controller to operate in |
| `--client` | false | Client operation |
| `--force` | false | Remove the controller credential even if models use it, clearing their credential bindings |

## Examples

Remove a credential on this client:

    juju remove-credential google production --client

Remove an unused credential on the controller:

    juju remove-credential google production -c mycontroller



## Details
The credential to be removed is specified by a credential name.
Credential names, and optionally the corresponding authentication
material, can be listed with `juju credentials`.

Use `--controller` to remove the credential under the account signed in
to that controller. The credential name does not select another owner.

A controller refuses ordinary removal while a model uses the credential.
Switch those models to a replacement with juju set-credential, then retry.
Forcing removal clears all remaining model bindings without replacing them.
Removing the credential in Juju does not revoke the key at the provider.

Use `--client` to remove the local copy. Supply both store flags
to remove both copies.

Further reading:

- https://canonical.com/juju/docs/juju-cli/latest/reference/credential/
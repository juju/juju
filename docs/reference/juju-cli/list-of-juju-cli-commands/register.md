(command-juju-register)=
# `juju register`
> See also: [add-user](#command-juju-add-user), [change-user-password](#command-juju-change-user-password), [unregister](#command-juju-unregister)

## Summary
Registers a controller.

## Usage
```text
juju register [options] <registration string>|<controller host name>
```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `--replace` | false | Replace a saved controller entry under its local name |

## Examples

Activate your local account. Set INVITATION to the argument in the
administrator's juju register command:

    juju register "$INVITATION"

Update a signed-out controller entry under its existing local name. Set
INVITATION to the argument in the administrator's new juju register command:

    juju register --replace "$INVITATION"

Register a public controller by hostname:

    juju register public-controller.example.com


## Details

Register a controller to save its connection details on this client.

To activate a local account, use the argument from the administrator's
`juju register` command. Choose a password when prompted. The controller
consumes the invitation when it accepts the password. Ask a model
administrator for access to an existing model if you need one.

Use `--replace` to update a saved controller entry under its existing local
name. If this client has an account saved for that controller, sign out first.

For a public controller, give its hostname and follow the sign-in prompts.
The CLI uses port 443 unless you specify another port. If the public endpoint
is already saved on this client, use `juju login -c NAME` with its saved name.

Further reading:

- https://canonical.com/juju/docs/juju-cli/latest/reference/user/
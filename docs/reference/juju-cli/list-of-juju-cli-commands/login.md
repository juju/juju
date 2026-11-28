(command-juju-login)=
# `juju login`
> See also: [disable-user](#command-juju-disable-user), [enable-user](#command-juju-enable-user), [logout](#command-juju-logout), [register](#command-juju-register), [unregister](#command-juju-unregister)

## Summary
Logs a user in to a controller.

## Usage
```text
juju login [options] [controller host name or alias]
```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `-c`, `--controller` |  | Controller to operate in |
| `--no-prompt` | false | Skip interactive prompts and read a local password from standard input when needed |
| `--trust` | false | Trust an unverified public controller CA in non-interactive mode only |
| `-u`, `--user` |  | Log in as this local user |

## Examples

Sign in to a public controller by its directory alias:

    juju login somepubliccontroller

Sign in to a public controller by hostname:

    juju login public-controller.example.com

Sign in to the current controller as the local user bob:

    juju login -u bob


## Details

By default, the `juju login` command logs the user into a controller.
The argument to the command can be a public controller
host name or alias (see Aliases below).

If no argument is provided, the controller specified with
the `-c` argument will be used, or the current controller
if that's not provided.

On success, the current controller is switched to the logged-in
controller.

To sign in as a specific local user, pass `--user NAME`. If this client
has saved a different account for the controller, run `juju logout` there
first. For a saved OpenID Connect controller, omit `--user` and sign in
through its identity provider.

For non-interactive local password login, supply `--user NAME` if the CLI
cannot determine the username. For an unverified public controller CA,
verify its fingerprint when prompted. The `--trust` option applies only
with `--no-prompt`.

For a local password login, the controller issues a token valid for 24 hours.
If a controller request needs fresh authentication, log in again. Commands
that only read the client store do not require an active token.

### Aliases

Public controller aliases are provided by a directory service
that is queried to find the host name for a given alias.
The URL for the directory service may be configured
by setting the environment variable `JUJU_DIRECTORY`.

Further reading:

- https://canonical.com/juju/docs/juju-cli/latest/reference/user/
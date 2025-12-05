(command-juju-login)=
# `juju login`
> See also: [disable-user](#disable-user), [enable-user](#enable-user), [logout](#logout), [register](#register), [unregister](#unregister)

## Summary
Logs a user in to a controller.

## Usage
```juju login [options] [controller host name or alias]```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Disables web browser for authentication. |
| `-c`, `--controller` |  | Specifies the controller to operate in. |
| `--no-prompt` | false | Skips password prompt; instead, read a line from `stdin`. |
| `--trust` | false | Automatically trusts controller CA certificate. |
| `-u`, `--user` |  | Logs in as this local user. |

## Examples

    juju login somepubliccontroller
    juju login jimm.jujucharms.com
    juju login -u bob


## Details

Logs the user in to a controller.
The argument to the command can be a public controller
host name or alias (see Aliases below).

If no argument is provided, the controller specified with
the `-c` argument will be used, or the current controller
if that's not provided.

On success, the current controller is switched to the logged-in
controller.

If the user is already logged in, the `juju login` command does nothing
except verify that fact.

If the `-u` option is provided, the `juju login` command will attempt to log
into the controller as that user.

After login, a token ('macaroon') will become active. It has an expiration
time of 24 hours. Upon expiration, no further `juju` commands can be issued
and the user will be prompted to log in again.

### Aliases

Public controller aliases are provided by a directory service
that is queried to find the host name for a given alias.
The URL for the directory service may be configured
by setting the environment variable `JUJU_DIRECTORY`.
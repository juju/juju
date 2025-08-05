> See also: [disable-user](#disable-user), [enable-user](#enable-user), [logout](#logout), [register](#register), [unregister](#unregister)

## Summary
Logs a user in to a controller.

## Usage
```juju login [options] [controller host name or alias]```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `-c`, `--controller` |  | Controller to operate in |
| `--no-prompt` | false | don't prompt for password just read a line from stdin |
| `--trust` | false | automatically trust controller CA certificate |
| `-u`, `--user` |  | log in as this local user |

## Examples

    juju login somepubliccontroller
    juju login jimm.jujucharms.com
    juju login -u bob


## Details

By default, the juju login command logs the user into a controller.
The argument to the command can be a public controller
host name or alias (see Aliases below).

If no argument is provided, the controller specified with
the -c argument will be used, or the current controller
if that's not provided.

On success, the current controller is switched to the logged-in
controller.

If the user is already logged in, the juju login command does nothing
except verify that fact.

If the -u option is provided, the juju login command will attempt to log
into the controller as that user.

After login, a token ("macaroon") will become active. It has an expiration
time of 24 hours. Upon expiration, no further Juju commands can be issued
and the user will be prompted to log in again.

Aliases
-------

Public controller aliases are provided by a directory service
that is queried to find the host name for a given alias.
The URL for the directory service may be configured
by setting the environment variable JUJU_DIRECTORY.




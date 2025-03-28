(hook-command-relation-get)=
# `relation-get`
## Summary
Get relation settings.

## Usage
``` relation-get [options] <key> <unit id>```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `--app` | false | Get the relation data for the overall application, not just a unit |
| `--format` | smart | Specify output format (json&#x7c;smart&#x7c;yaml) |
| `-o`, `--output` |  | Specify an output file |
| `-r`, `--relation` |  | Specify a relation by id |

## Examples

    # Getting the settings of the default unit in the default relation is done with:
    $ relation-get
    username: jim
    password: "12345"

    # To get a specific setting from the default remote unit in the default relation
    $ relation-get username
    jim

    # To get all settings from a particular remote unit in a particular relation you
    $ relation-get -r database:7 - mongodb/5
    username: bob
    password: 2db673e81ffa264c


## Details

relation-get prints the value of a unit's relation setting, specified by key.
If no key is given, or if the key is "-", all keys and values will be printed.

A unit can see its own settings by calling "relation-get - MYUNIT", this will include
any changes that have been made with "relation-set".

When reading remote relation data, a charm can call relation-get --app - to get
the data for the application data bag that is set by the remote applications
leader.

Further details:
relation-get reads the settings of the local unit, or of any remote unit, in a given
relation (set with -r, defaulting to the current relation identifier, as in relation-set).
The first argument specifies the settings key, and the second the remote unit, which may
be omitted if a default is available (that is, when running a relation hook other
than -relation-broken).

If the first argument is omitted, a dictionary of all current keys and values will be
printed; all values are always plain strings without any interpretation. If you need to
specify a remote unit but want to see all settings, use - for the first argument.

The environment variable JUJU_REMOTE_UNIT stores the default remote unit.

You should never depend upon the presence of any given key in relation-get output.
Processing that depends on specific values (other than private-address) should be
restricted to -relation-changed hooks for the relevant unit, and the absence of a
remote unit’s value should never be treated as an error in the local unit.

In practice, it is common and encouraged for -relation-changed hooks to exit early,
without error, after inspecting relation-get output and determining the data is
inadequate; and for all other hooks to be resilient in the face of missing keys,
such that -relation-changed hooks will be sufficient to complete all configuration
that depends on remote unit settings.

Key value pairs for remote units that have departed remain accessible for the lifetime
of the relation.
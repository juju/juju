(hook-command-relation-set)=
# `relation-set`

## Summary
Set relation settings.

## Usage
``` relation-set [options] key=value [key=value ...]```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `--app` | false | pick whether you are setting "application" settings or "unit" settings |
| `--file` |  | file containing key-value pairs |
| `--format` |  | deprecated format flag |
| `-r`, `--relation` |  | specify a relation by id |

## Examples

    relation-set port=80 tuning=default

    relation-set -r server:3 username=jim password=12345


## Details

"relation-set" writes the local unit's settings for some relation.
If no relation is specified then the current relation is used. The
setting values are not inspected and are stored as strings. Setting
an empty string causes the setting to be removed. Duplicate settings
are not allowed.

If the unit is the leader, it can set the application settings using
"--app". These are visible to related applications via 'relation-get --app'
or by supplying the application name to 'relation-get' in place of
a unit name.

The --file option should be used when one or more key-value pairs are
too long to fit within the command length limit of the shell or
operating system. The file will contain a YAML map containing the
settings.  Settings in the file will be overridden by any duplicate
key-value arguments. A value of "-" for the filename means &lt;stdin&gt;.

Further details:
relation-set writes the local unit’s settings for some relation. If it’s not running in a
relation hook, -r needs to be specified. The value part of an argument is not inspected,
and is stored directly as a string. Setting an empty string causes the setting to be removed.

relation-set is the tool for communicating information between units of related applications.
By convention the charm that provides an interface is likely to set values, and a charm that
requires that interface will read values; but there is nothing enforcing this. Whatever
information you need to propagate for the remote charm to work must be propagated via relation-set,
with the single exception of the private-address key, which is always set before the unit joins.

For some charms you may wish to overwrite the private-address setting, for example if you’re
writing a charm that serves as a proxy for some external application. It is rarely a good idea
to remove that key though, as most charms expect that value to exist unconditionally and may
fail if it is not present.

All values are set in a transaction at the point when the hook terminates successfully
(i.e. the hook exit code is 0). At that point all changed values will be communicated to
the rest of the system, causing -changed hooks to run in all related units.

There is no way to write settings for any unit other than the local unit. However, any hook
on the local unit can write settings for any relation which the local unit is participating in.
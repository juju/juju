(hook-command-secret-grant)=
# `secret-grant`

## Summary
Grant access to a secret.

## Usage
``` secret-grant [options] <ID>```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-r`, `--relation` |  | the relation with which to associate the grant |
| `--unit` |  | the unit to grant access |

## Examples

    secret-grant secret:9m4e2mr0ui3e8a215n4g -r 0 --unit mediawiki/6
    secret-grant secret:9m4e2mr0ui3e8a215n4g --relation db:2


## Details

Grant access to view the value of a specified secret.
Access is granted in the context of a relation - unless revoked
earlier, once the relation is removed, so too is the access grant.

By default, all units of the related application are granted access.
Optionally specify a unit name to limit access to just that unit.
(hook-command-secret-revoke)=
# `secret-revoke`
## Summary
Revoke access to a secret.

## Usage
``` secret-revoke [options] <ID>```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `--app`, `--application` |  | the application to revoke access |
| `-r`, `--relation` |  | the relation for which to revoke the grant |
| `--unit` |  | the unit to revoke access |

## Examples

    secret-revoke secret:9m4e2mr0ui3e8a215n4g
    secret-revoke secret:9m4e2mr0ui3e8a215n4g --relation 1
    secret-revoke secret:9m4e2mr0ui3e8a215n4g --app mediawiki
    secret-revoke secret:9m4e2mr0ui3e8a215n4g --unit mediawiki/6


## Details

Revoke access to view the value of a specified secret.
Access may be revoked from an application (all units of
that application lose access), or from a specified unit.
If run in a relation hook, the related application's 
access is revoked, unless a uni is specified, in which
case just that unit's access is revoked.'
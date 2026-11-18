(command-juju-recover)=
# `juju recover`
> See also: [bootstrap](#command-juju-bootstrap), [create-backup](#command-juju-create-backup), [unregister](#command-juju-unregister)

## Summary
Recover a controller from a backup archive.

## Usage
```text
juju recover [options] <backup-file> <assertion-file>
```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |

## Details
Controller recovery uses a backup archive and its unsigned SHA-256 assertion
to restore a controller on a replacement machine.

This version validates the archive and allocates the replacement machine only.
It leaves the instance for inspection and exits with an error reporting its ID.
No agent is installed and no controller is registered.
(command-juju-recover)=
# `juju recover`
> See also: [bootstrap](#command-juju-bootstrap), [create-backup](#command-juju-create-backup), [unregister](#command-juju-unregister)

## Summary
Validate a controller recovery archive.

## Usage
```text
juju recover [options] <backup-file> <assertion-file>
```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |

## Details
Validate an archive against its unsigned SHA-256 assertion. Controller recovery provisioning is not available in this build.
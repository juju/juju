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
to restore a controller on a replacement machine or Kubernetes namespace.
The client must be able to resolve the cloud named in the archive. On a new
workstation, add the cloud with 'juju add-cloud --client' if it is not
already available.

This version validates the archive and installs and starts a replacement
controller agent only. It leaves the machine or namespace for inspection and
exits with an error reporting its ID or namespace.
Database import and controller initialisation are not available, and the
controller is not registered.

The recovering client's Juju SSH public keys are installed on the replacement
machine so it can be inspected over SSH while recovery is pending. They grant
access to the machine only, not to the controller API.
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
workstation, add the cloud with 'juju add-cloud --client', or a Kubernetes
cloud with 'juju add-k8s --client', if it is not already available.

On a machine cloud, recovery uploads and verifies the archive on the replacement
machine, then initialises its Dqlite database with the controller-node row.
On Kubernetes, recovery validates the archive locally and starts a replacement
controller agent in the new namespace without initialising the database.

Neither path imports archived state or registers the controller. The command
leaves the replacement in place for inspection and exits with an error that
identifies the instance or namespace.

The recovering client's Juju SSH public keys are installed on the replacement
machine so it can be inspected over SSH while recovery is pending. They grant
access to the machine only, not to the controller API.
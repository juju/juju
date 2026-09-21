(command-juju-create-backup)=
# `juju create-backup`
## Summary
Create a backup.

## Usage
```text
juju create-backup [options] [<notes>]
```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `--filename` | juju-backup-&lt;date&gt;-&lt;time&gt;.tar.gz | Download to this file |
| `-m`, `--model` |  | Model to operate in. Accepts [&lt;controller name&gt;:]&lt;model name&gt;&#x7c;&lt;model UUID&gt; |

## Examples

    juju create-backup


## Details

This command requests that Juju creates a backup of its state.
You may provide a note to associate with the backup.

The backup archive is always downloaded to the local machine, and the
copy written on the controller is removed once it has been delivered.
The archive is verified against the recorded checksum before the
download is considered complete.

If the transfer is interrupted, the staged archive is kept on the
controller for the duration of the `backup-download-ttl` model config
attribute (15 minutes by default) so the download can be retried;
once that window lapses the staged copy is removed and the backup
must be created again.

The model config attribute `backup-dir` only serves as scratch space
during backup creation; no archive is kept there once the command
finishes. On an HA controller the archive is staged on the controller
machine that served the request and downloaded over the same API
connection, so a retry that reaches a different controller machine
will not find it; point `backup-dir` at a filesystem shared by all
controller machines if download retries must survive reconnection.

Use `--verbose` to see extra information about backup.
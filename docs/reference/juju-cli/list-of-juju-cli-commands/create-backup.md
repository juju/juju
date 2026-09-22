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

The backup archive is always downloaded to the local machine: the
controller creates the archive and streams it back in the same
request, and nothing is kept on the controller once the request ends.
The archive is verified against the recorded checksum before the
download is considered complete; if verification fails, the corrupt
archive is kept locally under a `.corrupt` suffix for inspection and
the backup must be created again. An interrupted transfer leaves no
archive on either side: re-run the command to create the backup
again.

The model config attribute `backup-dir` only serves as scratch space
during backup creation; no archive is kept there once the command
finishes.

Use `--verbose` to see extra information about backup.
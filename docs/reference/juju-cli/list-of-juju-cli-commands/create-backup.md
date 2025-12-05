(command-juju-create-backup)=
# `juju create-backup`
> See also: [download-backup](#download-backup)

## Summary
Creates a backup.

## Usage
```juju create-backup [options] [<notes>]```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Disables web browser for authentication. |
| `--filename` | juju-backup-&lt;date&gt;-&lt;time&gt;.tar.gz | Specifies the file to download to. |
| `-m`, `--model` |  | Model to operate in. Accepts [&lt;controller name&gt;:]&lt;model name&gt;&#x7c;&lt;model UUID&gt; |
| `--no-download` | false | Skips downloading the archive. DEPRECATED. |

## Examples

    juju create-backup
    juju create-backup --no-download


## Details

Creates a backup of Juju's state.

A note may be provided to associate with the backup.

By default, the backup archive and associated metadata are downloaded.

The `--no-download` option can be used to avoid getting a local copy of the backup downloaded
at the end of the backup process. In this case it is recommended that the
model config attribute `backup-dir` be set to point to a path where the
backup archives should be stored long term. This could be a remotely mounted
filesystem; the same path must exist on each controller if using HA.

The `--verbose` option can be used to see extra information about backup.

To access remote backups stored on the controller, `juju download-backup` can be used.
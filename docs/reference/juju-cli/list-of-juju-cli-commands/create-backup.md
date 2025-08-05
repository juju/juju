(command-juju-create-backup)=
# `juju create-backup`
> See also: [download-backup](#download-backup)

## Summary
Create a backup.

## Usage
```juju create-backup [options] [<notes>]```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `--filename` | juju-backup-&lt;date&gt;-&lt;time&gt;.tar.gz | Download to this file |
| `-m`, `--model` |  | Model to operate in. Accepts [&lt;controller name&gt;:]&lt;model name&gt;&#x7c;&lt;model UUID&gt; |
| `--no-download` | false | Do not download the archive. DEPRECATED. |

## Examples

    juju create-backup 
    juju create-backup --no-download


## Details

This command requests that Juju creates a backup of its state.
You may provide a note to associate with the backup.

By default, the backup archive and associated metadata are downloaded.

Use --no-download to avoid getting a local copy of the backup downloaded 
at the end of the backup process. In this case it is recommended that the
model config attribute "backup-dir" be set to point to a path where the
backup archives should be stored long term. This could be a remotely mounted
filesystem; the same path must exist on each controller if using HA.

Use --verbose to see extra information about backup.

To access remote backups stored on the controller, see 'juju download-backup'.
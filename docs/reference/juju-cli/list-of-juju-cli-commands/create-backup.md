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

The backup archive is always downloaded to the local machine, to the
file given by `--filename` (or a generated
`juju-backup-<date>-<time>.tar.gz` name), and is verified against the
recorded checksum before the download is considered complete.

The archive is kept on the controller only until it has been downloaded,
or for a short retention window, after which it is removed
automatically: a backup can be downloaded exactly once, at creation
time. The model config attribute `backup-dir` only serves as scratch
space during backup creation; no archive is kept there once the command
finishes.

Use `--verbose` to see extra information about backup.

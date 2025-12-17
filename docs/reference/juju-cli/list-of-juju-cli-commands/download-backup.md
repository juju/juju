(command-juju-download-backup)=
# `juju download-backup`
> See also: [create-backup](#create-backup)

## Summary
Downloads a backup archive file.

## Usage
```juju download-backup [options] /full/path/to/backup/on/controller```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Specifies whether to skip web browser for authentication. |
| `--filename` |  | Specifies the download target. |
| `-m`, `--model` |  | Specifies the model to operate in. Accepts `[<controller name>:]<model name>|<model UUID>`. |

## Examples

    juju download-backup /full/path/to/backup/on/controller


## Details

Retrieves a backup archive file.

If `--filename` is not used, the archive is downloaded to a temporary
location and the filename is printed to stdout.
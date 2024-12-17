(command-juju-download-backup)=
# `juju download-backup`
> See also: [create-backup](#create-backup)

## Summary
Download a backup archive file.

## Usage
```juju download-backup [options] /full/path/to/backup/on/controller```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `--filename` |  | Download target |
| `-m`, `--model` |  | Model to operate in. Accepts [&lt;controller name&gt;:]&lt;model name&gt;&#x7c;&lt;model UUID&gt; |

## Examples

    juju download-backup /full/path/to/backup/on/controller


## Details

download-backup retrieves a backup archive file.

If --filename is not used, the archive is downloaded to a temporary
location and the filename is printed to stdout.
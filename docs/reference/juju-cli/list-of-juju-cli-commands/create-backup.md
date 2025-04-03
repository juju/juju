(command-juju-create-backup)=
# `juju create-backup`

```
Usage: juju create-backup [options] [<notes>]

Summary:
Create a backup.

Global Options:
--debug  (= false)
    equivalent to --show-log --logging-config=<root>=DEBUG
-h, --help  (= false)
    Show help on a command or other topic.
--logging-config (= "")
    specify log levels for modules
--quiet  (= false)
    show no informational output
--show-log  (= false)
    if set, write the log file to stderr
--verbose  (= false)
    show more verbose output

Command Options:
-B, --no-browser-login  (= false)
    Do not use web browser for authentication
--filename (= "juju-backup-<date>-<time>.tar.gz")
    Download to this file
-m, --model (= "")
    Model to operate in. Accepts [<controller name>:]<model name>|<model UUID>
--no-download  (= false)
    Do not download the archive. DEPRECATED.

Details:
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

Examples:
    juju create-backup
    juju create-backup --no-download

See also:
    download-backup
```
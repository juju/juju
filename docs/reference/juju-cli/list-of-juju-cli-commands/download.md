(command-juju-download)=
# `juju download`

```
Usage: juju download [options] [options] <charm>

Summary:
Locates and then downloads a CharmHub charm.

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
--arch (= "all")
    specify an arch <all|amd64|arm64|armhf|i386|ppc64el|s390x>
--channel (= "")
    specify a channel to use instead of the default release
--charmhub-url (= "https://api.charmhub.io")
    specify the Charmhub URL for querying the store
--filepath (= "")
    filepath location of the charm to download to
--no-progress  (= false)
    disable the progress bar
--series (= "all")
    specify a series

Details:
Download a charm to the current directory from the CharmHub store
by a specified name.

Adding a hyphen as the second argument allows the download to be piped
to stdout.

Examples:
    juju download postgresql
    juju download postgresql --no-progress - > postgresql.charm

See also:
    info
    find
```
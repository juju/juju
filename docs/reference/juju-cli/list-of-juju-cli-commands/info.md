(command-juju-info)=
# `juju info`

```
Usage: juju info [options] [options] <charm>

Summary:
Displays detailed information about CharmHub charms.

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
--config  (= false)
    display config for this charm
--format  (= tabular)
    Specify output format (json|tabular|yaml)
-o, --output (= "")
    Specify an output file
--series (= "all")
    specify a series
--unicode (= "auto")
    display output using unicode <auto|never|always>

Details:
The charm can be specified by name or by path.

Channels displayed are supported by any series.
To see channels supported for only a specific series, use the --series flag.

Examples:
    juju info postgresql

See also:
    find
    download
```
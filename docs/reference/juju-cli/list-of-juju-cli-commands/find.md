(command-juju-find)=
# `juju find`

```
Usage: juju find [options] [options] <query>

Summary:
Queries the CharmHub store for available charms or bundles.

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
--category (= "")
    filter by a category name
--channel (= "")
    filter by channel"
--charmhub-url (= "https://api.charmhub.io")
    specify the Charmhub URL for querying the store
--columns (= "nbvps")
    display the columns associated with a find search.

    The following columns are supported:
        - n: Name
        - b: Bundle
        - v: Version
        - p: Publisher
        - s: Summary
		- a: Architecture
		- o: OS
        - S: Supports

--format  (= tabular)
    Specify output format (json|tabular|yaml)
-o, --output (= "")
    Specify an output file
--publisher (= "")
    search by a given publisher
--type (= "")
    search by a given type <charm|bundle>

Details:
The find command queries the CharmHub store for available charms or bundles.

Examples:
    juju find wordpress

See also:
    info
    download
```
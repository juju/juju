(command-juju-upgrade-dashboard)=
# `juju upgrade-dashboard`

```
Usage: juju upgrade-dashboard [options]

Summary:
Upgrade to a new Juju Dashboard version.

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
-c, --controller (= "")
    Controller to operate in
--gui-stream (= "released")
    Specify the stream used to fetch the dashboard
--list  (= false)
    List available Juju Dashboard release versions without upgrading

Details:
Upgrade to the latest Juju Dashboard released version:

	juju upgrade-dashboard

Upgrade to the latest Juju Dashboard development version:

	juju upgrade-dashboard --gui-stream=devel

Upgrade to a specific Juju Dashboard released version:

	juju upgrade-dashboard 2.2.0

Upgrade to a Juju Dashboard version present in a local tar.bz2 Dashboard release file:

	juju upgrade-dashboard /path/to/jujugui-2.2.0.tar.bz2

List available Juju Dashboard releases without upgrading:

	juju upgrade-dashboard --list

Aliases: upgrade-gui
```
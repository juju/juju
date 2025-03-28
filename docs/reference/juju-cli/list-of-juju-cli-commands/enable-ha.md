(command-juju-enable-ha)=
# `juju enable-ha`

```
Usage: juju enable-ha [options]

Summary:
Ensure that sufficient controllers exist to provide redundancy.

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
--constraints (= "")
    Additional machine constraints
--format  (= simple)
    Specify output format (json|simple|yaml)
-n  (= 0)
    Number of controllers to make available
-o, --output (= "")
    Specify an output file
--to (= "")
    The machine(s) to become controllers, bypasses constraints

Details:
To ensure availability of deployed applications, the Juju infrastructure
must itself be highly available. The enable-ha command will ensure
that the specified number of controller machines are used to make up the
controller.

An odd number of controllers is required.

Examples:
    # Ensure that the controller is still in highly available mode. If
    # there is only 1 controller running, this will ensure there
    # are 3 running. If you have previously requested more than 3,
    # then that number will be ensured.
    juju enable-ha

    # Ensure that 5 controllers are available.
    juju enable-ha -n 5

    # Ensure that 7 controllers are available, with newly created
    # controller machines having at least 8GB RAM.
    juju enable-ha -n 7 --constraints mem=8G

    # Ensure that 7 controllers are available, with machines server1 and
    # server2 used first, and if necessary, newly created controller
    # machines having at least 8GB RAM.
    juju enable-ha -n 7 --to server1,server2 --constraints mem=8G
```
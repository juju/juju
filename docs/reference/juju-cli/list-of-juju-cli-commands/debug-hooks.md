(command-juju-debug-hooks)=
# `juju debug-hooks`

```
Usage: juju debug-hooks [options] <unit name> [hook or action names]

Summary:
Launch a tmux session to debug hooks and/or actions.

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
--container (= "")
    the container name of the target pod
-m, --model (= "")
    Model to operate in. Accepts [<controller name>:]<model name>|<model UUID>
--no-host-key-checks  (= false)
    Skip host key checking (INSECURE)
--proxy  (= false)
    Proxy through the API server
--pty  (= <auto>)
    Enable pseudo-tty allocation
--remote  (= false)
    Target on the workload or operator pod (k8s-only)

Details:
Interactively debug hooks or actions remotely on an application unit.

Valid unit identifiers are:
  a standard unit ID, such as mysql/0 or;
  leader syntax of the form <application>/leader, such as mysql/leader.

See the "juju help ssh" for information about SSH related options
accepted by the debug-hooks command.

Aliases: debug-hook
```
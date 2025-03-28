(command-juju-debug-log)=
# `juju debug-log`

```
Usage: juju debug-log [options]

Summary:
Displays log messages for a model.

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
--color  (= false)
    Force use of ANSI color codes
--date  (= false)
    Show dates as well as times
--exclude-label  (= )
    Do not show log messages for these logging labels
--exclude-module  (= )
    Do not show log messages for these logging modules
-i, --include  (= )
    Only show log messages for these entities
--include-label  (= )
    Only show log messages for these logging labels
--include-module  (= )
    Only show log messages for these logging modules
-l, --level (= "")
    Log level to show, one of [TRACE, DEBUG, INFO, WARNING, ERROR]
--limit  (= 0)
    Exit once this many of the most recent (possibly filtered) lines are shown
--location  (= false)
    Show filename and line numbers
-m, --model (= "")
    Model to operate in. Accepts [<controller name>:]<model name>|<model UUID>
--ms  (= false)
    Show times to millisecond precision
-n, --lines  (= 10)
    Show this many of the most recent (possibly filtered) lines, and continue to append
--no-tail  (= false)
    Stop after returning existing log messages
--replay  (= false)
    Show the entire (possibly filtered) log and continue to append
--tail  (= false)
    Wait for new logs
--utc  (= false)
    Show times in UTC
-x, --exclude  (= )
    Do not show log messages for these entities

Details:
This command provides access to all logged Juju activity on a per-model
basis. By default, the logs for the currently select model are shown.

Each log line is emitted in this format:

  <entity> <timestamp> <log-level> <module>:<line-no> <message>

The "entity" is the source of the message: a machine or unit. The names for
machines and units can be seen in the output of `juju status`.

The '--include' and '--exclude' options filter by entity. The entity can be
a machine, unit, or application for vm models, but can be application only
for k8s models. These filters support wildcards `*` if filtering on the
entity full name (prefixed by `<entity type>-`)

The '--include-module' and '--exclude-module' options filter by (dotted)
logging module name. The module name can be truncated such that all loggers
with the prefix will match.

The '--include-label' and '--exclude-label' options filter by logging label.

The filtering options combine as follows:
* All --include options are logically ORed together.
* All --exclude options are logically ORed together.
* All --include-module options are logically ORed together.
* All --exclude-module options are logically ORed together.
* All --include-label options are logically ORed together.
* All --exclude-label options are logically ORed together.
* The combined --include, --exclude, --include-module, --exclude-module,
  --include-label and --exclude-label selections are logically ANDed to form
  the complete filter.

Examples:

Exclude all machine 0 messages; show a maximum of 100 lines; and continue to
append filtered messages:

    juju debug-log --exclude machine-0 --lines 100

Include only messages from the mysql/0 unit; show a maximum of 50 lines; and then
exit:

    juju debug-log --include mysql/0 --limit 50

Include only messages from the gitlab-k8s application:

    juju debug-log --include gitlab-k8s

Show all messages from the apache/2 unit or machine 1 and then exit:

    juju debug-log --replay --include apache/2 --include machine-1 --no-tail

Show all juju.worker.uniter logging module messages that are also unit
wordpress/0 messages, and then show any new log messages which match the
filter and append:

    juju debug-log --replay
        --include-module juju.worker.uniter \
        --include wordpress/0

Show all messages from the juju.worker.uniter module, except those sent from
machine-3 or machine-4, and then stop:

    juju debug-log --replay --no-tail
        --include-module juju.worker.uniter \
        --exclude machine-3 \
        --exclude machine-4

To see all WARNING and ERROR messages and then continue showing any
new WARNING and ERROR messages as they are logged:

    juju debug-log --replay --level WARNING

See also:
    status
    ssh
```
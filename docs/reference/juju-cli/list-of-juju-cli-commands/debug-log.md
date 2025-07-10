(command-juju-debug-log)=
# `juju debug-log`
> See also: [status](#status), [ssh](#ssh)

## Summary
Displays log messages for a model.

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `--color` | false | Force use of ANSI color codes |
| `--date` | false | Show dates as well as times |
| `--exclude-labels` |  | Do not show log messages for these logging label key values |
| `--exclude-module` |  | Do not show log messages for these logging modules |
| `--firehose` | false | Show logs from all models |
| `--format` | text | Specify output format (json&#x7c;text) |
| `-i`, `--include` |  | Only show log messages for these entities |
| `--include-labels` |  | Only show log messages for these logging label key values |
| `--include-module` |  | Only show log messages for these logging modules |
| `-l`, `--level` |  | Log level to show, one of [TRACE, DEBUG, INFO, WARNING, ERROR] |
| `--limit` | 0 | Show this many of the most recent logs and then exit |
| `--location` | false | Show filename and line numbers |
| `-m`, `--model` |  | Model to operate in. Accepts [&lt;controller name&gt;:]&lt;model name&gt;&#x7c;&lt;model UUID&gt; |
| `--ms` | false | Show times to millisecond precision |
| `-n`, `--lines` | 0 | Show this many of the most recent lines and continue to append new ones |
| `--no-tail` | false | Show existing log messages and then exit |
| `-o`, `--output` |  | Specify an output file |
| `--replay` | false | Show the entire log and continue to append new ones |
| `--retry` | false | Retry connection on failure |
| `--retry-delay` | 1s | Retry delay between connection failure retries |
| `--tail` | false | Show existing log messages and continue to append new ones |
| `--utc` | false | Show times in UTC |
| `-x`, `--exclude` |  | Do not show log messages for these entities |

## Examples


Begin with all the log messages:

    juju debug-log --replay

Begin with the last 500 lines, using grep as a text filter:

    juju debug-log -n 500 | grep amd64

Begin with the last 30 log messages:

    juju debug-log -n 30

Begin with the last 20 log messages for the 'lxd-pilot' model:

    juju debug-log -m lxd-pilot -n 20

Begin with the last 1000 lines and exclude messages from machine 3:

    juju debug-log -n 1000 --exclude machine-3

Select all the messages emitted from a particular unit (you can also write it as
 mysql/0) and a particular machine in the entire log:

juju debug-log --replay --include unit-mysql-0 --include machine-1

View all WARNING and ERROR messages in the entire log:

    juju debug-log --replay --level WARNING

View all WARNING and ERROR messages and then continue showing any
new WARNING and ERROR messages as they are logged:

    juju debug-log --replay --level WARNING

View all logs on the cmr topic (label):

    juju debug-log --include-labels cmr

Progressively exclude more content from the entire log:

    juju debug-log --replay --exclude-module juju.state.apiserver
    juju debug-log --replay --exclude-module juju.state
    juju debug-log --replay --exclude-module juju

Begin with the last 2000 lines and include messages pertaining to both the
juju.cmd and the juju.worker modules:

    juju debug-log --lines 2000 \
        --include-module juju.cmd \
        --include-module juju.worker

Exclude all messages from machine 0 ; show a maximum of 100 lines; and continue to
append filtered messages:

    juju debug-log --exclude machine-0 --lines 100

Include only messages from the mysql/0 unit; show a maximum of 50 lines; and then
exit:

    juju debug-log --include mysql/0 --limit 50

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


## Details

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

The '--include-labels' and '--exclude-labels' options filter by logging labels.

The filtering options combine as follows:
* All --include options are logically ORed together.
* All --exclude options are logically ORed together.
* All --include-module options are logically ORed together.
* All --exclude-module options are logically ORed together.
* All --include-labels options are logically ORed together.
* All --exclude-labels options are logically ORed together.
* The combined --include, --exclude, --include-module, --exclude-module,
  --include-labels and --exclude-labels selections are logically ANDed to form
  the complete filter.

The '--tail' option waits for and continuously prints new log lines after displaying the most recent log lines.

The '--no-tail' option displays the most recent log lines and then exits immediately.

The '--lines' and '--limit' options control the number of log lines displayed:
* --lines option prints the specified number of the most recent lines and then waits for new lines. This implies --tail.
* --limit option prints up to the specified number of the most recent lines and exits. This implies --no-tail.
* setting --lines or --limit to 0 will print the maximum number of the most recent lines available.

The '--replay' option displays log lines starting from the beginning.

Behavior when combining --replay with other options:
* --replay and --limit prints the specified number of lines from the beginning of the log.
* --replay and --lines is invalid as it causes confusion by skipping logs between the replayed lines and the current tailing point.

Given the above, the following flag combinations are incompatible and cannot be specified together:
* --tail and --no-tail
* --tail and --limit
* --no-tail and --lines (-n)
* --limit and --lines (-n)
* --replay and --lines (-n)
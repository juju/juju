(command-juju-exec)=
# `juju exec`
> See also: [run](#run), [ssh](#ssh)

## Summary
Runs the commands on the remote targets specified.

## Usage
```juju exec [options] <commands>```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Specifies whether to skip web browser for authentication. |
| `-a`, `--app`, `--application` |  | Specifies one or more application names. |
| `--all` | false | Specifies whether to run the commands on all the machines. |
| `--background` | false | Specifies whether to run the task in the background. |
| `--color` | false | Specifies whether to use ANSI color codes in output. |
| `--execution-group` |  | Runs commands in the same execution group sequentially. |
| `--format` | plain | Specify output format (json&#x7c;plain&#x7c;yaml) |
| `-m`, `--model` |  | Specifies the model to operate in. Accepts `[<controller name>:]<model name>|<model UUID>`. |
| `--machine` |  | Specifies one or more machine IDs. |
| `--no-color` | false | Specifies whether to disable ANSI color codes in output. |
| `-o`, `--output` |  | Specify an output file |
| `--operator` | false | (KUBERNETES ONLY) Specifies whether to run the commands on the operator. |
| `--parallel` | true | Specifies whether to run the commands in parallel without first acquiring a lock. |
| `-u`, `--unit` |  | Specifies one or more unit IDs. |
| `--utc` | false | Specifies whether to show times in UTC. |
| `--wait` | 0s | Specifies the maximum wait time for a task to complete. |

## Examples


    juju exec --all -- hostname -f

    juju exec --unit hello/0 env

    juju exec --unit controller/0 juju-engine-report


## Details

Runs a shell command on the specified targets. Only admin users of a model
are able to use this command.

Targets are specified using either machine IDs, application names or unit
names.  At least one target specifier is needed.

Multiple values can be set for `--machine`, `--application`, and `--unit` by using
comma separated values.

Depending on the type of target, the user which the command runs as will be:

  unit -&gt; `root`
  machine -&gt; `ubuntu`

The target and user are independent of whether `--all` or `--application` are used.
For example, `--all` will run as `ubuntu` on machines and `root` on units.
And `--application` will run as `root` on all units of that application.

Some options are shortened for usability purposes in the CLI:

- `--application` can also be specified as `--app` and `-a`
- `--unit` can also be specified as `-u`

Valid unit identifiers are:

  - a standard unit ID, such as `mysql/0` or;
  - leader syntax of the form `<application>/leader`, such as `mysql/leader`.

If the target is an application, the command is run on all units for that
application. For example, if there was an application `mysql` and that application
had two units, `mysql/0` and `mysql/1`, then `--application mysql`
is equivalent to `--unit mysql/0,mysql/1`.

If `--operator` is provided on Kubernetes models, commands are executed on the operator
instead of the workload. On machine models, `--operator` has no effect.

Commands run for applications or units are executed in a hook context for
the unit.

Commands run on machines via the -`-machine` argument are run in parallel
by default.
If you want commands to be run sequentially in order of submission,
use `--parallel=false`.
Such commands will first acquire a global execution lock on the host machine
before running, and release the lock when done.
It's also possible to group commands so that those in the same group run
sequentially, but in parallel with other groups. This is done using
`--execution-group=somegroup`.

`--all` is provided as a simple way to run the command on all the machines
in the model.  If you specify `--all` you cannot provide additional
targets.

Since `juju exec` creates tasks, you can query for the status of commands
started with `juju run` by calling `juju operations --machines <id>,... --actions juju-exec`.

If you need to pass options to the command being run, you must precede the
command and its arguments with `--`, to tell `juju exec` to stop processing
those arguments. For example:

    juju exec --all -- hostname -f
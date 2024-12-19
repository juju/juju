(command-juju-run)=
# `juju run`
> See also: [operations](#operations), [show-operation](#show-operation), [show-task](#show-task)

## Summary
Run an action on a specified unit.

## Usage
```juju run [options] <unit> [<unit> ...] <action-name> [<key>=<value> [<key>[.<key> ...]=<value>]]```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `--background` | false | Run the task in the background |
| `--color` | false | Use ANSI color codes in output |
| `--format` | plain | Specify output format (json&#x7c;plain&#x7c;yaml) |
| `-m`, `--model` |  | Model to operate in. Accepts [&lt;controller name&gt;:]&lt;model name&gt;&#x7c;&lt;model UUID&gt; |
| `--no-color` | false | Disable ANSI color codes in output |
| `-o`, `--output` |  | Specify an output file |
| `--params` |  | Path to yaml-formatted params file |
| `--string-args` | false | Use raw string values of CLI args |
| `--utc` | false | Show times in UTC |
| `--wait` | 0s | Maximum wait time for a task to complete |

## Examples

    juju run mysql/3 backup --background
    juju run mysql/3 backup --wait=2m
    juju run mysql/3 backup --format yaml
    juju run mysql/3 backup --utc
    juju run mysql/3 backup
    juju run mysql/leader backup
    juju show-operation <ID>
    juju run mysql/3 backup --params parameters.yml
    juju run mysql/3 backup out=out.tar.bz2 file.kind=xz file.quality=high
    juju run mysql/3 backup --params p.yml file.kind=xz file.quality=high
    juju run sleeper/0 pause time=1000
    juju run sleeper/0 pause --string-args time=1000


## Details

Run a charm action for execution on the given unit(s), with a given set of params.
An ID is returned for use with 'juju show-operation &lt;ID&gt;'.

All units must be of the same application.

A action executed on a given unit becomes a task with an ID that can be
used with 'juju show-task &lt;ID&gt;'.

Running an action returns the overall operation ID as well as the individual
task ID(s) for each unit.

To queue a action to be run in the background without waiting for it to finish,
use the --background option.

To set the maximum time to wait for a action to complete, use the --wait option.

By default, a single action will output its failure message if the action fails,
followed by any results set by the action. For multiple actions, each action's
results will be printed with the action id and action status. To see more detailed
information about run timings etc, use --format yaml.

Valid unit identifiers are: 
  a standard unit ID, such as mysql/0 or;
  leader syntax of the form &lt;application&gt;/leader, such as mysql/leader.

If the leader syntax is used, the leader unit for the application will be
resolved before the action is enqueued.

Params are validated according to the charm for the unit's application.  The
valid params can be seen using "juju actions &lt;application&gt; --schema".
Params may be in a yaml file which is passed with the --params option, or they
may be specified by a key.key.key...=value format (see examples below.)

Params given in the CLI invocation will be parsed as YAML unless the
--string-args option is set.  This can be helpful for values such as 'y', which
is a boolean true in YAML.

If --params is passed, along with key.key...=value explicit arguments, the
explicit arguments will override the parameter file.
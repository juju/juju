(hook-command)=
# Hook command

> See also: {ref}`Hook <hook>`, {ref}`Ops <ops-ops>`


In Juju, a **hook tool  (or 'hook command')** is a Bash script located in `/var/lib/juju/tools/unit-<app name>-<unit ID>` that a charm uses to communicate with its Juju unit agent in response to a {ref}`hook <hook>`. 

In the Juju ecosystem, in [Ops])(https://ops.readthedocs.io/en/latest/), hook tools are accessed through Ops constructs, specifically, those constructs designed to be used in the definition of the event handlers associated with the Ops events that translate Juju {ref}`hooks <hook>`. For example, when your charm calls `ops.Unit.is_leader`, in the background this calls `~/hooks/unit-name/leader-get`; its output is wrapped and returned as a Python `True/False` value.

<!--
 They provide the most raw interface to a Juju model from the perspective of a charm, that is, the low-level `juju` system calls that provide the backend for  {ref}`Ops <ops-ops>`.

```{note}

We currently recommend that people do not interact with hook tools directly but rather use  {ref}`Ops <ops-ops>`. Ops provides a Pythonic layer to interact with hook tools. For example, when you call `ops.Unit.is_leader`, in the background `~/hooks/unit-name/leader-get` gets called. Its output is wrapped and returned as a Python `True/False` value.

```
-->

In Juju, you can use hook commands for troubleshooting.


````{dropdown} Example: Use relation-get to change relation data

```text
# Get the relation ID

$ juju show-unit synapse/0

...
  - relation-id: 7
    endpoint: synapse-peers
    related-endpoint: synapse-peers
   application-data:
      secret-id: secret://1234
    local-unit:
      in-scope: true


# Check the output:
$ juju exec --unit synapse/0 "relation-get -r 7 --app secret-id synapse/0"
secret://1234

# Change the data:
juju exec --unit synapse/0 "relation-set -r 7 --app secret-id=something-else"

# Check the output again to verify the change.
```

````
# Index
0. [action-fail](#action-fail)
1. [action-get](#action-get)
2. [action-log](#action-log)
3. [action-set](#action-set)
4. [application-version-set](#application-version-set)
5. [close-port](#close-port)
6. [config-get](#config-get)
7. [credential-get](#credential-get)
9. [goal-state](#goal-state)
11. [is-leader](#is-leader)
12. [juju-log](#juju-log)
13. [juju-reboot](#juju-reboot)
14. [leader-get](#leader-get)
15. [leader-set](#leader-set)
16. [network-get](#network-get)
17. [open-port](#open-port)
18. [opened-ports](#opened-ports)
19. [relation-get](#relation-get)
20. [relation-ids](#relation-ids)
21. [relation-list](#relation-list)
22. [relation-model-get](#relation-model-get)
23. [relation-set](#relation-set)
24. [resource-get](#resource-get)
25. [secret-add](#secret-add)
26. [secret-get](#secret-get)
27. [secret-grant](#secret-grant)
28. [secret-ids](#secret-ids)
29. [secret-info-get](#secret-info-get)
30. [secret-remove](#secret-remove)
31. [secret-revoke](#secret-revoke)
32. [secret-set](#secret-set)
33. [state-delete](#state-delete)
34. [state-get](#state-get)
35. [state-set](#state-set)
36. [status-get](#status-get)
37. [status-set](#status-set)
38. [storage-add](#storage-add)
39. [storage-get](#storage-get)
40. [storage-list](#storage-list)
41. [unit-get](#unit-get)
---

# action-fail

## Summary
Set action fail status with message.

## Usage
``` action-fail [options] ["<failure message>"]```

## Examples

    action-fail 'unable to contact remote service'


## Details

action-fail sets the fail state of the action with a given error message.  Using
action-fail without a failure message will set a default message indicating a
problem with the action.


---

# action-get

## Summary
Get action parameters.

## Usage
``` action-get [options] [<key>[.<key>.<key>...]]```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `--format` | smart | Specify output format (json&#x7c;smart&#x7c;yaml) |
| `-o`, `--output` |  | Specify an output file |

## Examples

    TIMEOUT=$(action-get timeout)


## Details

action-get will print the value of the parameter at the given key, serialized
as YAML.  If multiple keys are passed, action-get will recurse into the param
map as needed.


---

# action-log

## Summary
Record a progress message for the current action.

## Usage
``` action-log [options] <message>```

---

# action-set

## Summary
Set action results.

## Usage
``` action-set [options] <key>=<value> [<key>=<value> ...]```

## Examples

    action-set outfile.size=10G
    action-set foo.bar=2
    action-set foo.baz.val=3
    action-set foo.bar.zab=4
    action-set foo.baz=1

will yield:

    outfile:
      size: "10G"
    foo:
      bar:
        zab: "4"
      baz: "1"


## Details

action-set adds the given values to the results map of the Action. This map
is returned to the user after the completion of the Action. Keys must start
and end with lowercase alphanumeric, and contain only lowercase alphanumeric,
hyphens and periods.  The following special keys are reserved for internal use: 
"stdout", "stdout-encoding", "stderr", "stderr-encoding".


---

# application-version-set

## Summary
Specify which version of the application is deployed.

## Usage
``` application-version-set [options] <new-version>```

## Examples

    application-version-set 1.1.10


## Details

application-version-set tells Juju which version of the application
software is running. This could be a package version number or some
other useful identifier, such as a Git hash, that indicates the
version of the deployed software. (It shouldn't be confused with the
charm revision.) The version set will be displayed in "juju status"
output for the application.


---

# close-port

## Summary
Register a request to close a port or port range.

## Usage
``` close-port [options] <port>[/<protocol>] or <from>-<to>[/<protocol>] or icmp```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `--endpoints` |  | a comma-delimited list of application endpoints to target with this operation |
| `--format` |  | deprecated format flag |

## Examples

    # Close single port
    close-port 80

    # Close a range of ports
    close-port 9000-9999/udp

    # Disable ICMP
    close-port icmp

    # Close a range of ports for a set of endpoints (since Juju 2.9)
    close-port 80-90 --endpoints dmz,public


## Details

close-port registers a request to close the specified port or port range.

By default, the specified port or port range will be closed for all defined
application endpoints. The --endpoints option can be used to constrain the
close request to a comma-delimited list of application endpoints.


---

# config-get

## Summary
Print application configuration.

## Usage
``` config-get [options] [<key>]```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-a`, `--all` | false | print all keys |
| `--format` | smart | Specify output format (json&#x7c;smart&#x7c;yaml) |
| `-o`, `--output` |  | Specify an output file |

## Examples

    INTERVAL=$(config-get interval)

    config-get --all


## Details

config-get returns information about the application configuration
(as defined by config.yaml). If called without arguments, it returns
a dictionary containing all config settings that are either explicitly
set, or which have a non-nil default value. If the --all flag is passed,
it returns a dictionary containing all defined config settings including
nil values (for those without defaults). If called with a single argument,
it returns the value of that config key. Missing config keys are reported
as nulls, and do not return an error.

&lt;key&gt; and --all are mutually exclusive.


---

# credential-get

## Summary
Access cloud credentials.

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `--format` | smart | Specify output format (json&#x7c;smart&#x7c;yaml) |
| `-o`, `--output` |  | Specify an output file |

## Details

credential-get returns the cloud specification used by the unit's model.


---

# documentation

## Summary
Generate the documentation for all commands

## Usage
``` documentation [options] --out <target-folder> --no-index --split --url <base-url> --discourse-ids <filepath>```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `--discourse-ids` |  | File containing a mapping of commands and their discourse ids |
| `--no-index` | false | Do not generate the commands index |
| `--out` |  | Documentation output folder if not set the result is displayed using the standard output |
| `--split` | false | Generate a separate Markdown file for each command |
| `--url` |  | Documentation host URL |

## Examples

    juju documentation
    juju documentation --split 
    juju documentation --split --no-index --out /tmp/docs

To render markdown documentation using a list of existing
commands, you can use a file with the following syntax

    command1: id1
    command2: id2
    commandN: idN

For example:

    add-cloud: 1183
    add-secret: 1284
    remove-cloud: 4344

Then, the urls will be populated using the ids indicated
in the file above.

    juju documentation --split --no-index --out /tmp/docs --discourse-ids /tmp/docs/myids


## Details

This command generates a markdown formatted document with all the commands, their descriptions, arguments, and examples.


---

# goal-state

## Summary
Print the status of the charm's peers and related units.

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `--format` | yaml | Specify output format (json&#x7c;yaml) |
| `-o`, `--output` |  | Specify an output file |

## Examples

    goal-state


## Details

'goal-state' command will list the charm units and relations, specifying their status and
their relations to other units in different charms.
goal-state queries information about charm deployment and returns it as structured data.

goal-state provides:
    - the details of other peer units have been deployed and their status
    - the details of remote units on the other end of each endpoint and their status

The output will be a subset of that produced by the juju status. There will be output
for sibling (peer) units and relation state per unit.

The unit status values are the workload status of the (sibling) peer units. We also use
a unit status value of dying when the unit’s life becomes dying. Thus unit status is one of:
    - allocating
    - active
    - waiting
    - blocked
    - error
    - dying

The relation status values are determined per unit and depend on whether the unit has entered
or left scope. The possible values are:
    - joining : a relation has been created, but no units are available. This occurs when the
      application on the other side of the relation is added to a model, but the machine hosting
      the first unit has not yet been provisioned. Calling relation-set will work correctly as
      that data will be passed through to the unit when it comes online, but relation-get will
      not provide any data.
    - joined : the relation is active. A unit has entered scope and is accessible to this one.
    - broken : unit has left, or is preparing to leave scope. Calling relation-get is not advised
      as the data will quickly out of date when the unit leaves.
    - suspended : parent cross model relation is suspended
    - error: an external error has been detected

By reporting error state, the charm has a chance to determine that goal state may not be reached
due to some external cause. As with status, we will report the time since the status changed to
allow the charm to empirically guess that a peer may have become stuck if it has not yet reached
active state.


---

# help

## Summary
Show help on a command or other topic.

## Usage
``` help [flags] [topic]```

## Details

See also: topics


---

# is-leader

## Summary
Print application leadership status.

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `--format` | smart | Specify output format (json&#x7c;smart&#x7c;yaml) |
| `-o`, `--output` |  | Specify an output file |

## Examples

    LEADER=$(is-leader)
    if [ "${LEADER}" == "True" ]; then
      # Do something a leader would do
    fi


## Details

is-leader prints a boolean indicating whether the local unit is guaranteed to
be application leader for at least 30 seconds. If it fails, you should assume that
there is no such guarantee.


---

# juju-log

## Summary
Write a message to the juju log.

## Usage
``` juju-log [options] <message>```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `--debug` | false | log at debug level |
| `--format` |  | deprecated format flag |
| `-l`, `--log-level` | INFO | Send log message at the given level |

## Examples

    juju-log -l 'WARN' Something has transpired


---

# juju-reboot

## Summary
Reboot the host machine.

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `--now` | false | reboot immediately, killing the invoking process |

## Examples

    # immediately reboot
    juju-reboot --now

    # Reboot after current hook exits
    juju-reboot


## Details

juju-reboot causes the host machine to reboot, after stopping all containers
hosted on the machine.

An invocation without arguments will allow the current hook to complete, and
will only cause a reboot if the hook completes successfully.

If the --now flag is passed, the current hook will terminate immediately, and
be restarted from scratch after reboot. This allows charm authors to write
hooks that need to reboot more than once in the course of installing software.

The --now flag cannot terminate a debug-hooks session; hooks using --now should
be sure to terminate on unexpected errors, so as to guarantee expected behaviour
in all situations.

juju-reboot is not supported when running actions.


---

# leader-get

## Summary
Print application leadership settings.

## Usage
``` leader-get [options] [<key>]```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `--format` | smart | Specify output format (json&#x7c;smart&#x7c;yaml) |
| `-o`, `--output` |  | Specify an output file |

## Examples

    ADDRESSS=$(leader-get cluster-leader-address)


## Details

leader-get prints the value of a leadership setting specified by key. If no key
is given, or if the key is "-", all keys and values will be printed.


---

# leader-set

## Summary
Write application leadership settings.

## Usage
``` leader-set [options] <key>=<value> [...]```

## Examples

    leader-set cluster-leader-address=10.0.0.123


## Details

leader-set immediate writes the supplied key/value pairs to the controller,
which will then inform non-leader units of the change. It will fail if called
without arguments, or if called by a unit that is not currently application leader.

leader-set lets you distribute string key=value pairs to other units, but with the
following differences:
    there’s only one leader-settings bucket per application (not one per unit)
    only the leader can write to the bucket
    only minions are informed of changes to the bucket
    changes are propagated instantly

The instant propagation may be surprising, but it exists to satisfy the use case where
shared data can be chosen by the leader at the very beginning of the install hook.

It is strongly recommended that leader settings are always written as a self-consistent
group leader-set one=one two=two three=three.


---

# network-get

## Summary
Get network config.

## Usage
``` network-get [options] <binding-name> [--ingress-address] [--bind-address] [--egress-subnets]```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `--bind-address` | false | get the address for the binding on which the unit should listen |
| `--egress-subnets` | false | get the egress subnets for the binding |
| `--format` | smart | Specify output format (json&#x7c;smart&#x7c;yaml) |
| `--ingress-address` | false | get the ingress address for the binding |
| `-o`, `--output` |  | Specify an output file |
| `--primary-address` | false | (deprecated) get the primary address for the binding |
| `-r`, `--relation` |  | specify a relation by id |

## Examples

    network-get dbserver
    network-get dbserver --bind-address

    See https://discourse.charmhub.io/t/charm-network-primitives/1126 for more
    in depth examples and explanation of usage.


## Details

network-get returns the network config for a given binding name. By default
it returns the list of interfaces and associated addresses in the space for
the binding, as well as the ingress address for the binding. If defined, any
egress subnets are also returned.
If one of the following flags are specified, just that value is returned.
If more than one flag is specified, a map of values is returned.

    --bind-address: the address the local unit should listen on to serve connections, as well
                    as the address that should be advertised to its peers.
    --ingress-address: the address the local unit should advertise as being used for incoming connections.
    --egress-subnets: subnets (in CIDR notation) from which traffic on this relation will originate.


---

# open-port

## Summary
Register a request to open a port or port range.

## Usage
``` open-port [options] <port>[/<protocol>] or <from>-<to>[/<protocol>] or icmp```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `--endpoints` |  | a comma-delimited list of application endpoints to target with this operation |
| `--format` |  | deprecated format flag |

## Examples

    # Open port 80 to TCP traffic:
    open-port 80/tcp

    # Open port 1234 to UDP traffic:
    open-port 1234/udp

    # Open a range of ports to UDP traffic:
    open-port 1000-2000/udp

    # Open a range of ports to TCP traffic for specific
    # application endpoints (since Juju 2.9):
    open-port 1000-2000/tcp --endpoints dmz,monitoring


## Details

open-port registers a request to open the specified port or port range.

By default, the specified port or port range will be opened for all defined
application endpoints. The --endpoints option can be used to constrain the
open request to a comma-delimited list of application endpoints.

The behavior differs a little bit between machine charms and Kubernetes charms.

Machine charms
On public clouds the port will only be open while the application is exposed.
It accepts a single port or range of ports with an optional protocol, which
may be icmp, udp, or tcp. tcp is the default.

open-port will not have any effect if the application is not exposed, and may
have a somewhat delayed effect even if it is. This operation is transactional,
so changes will not be made unless the hook exits successfully.

Prior to Juju 2.9, when charms requested a particular port range to be opened,
Juju would automatically mark that port range as opened for all defined
application endpoints. As of Juju 2.9, charms can constrain opened port ranges
to a set of application endpoints by providing the --endpoints flag followed by
a comma-delimited list of application endpoints.

Kubernetes charms
The port will open directly regardless of whether the application is exposed or not.
This connects to the fact that juju expose currently has no effect on sidecar charms.
Additionally, it is currently not possible to designate a range of ports to open for
Kubernetes charms; to open a range, you will have to run open-port multiple times.


---

# opened-ports

## Summary
List all ports or port ranges opened by the unit.

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `--endpoints` | false | display the list of target application endpoints for each port range |
| `--format` | smart | Specify output format (json&#x7c;smart&#x7c;yaml) |
| `-o`, `--output` |  | Specify an output file |

## Examples

    opened-ports


## Details

opened-ports lists all ports or port ranges opened by a unit.

By default, the port range listing does not include information about the 
application endpoints that each port range applies to. Each list entry is
formatted as &lt;port&gt;/&lt;protocol&gt; (e.g. "80/tcp") or &lt;from&gt;-&lt;to&gt;/&lt;protocol&gt; 
(e.g. "8080-8088/udp").

If the --endpoints option is specified, each entry in the port list will be
augmented with a comma-delimited list of endpoints that the port range 
applies to (e.g. "80/tcp (endpoint1, endpoint2)"). If a port range applies to
all endpoints, this will be indicated by the presence of a '*' character
(e.g. "80/tcp (*)").

Opening ports is transactional (i.e. will take place on successfully exiting
the current hook), and therefore opened-ports will not return any values for
pending open-port operations run from within the same hook.


---

# relation-get

## Summary
Get relation settings.

## Usage
``` relation-get [options] <key> <unit id>```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `--app` | false | Get the relation data for the overall application, not just a unit |
| `--format` | smart | Specify output format (json&#x7c;smart&#x7c;yaml) |
| `-o`, `--output` |  | Specify an output file |
| `-r`, `--relation` |  | Specify a relation by id |

## Examples

    # Getting the settings of the default unit in the default relation is done with:
    $ relation-get
    username: jim
    password: "12345"

    # To get a specific setting from the default remote unit in the default relation
    $ relation-get username
    jim

    # To get all settings from a particular remote unit in a particular relation you
    $ relation-get -r database:7 - mongodb/5
    username: bob
    password: 2db673e81ffa264c


## Details

relation-get prints the value of a unit's relation setting, specified by key.
If no key is given, or if the key is "-", all keys and values will be printed.

A unit can see its own settings by calling "relation-get - MYUNIT", this will include
any changes that have been made with "relation-set".

When reading remote relation data, a charm can call relation-get --app - to get
the data for the application data bag that is set by the remote applications
leader.

Further details:
relation-get reads the settings of the local unit, or of any remote unit, in a given
relation (set with -r, defaulting to the current relation identifier, as in relation-set).
The first argument specifies the settings key, and the second the remote unit, which may
be omitted if a default is available (that is, when running a relation hook other
than -relation-broken).

If the first argument is omitted, a dictionary of all current keys and values will be
printed; all values are always plain strings without any interpretation. If you need to
specify a remote unit but want to see all settings, use - for the first argument.

The environment variable JUJU_REMOTE_UNIT stores the default remote unit.

You should never depend upon the presence of any given key in relation-get output.
Processing that depends on specific values (other than private-address) should be
restricted to -relation-changed hooks for the relevant unit, and the absence of a
remote unit’s value should never be treated as an error in the local unit.

In practice, it is common and encouraged for -relation-changed hooks to exit early,
without error, after inspecting relation-get output and determining the data is
inadequate; and for all other hooks to be resilient in the face of missing keys,
such that -relation-changed hooks will be sufficient to complete all configuration
that depends on remote unit settings.

Key value pairs for remote units that have departed remain accessible for the lifetime
of the relation.


---

# relation-ids

## Summary
List all relation IDs for the given endpoint.

## Usage
``` relation-ids [options] <name>```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `--format` | smart | Specify output format (json&#x7c;smart&#x7c;yaml) |
| `-o`, `--output` |  | Specify an output file |

## Examples

    relation-ids database


## Details

relation-ids outputs a list of the related applications with a relation name.
Accepts a single argument (relation-name) which, in a relation hook, defaults
to the name of the current relation. The output is useful as input to the
relation-list, relation-get, relation-set, and relation-model-get commands
to read or write other relation values.

Only relation ids for relations which are not broken are included.


---

# relation-list

## Summary
List relation units.

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `--app` | false | List remote application instead of participating units |
| `--format` | smart | Specify output format (json&#x7c;smart&#x7c;yaml) |
| `-o`, `--output` |  | Specify an output file |
| `-r`, `--relation` |  | Specify a relation by id |

## Details

-r must be specified when not in a relation hook

relation-list outputs a list of all the related units for a relation identifier.
If not running in a relation hook context, -r needs to be specified with a
relation identifier similar to the relation-get and relation-set commands.


---

# relation-model-get

## Summary
Get details about the model hosing a related application.

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `--format` | smart | Specify output format (json&#x7c;smart&#x7c;yaml) |
| `-o`, `--output` |  | Specify an output file |
| `-r`, `--relation` |  | Specify a relation by id |

## Details

-r must be specified when not in a relation hook

relation-model-get outputs details about the model hosting the application
on the other end of a unit relation.
If not running in a relation hook context, -r needs to be specified with a
relation identifier similar to the relation-get and relation-set commands.


---

# relation-set

## Summary
Set relation settings.

## Usage
``` relation-set [options] key=value [key=value ...]```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `--app` | false | pick whether you are setting "application" settings or "unit" settings |
| `--file` |  | file containing key-value pairs |
| `--format` |  | deprecated format flag |
| `-r`, `--relation` |  | specify a relation by id |

## Examples

    relation-set port=80 tuning=default

    relation-set -r server:3 username=jim password=12345


## Details

"relation-set" writes the local unit's settings for some relation.
If no relation is specified then the current relation is used. The
setting values are not inspected and are stored as strings. Setting
an empty string causes the setting to be removed. Duplicate settings
are not allowed.

If the unit is the leader, it can set the application settings using
"--app". These are visible to related applications via 'relation-get --app'
or by supplying the application name to 'relation-get' in place of
a unit name.

The --file option should be used when one or more key-value pairs are
too long to fit within the command length limit of the shell or
operating system. The file will contain a YAML map containing the
settings.  Settings in the file will be overridden by any duplicate
key-value arguments. A value of "-" for the filename means &lt;stdin&gt;.

Further details:
relation-set writes the local unit’s settings for some relation. If it’s not running in a
relation hook, -r needs to be specified. The value part of an argument is not inspected,
and is stored directly as a string. Setting an empty string causes the setting to be removed.

relation-set is the tool for communicating information between units of related applications.
By convention the charm that provides an interface is likely to set values, and a charm that
requires that interface will read values; but there is nothing enforcing this. Whatever
information you need to propagate for the remote charm to work must be propagated via relation-set,
with the single exception of the private-address key, which is always set before the unit joins.

For some charms you may wish to overwrite the private-address setting, for example if you’re
writing a charm that serves as a proxy for some external application. It is rarely a good idea
to remove that key though, as most charms expect that value to exist unconditionally and may
fail if it is not present.

All values are set in a transaction at the point when the hook terminates successfully
(i.e. the hook exit code is 0). At that point all changed values will be communicated to
the rest of the system, causing -changed hooks to run in all related units.

There is no way to write settings for any unit other than the local unit. However, any hook
on the local unit can write settings for any relation which the local unit is participating in.


---

# resource-get

## Summary
Get the path to the locally cached resource file.

## Usage
``` resource-get [options] <resource name>```

## Examples

    # resource-get software
    /var/lib/juju/agents/unit-resources-example-0/resources/software/software.zip


## Details

"resource-get" is used while a hook is running to get the local path
to the file for the identified resource. This file is an fs-local copy,
unique to the unit for which the hook is running. It is downloaded from
the controller, if necessary.

If "resource-get" for a resource has not been run before (for the unit)
then the resource is downloaded from the controller at the revision
associated with the unit's application. That file is stored in the unit's
local cache. If "resource-get" *has* been run before then each
subsequent run syncs the resource with the controller. This ensures
that the revision of the unit-local copy of the resource matches the
revision of the resource associated with the unit's application.

Either way, the path provided by "resource-get" references the
up-to-date file for the resource. Note that the resource may get
updated on the controller for the application at any time, meaning the
cached copy *may* be out of date at any time after you call
"resource-get". Consequently, the command should be run at every
point where it is critical that the resource be up to date.

The "upgrade-charm" hook is useful for keeping your charm's resources
on a unit up to date.  Run "resource-get" there for each of your
charm's resources to do so. The hook fires whenever the the file for
one of the application's resources changes on the controller (in addition
to when the charm itself changes). That means it happens in response
to "juju upgrade-charm" as well as to "juju push-resource".

Note that the "upgrade-charm" hook does not run when the unit is
started up. So be sure to run "resource-get" for your resources in the
"install" hook (or "config-changed", etc.).

Note that "resource-get" only provides an FS path to the resource file.
It does not provide any information about the resource (e.g. revision).

Further details:
resource-get fetches a resource from the Juju controller or Charmhub.
The command returns a local path to the file for a named resource.

If resource-get has not been run for the named resource previously, then the
resource is downloaded from the controller at the revision associated with
the unit’s application. That file is stored in the unit’s local cache.
If resource-get has been run before then each subsequent run synchronizes the
resource with the controller. This ensures that the revision of the unit-local
copy of the resource matches the revision of the resource associated with the
unit’s application.

The path provided by resource-get references the up-to-date file for the resource.
Note that the resource may get updated on the controller for the application at
any time, meaning the cached copy may be out of date at any time after
resource-get is called. Consequently, the command should be run at every point
where it is critical for the resource be up to date.


---

# secret-add

## Summary
Add a new secret.

## Usage
``` secret-add [options] [key[#base64|#file]=value...]```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `--description` |  | the secret description |
| `--expire` |  | either a duration or time when the secret should expire |
| `--file` |  | a YAML file containing secret key values |
| `--label` |  | a label used to identify the secret in hooks |
| `--owner` | application | the owner of the secret, either the application or unit |
| `--rotate` |  | the secret rotation policy |

## Examples

    secret-add token=34ae35facd4
    secret-add key#base64=AA==
    secret-add key#file=/path/to/file another-key=s3cret
    secret-add --owner unit token=s3cret 
    secret-add --rotate monthly token=s3cret 
    secret-add --expire 24h token=s3cret 
    secret-add --expire 2025-01-01T06:06:06 token=s3cret 
    secret-add --label db-password \
        --description "my database password" \
        data#base64=s3cret== 
    secret-add --label db-password \
        --description "my database password" \
        --file=/path/to/file


## Details

Add a secret with a list of key values.

If a key has the '#base64' suffix, the value is already in base64 format and no
encoding will be performed, otherwise the value will be base64 encoded
prior to being stored.

If a key has the '#file' suffix, the value is read from the corresponding file.

By default, a secret is owned by the application, meaning only the unit
leader can manage it. Use "--owner unit" to create a secret owned by the
specific unit which created it.


---

# secret-get

## Summary
Get the content of a secret.

## Usage
``` secret-get [options] <ID> [key[#base64]]```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `--format` | yaml | Specify output format (json&#x7c;yaml) |
| `--label` |  | a label used to identify the secret in hooks |
| `-o`, `--output` |  | Specify an output file |
| `--peek` | false | get the latest revision just this time |
| `--refresh` | false | get the latest revision and also get this same revision for subsequent calls |

## Examples

    secret-get secret:9m4e2mr0ui3e8a215n4g
    secret-get secret:9m4e2mr0ui3e8a215n4g token
    secret-get secret:9m4e2mr0ui3e8a215n4g token#base64
    secret-get secret:9m4e2mr0ui3e8a215n4g --format json
    secret-get secret:9m4e2mr0ui3e8a215n4g --peek
    secret-get secret:9m4e2mr0ui3e8a215n4g --refresh
    secret-get secret:9m4e2mr0ui3e8a215n4g --label db-password


## Details

Get the content of a secret with a given secret ID.
The first time the value is fetched, the latest revision is used.
Subsequent calls will always return this same revision unless
--peek or --refresh are used.
Using --peek will fetch the latest revision just this time.
Using --refresh will fetch the latest revision and continue to
return the same revision next time unless --peek or --refresh is used.

Either the ID or label can be used to identify the secret.


---

# secret-grant

## Summary
Grant access to a secret.

## Usage
``` secret-grant [options] <ID>```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-r`, `--relation` |  | the relation with which to associate the grant |
| `--unit` |  | the unit to grant access |

## Examples

    secret-grant secret:9m4e2mr0ui3e8a215n4g -r 0 --unit mediawiki/6
    secret-grant secret:9m4e2mr0ui3e8a215n4g --relation db:2


## Details

Grant access to view the value of a specified secret.
Access is granted in the context of a relation - unless revoked
earlier, once the relation is removed, so too is the access grant.

By default, all units of the related application are granted access.
Optionally specify a unit name to limit access to just that unit.


---

# secret-ids

## Summary
Print secret IDs.

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `--format` | smart | Specify output format (json&#x7c;smart&#x7c;yaml) |
| `-o`, `--output` |  | Specify an output file |

## Examples

    secret-ids


## Details

Returns the secret ids for secrets owned by the application.


---

# secret-info-get

## Summary
Get a secret's metadata info.

## Usage
``` secret-info-get [options] <ID>```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `--format` | yaml | Specify output format (json&#x7c;yaml) |
| `--label` |  | a label used to identify the secret |
| `-o`, `--output` |  | Specify an output file |

## Examples

    secret-info-get secret:9m4e2mr0ui3e8a215n4g
    secret-info-get --label db-password


## Details

Get the metadata of a secret with a given secret ID.
Either the ID or label can be used to identify the secret.


---

# secret-remove

## Summary
Remove an existing secret.

## Usage
``` secret-remove [options] <ID>```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `--revision` | 0 | remove the specified revision |

## Examples

    secret-remove secret:9m4e2mr0ui3e8a215n4g


## Details

Remove a secret with the specified URI.


---

# secret-revoke

## Summary
Revoke access to a secret.

## Usage
``` secret-revoke [options] <ID>```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `--app`, `--application` |  | the application to revoke access |
| `-r`, `--relation` |  | the relation for which to revoke the grant |
| `--unit` |  | the unit to revoke access |

## Examples

    secret-revoke secret:9m4e2mr0ui3e8a215n4g
    secret-revoke secret:9m4e2mr0ui3e8a215n4g --relation 1
    secret-revoke secret:9m4e2mr0ui3e8a215n4g --app mediawiki
    secret-revoke secret:9m4e2mr0ui3e8a215n4g --unit mediawiki/6


## Details

Revoke access to view the value of a specified secret.
Access may be revoked from an application (all units of
that application lose access), or from a specified unit.
If run in a relation hook, the related application's 
access is revoked, unless a uni is specified, in which
case just that unit's access is revoked.'


---

# secret-set

## Summary
Update an existing secret.

## Usage
``` secret-set [options] <ID> [key[#base64]=value...]```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `--description` |  | the secret description |
| `--expire` |  | either a duration or time when the secret should expire |
| `--file` |  | a YAML file containing secret key values |
| `--label` |  | a label used to identify the secret in hooks |
| `--owner` | application | the owner of the secret, either the application or unit |
| `--rotate` |  | the secret rotation policy |

## Examples

    secret-set secret:9m4e2mr0ui3e8a215n4g token=34ae35facd4
    secret-set secret:9m4e2mr0ui3e8a215n4g key#base64 AA==
    secret-set secret:9m4e2mr0ui3e8a215n4g --rotate monthly token=s3cret 
    secret-set secret:9m4e2mr0ui3e8a215n4g --expire 24h
    secret-set secret:9m4e2mr0ui3e8a215n4g --expire 24h token=s3cret 
    secret-set secret:9m4e2mr0ui3e8a215n4g --expire 2025-01-01T06:06:06 token=s3cret 
    secret-set secret:9m4e2mr0ui3e8a215n4g --label db-password \
        --description "my database password" \
        data#base64 s3cret== 
    secret-set secret:9m4e2mr0ui3e8a215n4g --label db-password \
        --description "my database password"
    secret-set secret:9m4e2mr0ui3e8a215n4g --label db-password \
        --description "my database password" \
        --file=/path/to/file


## Details

Update a secret with a list of key values, or set new metadata.
If a value has the '#base64' suffix, it is already in base64 format and no
encoding will be performed, otherwise the value will be base64 encoded
prior to being stored.
To just update selected metadata like rotate policy, do not specify any secret value.


---

# state-delete

> See also: [state-get](#state-get), [state-set](#state-set)

## Summary
Delete server-side-state key value pairs.

## Usage
``` state-delete [options] <key>```

## Details

state-delete deletes the value of the server side state specified by key.


---

# state-get

> See also: [state-delete](#state-delete), [state-set](#state-set)

## Summary
Print server-side-state value.

## Usage
``` state-get [options] [<key>]```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `--format` | smart | Specify output format (json&#x7c;smart&#x7c;yaml) |
| `-o`, `--output` |  | Specify an output file |
| `--strict` | false | Return an error if the requested key does not exist |

## Details

state-get prints the value of the server side state specified by key.
If no key is given, or if the key is "-", all keys and values will be printed.


---

# state-set

> See also: [state-delete](#state-delete), [state-get](#state-get)

## Summary
Set server-side-state values.

## Usage
``` state-set [options] key=value [key=value ...]```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `--file` |  | file containing key-value pairs |

## Details

state-set sets the value of the server side state specified by key.

The --file option should be used when one or more key-value pairs
are too long to fit within the command length limit of the shell
or operating system. The file will contain a YAML map containing
the settings as strings.  Settings in the file will be overridden
by any duplicate key-value arguments. A value of "-" for the filename
means &lt;stdin&gt;.

The following fixed size limits apply:
- Length of stored keys cannot exceed 256 bytes.
- Length of stored values cannot exceed 65536 bytes.


---

# status-get

## Summary
Print status information.

## Usage
``` status-get [options] [--include-data] [--application]```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `--application` | false | print status for all units of this application if this unit is the leader |
| `--format` | smart | Specify output format (json&#x7c;smart&#x7c;yaml) |
| `--include-data` | false | print all status data |
| `-o`, `--output` |  | Specify an output file |

## Examples

    # Access the unit’s status:
    status-get
    status-get --include-data

    # Access the application’s status:
    status-get --application


## Details

By default, only the status value is printed.
If the --include-data flag is passed, the associated data are printed also.

Further details:
status-get allows charms to query the current workload status.

Without arguments, it just prints the status code e.g. ‘maintenance’.
With --include-data specified, it prints YAML which contains the status
value plus any data associated with the status.

Include the --application option to get the overall status for the application, rather than an individual unit.


---

# status-set

## Summary
Set status information.

## Usage
``` status-set [options] <maintenance | blocked | waiting | active> [message]```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `--application` | false | set this status for the application to which the unit belongs if the unit is the leader |

## Examples

Set the unit’s status

    # Set the unit's workload status to "maintenance".
    # This implies a short downtime that should self-resolve.
    status-set maintenance "installing software"
    status-set maintenance "formatting storage space, time left: 120s"

    # Set the unit's workload status to "waiting"
    # The workload is awaiting something else in the model to become active
    status-set waiting "waiting for database"

    # Set the unit workload's status to "active"
    # The workload is installed and running. Any messages should be informational.
    status-set active
    status-set active "Storage 95% full"

    # Set the unit's workload status to "blocked"
    # This implies human intervention is required to unblock the unit.
    # Messages should describe what is needed to resolve the problem.
    status-set blocked "Add a database relation"
    status-set blocked "Storage full"

Set the application’s status:

    # From a unit, update its status
    status-set maintenance "Upgrading to 4.1.1"

    # From the leader, update the application's status line
    status-set --application maintenance "Application upgrade underway"

Non-leader units which attempt to use --application will receive an error:

    $ status-set --application maintenance "I'm not the leader."
    error: this unit is not the leader


## Details

Sets the workload status of the charm. Message is optional.
The "last updated" attribute of the status is set, even if the
status and message are the same as what's already set.

Further details:
status-set changes what is displayed in juju status.
status-set allows charms to describe their current status.
This places the responsibility on the charm to know its status,
and set it accordingly using the status-set hook tool.
Changes made via status-set are applied without waiting for a
hook execution to end and are not rolled back if a hook
execution fails.

The leader unit is responsible for setting the overall status
of the application by using the --application option.

This hook tool takes 2 arguments. The first is the status code
and the second is a message to report to the user.

Valid status codes are:
    maintenance (the unit is not currently providing a service,
	  but expects to be soon, e.g. when first installing)
    blocked (the unit cannot continue without user input)
    waiting (the unit itself is not in error and requires no
	  intervention, but it is not currently in service as it
	  depends on some external factor, e.g. an application to
	  which it is related is not running)
    active (This unit believes it is correctly offering all
	  the services it is primarily installed to provide)

For more extensive explanations of these status codes, please see
the status reference page https://juju.is/docs/juju/status.

The second argument is a user-facing message, which will be displayed
to any users viewing the status, and will also be visible in the status
history. This can contain any useful information.

In the case of a blocked status though the status message should tell
the user explicitly how to unblock the unit insofar as possible, as this
is primary way of indicating any action to be taken (and may be surfaced
by other tools using Juju, e.g. the Juju GUI).

A unit in the active state with should not generally expect anyone to
look at its status message, and often it is better not to set one at
all. In the event of a degradation of service, this is a good place to
surface an explanation for the degradation (load, hardware failure
or other issue).

A unit in error state will have a message that is set by Juju and not
the charm because the error state represents a crash in a charm hook
- an unmanaged and uninterpretable situation. Juju will set the message
to be a reflection of the hook which crashed.
For example “Crashed installing the software” for an install hook crash
, or “Crash establishing database link” for a crash in a relationship hook.


---

# storage-add

## Summary
Add storage instances.

## Usage
``` storage-add [options] <charm storage name>[=count] ...```

## Examples

    storage-add database-storage=1


## Details

Storage add adds storage instances to unit using provided storage directives.
A storage directive consists of a storage name as per charm specification
and optional storage COUNT.

COUNT is a positive integer indicating how many instances
of the storage to create. If unspecified, COUNT defaults to 1.

Further details:

storage-add adds storage volumes to the unit.
storage-add takes the name of the storage volume (as defined in the
charm metadata), and optionally the number of storage instances to add.
By default, it will add a single storage instance of the name.


---

# storage-get

## Summary
Print information for the storage instance with the specified ID.

## Usage
``` storage-get [options] [<key>]```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `--format` | smart | Specify output format (json&#x7c;smart&#x7c;yaml) |
| `-o`, `--output` |  | Specify an output file |
| `-s` |  | specify a storage instance by id |

## Examples

    # retrieve information by UUID
    storage-get 21127934-8986-11e5-af63-feff819cdc9f

    # retrieve information by name
    storage-get -s data/0


## Details

When no &lt;key&gt; is supplied, all keys values are printed.

Further details:
storage-get obtains information about storage being attached
to, or detaching from, the unit.

If the executing hook is a storage hook, information about
the storage related to the hook will be reported; this may
be overridden by specifying the name of the storage as reported
by storage-list, and must be specified for non-storage hooks.

storage-get can be used to identify the storage location during
storage-attached and storage-detaching hooks. The exception to
this is when the charm specifies a static location for
singleton stores.


---

# storage-list

## Summary
List storage attached to the unit.

## Usage
``` storage-list [options] [<storage-name>]```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `--format` | smart | Specify output format (json&#x7c;smart&#x7c;yaml) |
| `-o`, `--output` |  | Specify an output file |

## Examples

    storage-list pgdata


## Details

storage-list will list the names of all storage instances
attached to the unit. These names can be passed to storage-get
via the "-s" flag to query the storage attributes.

A storage name may be specified, in which case only storage
instances for that named storage will be returned.

Further details:
storage-list list storages instances that are attached to the unit.
The storage instance identifiers returned from storage-list may be
passed through to the storage-get command using the -s option.


---

# unit-get

## Summary
Print public-address or private-address.

## Usage
``` unit-get [options] <setting>```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `--format` | smart | Specify output format (json&#x7c;smart&#x7c;yaml) |
| `-o`, `--output` |  | Specify an output file |

## Details

Further details:
unit-get returns the IP address of the unit.

It accepts a single argument, which must be
private-address or public-address. It is not
affected by context.

Note that if a unit has been deployed with
--bind space then the address returned from
unit-get private-address will get the address
from this space, not the ‘default’ space.


---


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


## List of hook tools

```{important}

This list replicates the output of `juju help hook-tool` and of `juju help-tool <name of hook tool`.

```

<!--Units deployed with Juju have a suite of tooling available to them, called ‘hook tools’. These commands provide the charm developer with a consistent interface to take action on the unit's behalf, such as opening ports, obtaining configuration, even determining which unit is the leader in a cluster. The listed hook-tools are available in any hook running on the unit, and are only available within ‘hook context’.-->


### `action-fail`

#### Usage 

```text
action-fail ["<failure message>"]
```

#### Summary

Set action fail status with message.

#### Details

action-fail sets the fail state of the action with a given error message.  Using
action-fail without a failure message will set a default message indicating a
problem with the action.


#### Examples

```bash
action-fail 'unable to contact remote service'
```


### `action-get`

#### Usage 

```text
action-get [options] [<key>[.<key>.<key>...]]
```

#### Summary
Get action parameters.

#### Options

```
--format  (= smart)
    Specify output format (json|smart|yaml)
-o, --output (= "")
    Specify an output file
```

#### Details

action-get will print the value of the parameter at the given key, serialized
as YAML.  If multiple keys are passed, action-get will recurse into the param
map as needed.



#### Examples


```bash
TIMEOUT=$(action-get timeout)
```

### `action-log`

#### Usage 

```text
action-log <message>
```

#### Summary

record a progress message for the current action


### `action-set`


#### Usage 

```
action-set <key>=<value> [<key>=<value> ...]
```

#### Summary

set action results

#### Details

action-set adds the given values to the results map of the Action. This map
is returned to the user after the completion of the Action. Keys must start
and end with lowercase alphanumeric, and contain only lowercase alphanumeric,
hyphens and periods.  The following special keys are reserved for internal use: 
"stdout", "stdout-encoding", "stderr", "stderr-encoding".

Example usage:

```text
 action-set outfile.size=10G
 action-set foo.bar=2
 action-set foo.baz.val=3
 action-set foo.bar.zab=4
 action-set foo.baz=1
```

 will yield:

```text
 outfile:
   size: "10G"
 foo:
   bar:
     zab: "4"
   baz: "1"
```


#### Examples

```bash
action-set answer 42
```



### `add-metric`

> The `add-metric` hook tool may only be executed from the `collect-metrics` hook.


#### Usage 

```text
add-metric [options] key1=value1 [key2=value2 ...]
```

#### Summary

Records a measurement which will be forwarded to the Juju controller. The same metric may not be collected twice in the same command.

#### Options

```text
-l, --labels (= "")
    labels to be associated with metric values
```


#### Examples

```bash
add-metric metric1=value1 [metric2=value2 …]
```



### `application-version-set`

#### Usage 

```text
application-version-set <new-version>
```

#### Summary

Specify which version of the application is deployed. This will be provided to users via `juju status`.

#### Details

application-version-set tells Juju which version of the application
software is running. This could be a package version number or some
other useful identifier, such as a Git hash, that indicates the
version of the deployed software. (It shouldn't be confused with the
charm revision.) The version set will be displayed in "juju status"
output for the application.


#### Examples


```bash
application-version-set 1.1.10
```

### `close-port`

#### Usage 

```text
close-port [options] <port>[/<protocol>] or <from>-<to>[/<protocol>] or icmp
```

#### Summary

Register a request to close a port or port range.

#### Options

```text
--endpoints (= "")
    a comma-delimited list of application endpoints to target with this operation
--format (= "")
    deprecated format flag
```

#### Details

close-port registers a request to close the specified port or port range.

By default, the specified port or port range will be closed for all defined
application endpoints. The --endpoints option can be used to constrain the
close request to a comma-delimited list of application endpoints.


`close-port` ensures a port, or port range, is not accessible from the public interface.


#### Examples

```bash
# Close single port
close-port 80

# Close a range of ports
close-port 9000-9999/udp

# Disable ICMP
close-port icmp

# Close a range of ports for a set of endpoints (since Juju 2.9)
close-port 80-90 --endpoints dmz,public
```


### `config-get`

#### Usage 

```text
config-get [options] [<key>]
```

#### Summary

Print application configuration.

#### Options

```text
-a, --all  (= false)
    print all keys
--format  (= smart)
    Specify output format (json|smart|yaml)
-o, --output (= "")
    Specify an output file
```

#### Details

<!--
When no `<key>` is supplied, all keys with values or defaults are printed. If
`--all` is set, all known keys are printed; those without defaults or values are
reported as null. `<key>` and `--all` are mutually exclusive.
-->

`config-get` returns information about the application configuration (as defined by `config.yaml`). If called without arguments, it returns a dictionary containing all config settings that are either explicitly set, or which have a non-nil default value. If the `--all` flag is passed, it returns a dictionary containing all defined config settings including nil values (for those without defaults). If called with a single argument, it returns the value of that config key. Missing config keys are reported as nulls, and do not return an error.



#### Examples

```bash
INTERVAL=$(config-get interval)

config-get --all
```

### `credential-get`


#### Usage 

```text
credential-get [options]
```

#### Summary

Access cloud credentials.

#### Options

```text
--format  (= smart)
    Specify output format (json|smart|yaml)
-o, --output (= "")
    Specify an output file
```

#### Details

credential-get returns the cloud specification used by the unit's model.


### `goal-state`

#### Usage 

```text
goal-state [options]
```

#### Summary

Print the status of the charm's peers and related units.

#### Options

```text
--format  (= yaml)
    Specify output format (json|yaml)
-o, --output (= "")
    Specify an output file
```

#### Details

'goal-state' command will list the charm units and relations, specifying their status and their relations to other units in different charms.


`goal-state` queries information about charm deployment and returns it as structured data.



`goal-state` provides:

-   the details of other peer units have been deployed and their status
-   the details of remote units on the other end of each endpoint and their status

The output will be a subset of that produced by the `juju status`. There will be output for sibling (peer) units and relation state per unit.

The unit status values are the workload status of the (sibling) peer units. We also use a unit status value of dying when the unit's life becomes dying. Thus unit status is one of:

`allocating`
`active`
`waiting`
`blocked`
`error`
`dying`

The relation status values are determined per unit and depend on whether the unit has entered or left scope. The possible values are:

- `joining` : a relation has been created, but no units are available. This occurs when the application on the other side of the relation is added to a model, but the machine hosting the first unit has not yet been provisioned. Calling `relation-set` will work correctly as that data will be passed through to the unit when it comes online, but `relation-get` will not provide any data.
- `joined` : the relation is active. A unit has entered scope and is accessible to this one.
- `broken` : unit has left, or is preparing to leave scope. Calling `relation-get` is not advised as the data will quickly out of date when the unit leaves.
- `suspended` : parent cross model relation is suspended
- `error`: an external error has been detected

By reporting error state, the charm has a chance to determine that goal state may not be reached due to some external cause. As with status, we will report the time since the status changed to allow the charm to empirically guess that a peer may have become stuck if it has not yet reached active state.



#### Examples


```bash
goal-state
```

### `is-leader`

#### Usage 

```text
is-leader [options]
```

#### Summary

Print application leadership status.

#### Options

```text
--format  (= smart)
    Specify output format (json|smart|yaml)
-o, --output (= "")
    Specify an output file
```


#### Details

is-leader prints a boolean indicating whether the local unit is guaranteed to
be application leader for at least 30 seconds. If it fails, you should assume that
there is no such guarantee.


`is-leader` indicates whether the current unit is the application leader.


`is-leader`will write `"True"` to STDOUT and return 0 if the unit is currently leader and can be guaranteed to remain so for 30 seconds.

Output can be expressed as `--format json` or `--format yaml` if desired.


#### Examples


```bash
LEADER=$(is-leader)
if [ "${LEADER}" == "True" ]; then
  # Do something a leader would do
fi
```

### `juju-log`

#### Usage 

```text
juju-log [options] <message>
```

#### Summary

Write a message to the juju log.

#### Options

```text
--debug  (= false)
    log at debug level
--format (= "")
    deprecated format flag
-l, --log-level (= "INFO")
    Send log message at the given level
```


`juju-log` writes messages directly to the unit's log file. Valid levels are: INFO, WARN, ERROR, DEBUG

#### Examples


```bash
juju-log -l 'WARN' Something has transpired
```


### `juju-reboot`


#### Usage 

```text
juju-reboot [options]
```

#### Summary

Reboot the host machine.

#### Options

```text
--now  (= false)
    reboot immediately, killing the invoking process
```

#### Details

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


#### Examples

```bash
# immediately reboot
juju-reboot --now

# Reboot after current hook exits
juju-reboot
```

### `k8s-raw-get`

#### Usage 

```text
k8s-raw-get
```

#### Summary

Get k8s raw spec information.

#### Details

Gets configuration data used to set up k8s resources.


### `k8s-raw-set`

#### Usage 

```text
k8s-raw-set [options] --file <core spec file>
```

#### Summary

Set k8s raw spec information.

#### Options

```text
--file  (= -)
    file containing k8s raw spec
```

#### Details

Sets configuration data in k8s raw format to use for k8s resources.
The spec applies to all units for the application.


### `k8s-spec-get`

#### Usage 

```text
k8s-spec-get
```

#### Summary

Get k8s spec information.

#### Details

Gets configuration data used to set up k8s resources.


### `k8s-spec-set`

#### Usage 

```text
k8s-spec-set [options] --file <core spec file> [--k8s-resources <k8s spec file>]
```

#### Summary

Set k8s spec information.

#### Options

```text
--file  (= -)
    file containing pod spec
--k8s-resources  (= )
    file containing k8s specific resources not yet modelled by Juju
```

#### Details

Sets configuration data to use for k8s resources.
The spec applies to all units for the application.

### `leader-get`
> :warning: The functionality provided by leader data (`leader-get` and `leader-set`) is now being replaced by "application-level relation data". See [`relation-get`](#heading--relation-get) and [`relation-set`](#heading--relation-set). 

#### Usage 

```text
leader-get [options] [<key>]
```

#### Summary

Print application leadership settings.

#### Options

```text
--format  (= smart)
    Specify output format (json|smart|yaml)
-o, --output (= "")
    Specify an output file
```	

#### Details

leader-get prints the value of a leadership setting specified by key. If no key
is given, or if the key is "-", all keys and values will be printed.


#### Examples:

``` text
ADDRESSS=$(leader-get cluster-leader-address)
```


### `leader-set`
> :warning: The functionality provided by leader data (`leader-get` and `leader-set`) is now being replaced by "application-level relation data". See [`relation-get`](#heading--relation-get) and [`relation-set`](#heading--relation-set). 


#### Usage 

```text
leader-set <key>=<value> {ref}`...]
```

#### Summary

Write application leadership settings.

#### Details

leader-set immediate writes the supplied key/value pairs to the controller,
which will then inform non-leader units of the change. It will fail if called
without arguments, or if called by a unit that is not currently application leader.


`leader-set` lets you distribute string key=value pairs to other units, but with the following differences:

-   there's only one leader-settings bucket per application (not one per unit)
-   only the leader can write to the bucket
-   only minions are informed of changes to the bucket
-   changes are propagated instantly

The instant propagation may be surprising, but it exists to satisfy the use case where shared data can be chosen by the leader at the very beginning of the install hook.

It is strongly recommended that leader settings are always written as a self-consistent group `leader-set one=one two=two three=three`.

#### Examples:


```bash
leader-set cluster-leader-address=10.0.0.123
```

### `network-get`

#### Usage 

```text
network-get [options] <binding-name> [--ingress-address] [--bind-address] [--egress-subnets]
```

#### Summary

Get network config.

#### Options

```text
--bind-address  (= false)
    get the address for the binding on which the unit should listen
--egress-subnets  (= false)
    get the egress subnets for the binding
--format  (= smart)
    Specify output format (json|smart|yaml)
--ingress-address  (= false)
    get the ingress address for the binding
-o, --output (= "")
    Specify an output file
--primary-address  (= false)
    (deprecated) get the primary address for the binding
-r, --relation  (= )
    specify a relation by id
```

#### Details

network-get returns the network config for a given binding name. By default
it returns the list of interfaces and associated addresses in the space for
the binding, as well as the ingress address for the binding. If defined, any
egress subnets are also returned.

If one of the following flags are specified, just that value is returned.

If more than one flag is specified, a map of values is returned.

```text
    --bind-address: the address the local unit should listen on to serve connections, as well
                    as the address that should be advertised to its peers.
    --ingress-address: the address the local unit should advertise as being used for incoming connections.
    --egress-subnets: subnets (in CIDR notation) from which traffic on this relation will originate.

```


`network-get` reports hostnames, IP addresses and CIDR blocks related to endpoint bindings.


By default it lists three pieces of address information:

-   binding address(es)
-   ingress address(es)
-   egress subnets


See [Discourse | Charm network primitives](https://discourse.charmhub.io/t/charm-network-primitives/1126) for in-depth coverage.



### `open-port`
> **Requires Juju 3.1+ for Kubernetes charms**

#### Usage 

```text
open-port [options] <port>[/<protocol>] or <from>-<to>[/<protocol>] or icmp
```

#### Summary

Register a request to open a port or port range.

#### Options

```text
--endpoints (= "")
    a comma-delimited list of application endpoints to target with this operation
--format (= "")
    deprecated format flag
```

#### Details

`open-port` registers a port or range to open on the public-interface.

By default, the specified port or port range will be opened for all defined
application endpoints. The --endpoints option can be used to constrain the
open request to a comma-delimited list of application endpoints.

The behavior differs a little bit between machine charms and Kubernetes charms.

**Machine charms.** On public clouds the port will only be open while the application is exposed. It accepts a single port or range of ports with an optional protocol, which may be `icmp`, `udp`, or `tcp`. `tcp` is the default.

`open-port` will not have any effect if the application is not exposed, and may have a somewhat delayed effect even if it is. This operation is transactional, so changes will not be made unless the hook exits successfully.

Prior to Juju 2.9, when charms requested a particular port range to be opened, Juju would automatically mark that port range as opened for  **all**  defined application endpoints. As of Juju 2.9, charms can constrain opened port ranges to a set of application endpoints by providing the `--endpoints` flag followed by a comma-delimited list of application endpoints.

**Kubernetes charms.** The port will open directly regardless of whether the application is exposed or not. This connects to the fact that `juju expose` currently has no effect on sidecar charms. Additionally, it is currently not possible to designate a range of ports to open for Kubernetes charms; to open a range, you will have to run `open-port` multiple times.


#### Examples:

Open port 80 to TCP traffic:

```bash
open-port 80/tcp
```
Open port 1234 to UDP traffic:

```bash
open-port 1234/udp
```

Open a range of ports to UDP traffic:

```bash
open-port 1000-2000/udp
```

Open a range of ports to TCP traffic for specific application endpoints (since Juju 2.9):

```bash
open-port 1000-2000/tcp --endpoints dmz,monitoring
```

### `opened-ports`
> The opened-ports hook tool lists all the ports currently opened **by the running charm**. It does not, at the moment, include ports which may be opened by other charms co-hosted on the same machine [lp#1427770](https://bugs.launchpad.net/juju-core/+bug/1427770).


#### Usage 

```text
opened-ports {ref}`options]
```

#### Summary

List all ports or port ranges opened by the unit.

#### Options

```text
--endpoints  (= false)
    display the list of target application endpoints for each port range
--format  (= smart)
    Specify output format (json|smart|yaml)
-o, --output (= "")
    Specify an output file
```	
	

#### Details

opened-ports lists all ports or port ranges opened by a unit.

By default, the port range listing does not include information about the 
application endpoints that each port range applies to. Each list entry is
formatted as <port>/<protocol> (e.g. "80/tcp") or <from>-<to>/<protocol> 
(e.g. "8080-8088/udp").

If the --endpoints option is specified, each entry in the port list will be
augmented with a comma-delimited list of endpoints that the port range 
applies to (e.g. "80/tcp (endpoint1, endpoint2)"). If a port range applies to
all endpoints, this will be indicated by the presence of a '*' character
(e.g. "80/tcp (*)").




Opening ports is transactional (i.e. will take place on successfully exiting the current hook), and therefore `opened-ports` will not return any values for pending `open-port` operations run from within the same hook.


#### Examples:


``` text
opened-ports
```

Prior to Juju 2.9, when charms requested a particular port range to be opened, Juju would automatically mark that port range as opened for **all** defined application endpoints.  As of Juju 2.9, charms can constrain opened port ranges to a set of application endpoints.  To ensure backwards compatibility, `opened-ports` will, by default, display the unique set of opened port ranges for all endpoints. To list of opened port ranges grouped by application endpoint can be obtained by running `opened-ports --endpoints`.

### `payload-register`

#### Usage 

```text
payload-register <type> <class> <id> [tags...]
```

#### Summary

Register a charm payload with Juju.

#### Details

"payload-register" is used while a hook is running to let Juju know that a
payload has been started. The information used to start the payload must be
provided when "register" is run.

The payload class must correspond to one of the payloads defined in
the charm's metadata.yaml.


An example fragment from `metadata.yaml`:

``` yaml
payloads:
    monitoring:
        type: docker
    kvm-guest:
        type: kvm
```


#### Examples:

```bash
payload-register monitoring docker 0fcgaba
```


### `payload-status-set`

#### Usage 

```text
payload-status-set <class> <id> <status>
```

#### Summary

Update the status of a payload.


#### Details

"payload-status-set" is used to update the current status of a registered payload.
The `<class>` and `<id>` provided must match a payload that has been previously
registered with juju using payload-register. The `<status>` must be one of the
follow: `starting`, `started`, `stopping`, `stopped`.

#### Examples:

```bash
payload-status-set monitor abcd13asa32c starting
```


### `payload-unregister`

#### Usage 

```text
payload-unregister <class> <id>
```

#### Summary

Stop tracking a payload.

#### Details

`payload-unregister` is used while a hook is running to let Juju know
that a payload has been manually stopped. The `<class>` and `<id>` provided
must match a payload that has been previously registered with juju using
`payload-register`.


#### Examples:

``` text
payload-unregister monitoring 0fcgaba
```


### `pod-spec-get`

#### Usage 

```text
pod-spec-get
```

#### Summary

Get k8s spec information (deprecated).

#### Details

Gets configuration data used to set up k8s resources.


### `pod-spec-set`

#### Usage 

```text
pod-spec-set [options] --file <core spec file> [--k8s-resources <k8s spec file>]
```

#### Summary

Set k8s spec information (deprecated).

#### Options

```text
--file  (= -)
    file containing pod spec
--k8s-resources  (= )
    file containing k8s specific resources not yet modelled by Juju
```

#### Details

Sets configuration data to use for k8s resources.
The spec applies to all units for the application.



### `relation-get`

#### Usage 

```text
relation-get [options] <key> <unit id>
```

#### Summary

Get relation settings.

#### Options

```text
--app  (= false)
    Get the relation data for the overall application, not just a unit
--format  (= smart)
    Specify output format (json|smart|yaml)
-o, --output (= "")
    Specify an output file
-r, --relation  (= )
    Specify a relation by id	
```	

#### Details


relation-get prints the value of a unit's relation setting, specified by key.
If no key is given, or if the key is "-", all keys and values will be printed.

A unit can see its own settings by calling "relation-get - MYUNIT", this will include
any changes that have been made with "relation-set".

When reading remote relation data, a charm can call relation-get --app - to get
the data for the application data bag that is set by the remote applications
leader.



Further details:


`relation-get` reads the settings of the local unit, or of any remote unit, in a given relation (set with `-r`, defaulting to the current relation identifier, as in `relation-set`). The first argument specifies the settings key, and the second the remote unit, which may be omitted if a default is available (that is, when running a relation hook other than `*-relation-broken`).

If the first argument is omitted, a dictionary of all current keys and values will be printed; all values are always plain strings without any interpretation. If you need to specify a remote unit but want to see all settings, use `-` for the first argument.

The environment variable {ref}`JUJU_REMOTE_UNIT` stores the default remote unit.

You should never depend upon the presence of any given key in `relation-get` output. Processing that depends on specific values (other than `private-address`) should be restricted to {ref}`*-relation-changed>` hooks for the relevant unit, and the absence of a remote unit's value should never be treated as an error in the local unit.

In practice, it is common and encouraged for {ref}`*-relation-changed` hooks to exit early, without error, after inspecting `relation-get` output and determining the data is inadequate; and for all other hooks to be resilient in the face of missing keys, such that -relation-changed hooks will be sufficient to complete all configuration that depends on remote unit settings.

Key value pairs for remote units that have departed remain accessible for the lifetime of the relation.


#### Examples:


``` text
# Getting the settings of the default unit in the default relation is done with:
 relation-get
  username: jim
  password: "12345"

# To get a specific setting from the default remote unit in the default relation
  relation-get username
   jim

# To get all settings from a particular remote unit in a particular relation you
   relation-get -r database:7 - mongodb/5
    username: bob
    password: 2db673e81ffa264c
```

### `relation-ids`


#### Usage relation-ids [options] <name>

#### Summary

List all relation ids with the given relation name.

#### Options

```text
--format  (= smart)
    Specify output format (json|smart|yaml)
-o, --output (= "")
    Specify an output file
```




`relation-ids` outputs a list of the related **applications** with a relation name. Accepts a single argument (relation-name) which, in a relation hook, defaults to the name of the current relation. The output is useful as input to the `relation-list`, `relation-get`, and `relation-set` commands to read or write other relation values.


#### Examples:

``` text
relation-ids database
```


### `relation-list`

#### Usage 

```text
relation-list [options]
```

#### Summary

List relation units.

#### Options

```text
--app  (= false)
    List remote application instead of participating units
--format  (= smart)
    Specify output format (json|smart|yaml)
-o, --output (= "")
    Specify an output file
-r, --relation  (= )
    Specify a relation by id
```

#### Details

`-r` must be specified when not in a relation hook.




`relation-list` outputs a list of all the related **units** for a relation identifier. If not running in a relation hook context, `-r` needs to be specified with a relation identifier similar to the`relation-get` and `relation-set` commands.


#### Examples: 

``` text
relation-list 9
```


### `relation-set`


#### Usage 

```text
relation-set [options] key=value [key=value ...]
```

#### Summary

Set relation settings.

#### Options

```text
--app  (= false)
    pick whether you are setting "application" settings or "unit" settings
--file  (= )
    file containing key-value pairs
--format (= "")
    deprecated format flag
-r, --relation  (= )
    specify a relation by id
```	

#### Details

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
key-value arguments. A value of "-" for the filename means <stdin>.


Further details:


`relation-set` writes the local unit's settings for some relation. If it's not running in a relation hook, `-r` needs to be specified. The `value` part of an argument is not inspected, and is stored directly as a string. Setting an empty string causes the setting to be removed.

`relation-set` is the tool for communicating information between units of related applications. By convention the charm that `provides` an interface is likely to set values, and a charm that `requires` that interface will read values; but there is nothing enforcing this. Whatever information you need to propagate for the remote charm to work must be propagated via relation-set, with the single exception of the `private-address` key, which is always set before the unit joins.

For some charms you may wish to overwrite the `private-address` setting, for example if you're writing a charm that serves as a proxy for some external application. It is rarely a good idea to *remove* that key though, as most charms expect that value to exist unconditionally and may fail if it is not present.

All values are set in a [transaction](https://en.wikipedia.org/wiki/Transaction_processing) at the point when the hook terminates successfully (i.e. the hook exit code is 0). At that point all changed values will be communicated to the rest of the system, causing -changed hooks to run in all related units.

There is no way to write settings for any unit other than the local unit. However, any hook on the local unit can write settings for any relation which the local unit is participating in.

#### Examples:

``` text
relation-set port=80 tuning=default

relation-set -r server:3 username=jim password=12345
```


### `resource-get`


#### Usage 

```text
resource-get <resource name>
```

#### Summary

Get the path to the locally cached resource file.

#### Details

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

`resource-get` fetches a resource from the Juju controller or the Juju Charm store. The command returns a local path to the file for a named resource.

If `resource-get` has not been run for the named resource previously, then the resource is downloaded from the controller at the revision associated with the unit's application. That file is stored in the unit's local cache. If `resource-get` *has* been run before then each subsequent run synchronizes the resource with the controller. This ensures that the revision of the unit-local copy of the resource matches the revision of the resource associated with the unit's application.

The path provided by `resource-get` references the up-to-date file for the resource. Note that the resource may get updated on the controller for the application at any time, meaning the cached copy *may* be out of date at any time after `resource-get` is called. Consequently, the command should be run at every point where it is critical for the resource be up to date.


#### Examples:

```bash
# resource-get software
/var/lib/juju/agents/unit-resources-example-0/resources/software/software.zip
```


### `secret-add`

#### Usage 

```text
secret-add {ref}`options] [key[#base64|#file]=value...]
```

#### Summary

Add a new secret.

#### Options

```text
--description (= "")
    the secret description
--expire (= "")
    either a duration or time when the secret should expire
--file (= "")
    a YAML file containing secret key values
--label (= "")
    a label used to identify the secret in hooks
--owner (= "application")
    the owner of the secret, either the application or unit
--rotate (= "")
    the secret rotation policy
```

#### Details

Add a secret with a list of key values.

If a key has the '#base64' suffix, the value is already in base64 format and no
encoding will be performed, otherwise the value will be base64 encoded
prior to being stored.

If a key has the '#file' suffix, the value is read from the corresponding file.

By default, a secret is owned by the application, meaning only the unit
leader can manage it. Use "--owner unit" to create a secret owned by the
specific unit which created it.

#### Examples:

```text

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
```


### `secret-get`


#### Usage 

```text
secret-get [options] <ID> [key[#base64]]
```

#### Summary

Get the content of a secret.

#### Options

```text
--format  (= yaml)
    Specify output format (json|yaml)
--label (= "")
    a label used to identify the secret in hooks
-o, --output (= "")
    Specify an output file
--peek  (= false)
    get the latest revision just this time
--refresh  (= false)
    get the latest revision and also get this same revision for subsequent calls
```


#### Details

Get the content of a secret with a given secret ID.
The first time the value is fetched, the latest revision is used.
Subsequent calls will always return this same revision unless
--peek or --refresh are used.
Using --peek will fetch the latest revision just this time.
Using --refresh will fetch the latest revision and continue to
return the same revision next time unless --peek or --refresh is used.

Either the ID or label can be used to identify the secret.

#### Examples:

```text
secret-get secret:9m4e2mr0ui3e8a215n4g
secret-get secret:9m4e2mr0ui3e8a215n4g token
secret-get secret:9m4e2mr0ui3e8a215n4g token#base64
secret-get secret:9m4e2mr0ui3e8a215n4g --format json
secret-get secret:9m4e2mr0ui3e8a215n4g --peek
secret-get secret:9m4e2mr0ui3e8a215n4g --refresh
secret-get secret:9m4e2mr0ui3e8a215n4g --label db-password
```

### `secret-grant`

#### Usage 

```text
secret-grant [options] <ID>
```

#### Summary

Grant access to a secret.

#### Options

```text
-r, --relation  (= )
    the relation with which to associate the grant
--unit (= "")
    the unit to grant access
```

#### Details

Grant access to view the value of a specified secret.
Access is granted in the context of a relation - unless revoked
earlier, once the relation is removed, so too is the access grant.

By default, all units of the related application are granted access.
Optionally specify a unit name to limit access to just that unit.

#### Examples:

```text
secret-grant secret:9m4e2mr0ui3e8a215n4g -r 0 --unit mediawiki/6
secret-grant secret:9m4e2mr0ui3e8a215n4g --relation db:2
```


### `secret-ids`

#### Usage 


```text
secret-ids [options]
```

#### Summary

Print secret ids.

#### Options

```text
--format  (= smart)
    Specify output format (json|smart|yaml)
-o, --output (= "")
    Specify an output file
```	

#### Details

Returns the secret ids for secrets owned by the application.

#### Examples:

```text
secret-ids
```


### `secret-info-get`

#### Usage 

```text
secret-info-get [options] <ID>
```

#### Summary

Get a secret's metadata info.

#### Options

```text
--format  (= yaml)
    Specify output format (json|yaml)
--label (= "")
    a label used to identify the secret
-o, --output (= "")
    Specify an output file
```	
	

#### Details

Get the metadata of a secret with a given secret ID.
Either the ID or label can be used to identify the secret.

#### Examples:

```text
secret-info-get --label db-password
secret-info-get --label db-password

```


### `secret-remove`


#### Usage 

```text
secret-remove [options] <ID>
```

#### Summary

remove a existing secret

#### Options

```
--revision  (= 0)
    remove the specified revision
```


#### Details

Remove a secret with the specified URI.

#### Examples:

```text
secret-remove secret:9m4e2mr0ui3e8a215n4g
```


### `secret-revoke`

#### Usage 

```text
secret-revoke [options] <ID>
```

#### Summary

Revoke access to a secret.

#### Options
```text
--app, --application (= "")
    the application to revoke access
-r, --relation  (= )
    the relation for which to revoke the grant
--unit (= "")
    the unit to revoke access
```	
	

#### Details

Revoke access to view the value of a specified secret.
Access may be revoked from an application (all units of
that application lose access), or from a specified unit.
If run in a relation hook, the related application's 
access is revoked, unless a uni is specified, in which
case just that unit's access is revoked.'

#### Examples:

```text
secret-revoke secret:9m4e2mr0ui3e8a215n4g
secret-revoke secret:9m4e2mr0ui3e8a215n4g --relation 1
secret-revoke secret:9m4e2mr0ui3e8a215n4g --app mediawiki
secret-revoke secret:9m4e2mr0ui3e8a215n4g --unit mediawiki/6
```

### `secret-set`

#### Usage 

```text
secret-set [options] <ID> [key[#base64]=value...]
```

#### Summary

Update an existing secret.

#### Options

```text
--description (= "")
    the secret description
--expire (= "")
    either a duration or time when the secret should expire
--file (= "")
    a YAML file containing secret key values
--label (= "")
    a label used to identify the secret in hooks
--owner (= "application")
    the owner of the secret, either the application or unit
--rotate (= "")
    the secret rotation policy
```	
	

#### Details

Update a secret with a list of key values, or set new metadata.
If a value has the '#base64' suffix, it is already in base64 format and no
encoding will be performed, otherwise the value will be base64 encoded
prior to being stored.
To just update selected metadata like rotate policy, do not specify any secret value.

#### Examples:

```text
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
```


### `state-delete`


#### Usage 

```text
state-delete <key>
```

#### Summary

Delete server-side-state key value pair.

#### Details

state-delete deletes the value of the server side state specified by key.

See also:

    state-get
    state-set


### `state-get`

#### Usage 

```text
state-get [options] [<key>]
```

#### Summary

Print server-side-state value.

#### Options

```text
--format  (= smart)
    Specify output format (json|smart|yaml)
-o, --output (= "")
    Specify an output file
--strict  (= false)
    Return an error if the requested key does not exist
```

#### Details

state-get prints the value of the server side state specified by key.
If no key is given, or if the key is "-", all keys and values will be printed.

See also:

    state-delete
    state-set



### `state-set`


#### Usage 

```text
state-set [options] key=value [key=value ...]
```

#### Summary

Set server-side-state values.

#### Options

```text
--file  (= )
    file containing key-value pairs
```

#### Details

state-set sets the value of the server side state specified by key.

The --file option should be used when one or more key-value pairs
are too long to fit within the command length limit of the shell
or operating system. The file will contain a YAML map containing
the settings as strings.  Settings in the file will be overridden
by any duplicate key-value arguments. A value of "-" for the filename
means <stdin>.

The following fixed size limits apply:
- Length of stored keys cannot exceed 256 bytes.
- Length of stored values cannot exceed 65536 bytes.

See also:

    state-delete
    state-get


### `status-get`

#### Usage 

```text
status-get [options] [--include-data] [--application]
```

#### Summary

Print status information.

#### Options

```text
--application  (= false)
    print status for all units of this application if this unit is the leader
--format  (= smart)
    Specify output format (json|smart|yaml)
--include-data  (= false)
    print all status data
-o, --output (= "")
    Specify an output file
```	

#### Details


By default, only the status value is printed.
If the --include-data flag is passed, the associated data are printed also.


Further details:

`status-get` allows charms to query the current workload status. 


Without arguments, it just prints the status code e.g. 'maintenance'. With `--include-data` specified, it prints YAML which contains the status value plus any data associated with the status.

Include the `--application` option to get the overall status for the application, rather than an individual unit.


#### Examples:

Access the unit's status:


``` text
status-get

status-get --include-data
```


Access the application's status:


``` text
status-get --application
```


### `status-set`

#### Usage 

```text
status-set [options] <maintenance | blocked | waiting | active> [message]
```

#### Summary

Set status information.

#### Options

```text
--application  (= false)
    set this status for the application to which the unit belongs if the unit is the leader
```	
	

#### Details

Sets the workload status of the charm. Message is optional.
The "last updated" attribute of the status is set, even if the
status and message are the same as what's already set.



Further details:

`status-set` changes what is displayed in `juju status`. 


`status-set` allows charms to describe their current status. This places the responsibility on the charm to know its status, and set it accordingly using the `status-set` hook tool. Changes made via `status-set` are applied without waiting for a hook execution to end and are not rolled back if a hook execution fails.

The leader unit is responsible for setting the overall status of the application by using the `--application` option.

This hook tool takes 2 arguments. The first is the status code and the second is a message to report to the user.

Valid status codes are:

-   `maintenance` (the unit is not currently providing a service, but expects to be soon, e.g. when first installing)
-   `blocked` (the unit cannot continue without user input)
-   `waiting` (the unit itself is not in error and requires no intervention, but it is not currently in service as it depends on some external factor, e.g. an application to which it is related is not running)
-   `active` (This unit believes it is correctly offering all the services it is primarily installed to provide)

For more extensive explanations of these status codes, [please see the status reference page <status>`.

The second argument is a user-facing message, which will be displayed to any users viewing the status, and will also be visible in the status history. This can contain any useful information.

In the case of a `blocked` status though the **status message should tell the user explicitly how to unblock the unit** insofar as possible, as this is primary way of indicating any action to be taken (and may be surfaced by other tools using Juju, e.g. the Juju GUI).

A unit in the `active` state with should not generally expect anyone to look at its status message, and often it is better not to set one at all. In the event of a degradation of service, this is a good place to surface an explanation for the degradation (load, hardware failure or other issue).

A unit in `error` state will have a message that is set by Juju and not the charm because the error state represents a crash in a charm hook - an unmanaged and uninterpretable situation. Juju will set the message to be a reflection of the hook which crashed. For example “Crashed installing the software” for an install hook crash, or “Crash establishing database link” for a crash in a relationship hook.



#### Examples:

Set the unit's status:


```bash
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
```


Set the application's status:

```bash
# From a unit, update its status
status-set maintenance "Upgrading to 4.1.1"

# From the leader, update the application's status line 
status-set --application maintenance "Application upgrade underway"
```

Non-leader units which attempt to use `--application` will receive an error:

``` text
status-set --application maintenance "I'm not the leader."
error: this unit is not the leader
```


### `storage-add`

#### Usage 

```text
storage-add <charm storage name>{ref}`=count] ...
```

#### Summary

Add storage instances.

#### Details

Storage add adds storage instances to unit using provided storage directives.
A storage directive consists of a storage name as per charm specification
and optional storage COUNT.

COUNT is a positive integer indicating how many instances
of the storage to create. If unspecified, COUNT defaults to 1.

Further details:

`storage-add` adds storage volumes to the unit.


`storage-add` takes the name of the storage volume (as defined in the charm metadata), and optionally the number of storage instances to add. By default, it will add a single storage instance of the name.


#### Examples:


``` text
storage-add database-storage=1
```



### `storage-get`

#### Usage 


```text
storage-get [options] [<key>]
```

#### Summary

Print information for storage instance with specified id.

#### Options

```text
--format  (= smart)
    Specify output format (json|smart|yaml)
-o, --output (= "")
    Specify an output file
-s  (= )
    specify a storage instance by id
```

#### Details

When no `<key>` is supplied, all keys values are printed.


Further details:

`storage-get` obtains information about storage being attached to, or detaching from, the unit. 


If the executing hook is a storage hook, information about the storage related to the hook will be reported; this may be overridden by specifying the name of the storage as reported by storage-list, and must be specified for non-storage hooks.

`storage-get` can be used to identify the storage location during storage-attached and storage-detaching hooks. The exception to this is when the charm specifies a static location for singleton stores.


#### Examples:


```bash
# retrieve information by UUID
storage-get 21127934-8986-11e5-af63-feff819cdc9f

# retrieve information by name
storage-get -s data/0
```



### `storage-list`

#### Usage 

```text
storage-list [options] [<storage-name>]
```

#### Summary

List storage attached to the unit.

#### Options

```text
--format  (= smart)
    Specify output format (json|smart|yaml)
-o, --output (= "")
    Specify an output file
```	

#### Details

storage-list will list the names of all storage instances
attached to the unit. These names can be passed to storage-get
via the "-s" flag to query the storage attributes.

A storage name may be specified, in which case only storage
instances for that named storage will be returned.


Further details:

`storage-list` list storages instances that are attached to the unit. 


The storage instance identifiers returned from `storage-list` may be passed through to the `storage-get` command using the -s option.


### `unit-get`
> :warning: `unit-get` is deprecated in favour of `network-get` hook tool. See [Discourse | Charm network primitives](https://discourse.charmhub.io/t/charm-network-primitives/1126) for in-depth coverage.


#### Usage 

```text
unit-get [options] <setting>
```

#### Summary

Print public-address or private-address.

#### Options

```text
--format  (= smart)
    Specify output format (json|smart|yaml)
-o, --output (= "")
    Specify an output file
```

Further details:

`unit-get` returns the IP address of the unit. 


It accepts a single argument, which must be `private-address` or `public-address`. It is not affected by context.

Note that if a unit has been deployed with `--bind space` then the address returned from `unit-get private-address` will get the address from this space, not the 'default' space.
[/details]

#### Examples:

``` text
unit-get public-address
```

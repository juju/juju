(command-juju-controller-config)=
# `juju controller-config`

```
Usage: juju controller-config [options] [<attribute key>[=<value>] ...]

Summary:
Displays or sets configuration settings for a controller.

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
--format  (= tabular)
    Specify output format (json|tabular|yaml)
-o, --output (= "")
    Specify an output file

Details:
By default, all configuration (keys and values) for the controller are
displayed if a key is not specified. Supplying one key name returns
only the value for that key.

Supplying key=value will set the supplied key to the supplied value;
this can be repeated for multiple keys. You can also specify a yaml
file containing key values. Not all keys can be updated after
bootstrap time.


The following keys are available:

agent-logfile-max-backups:
  type: int
  description: The number of old agent log files to keep (compressed)
agent-logfile-max-size:
  type: string
  description: The maximum size of the agent log file
agent-ratelimit-max:
  type: int
  description: The maximum size of the token bucket used to ratelimit agent connections
agent-ratelimit-rate:
  type: string
  description: The time taken to add a new token to the ratelimit bucket
api-port-open-delay:
  type: string
  description: "The duration that the controller will wait \nbetween when the controller
    has been deemed to be ready to open \nthe api-port and when the api-port is actually
    opened \n(only used when a controller-api-port value is set)."
application-resource-download-limit:
  type: int
  description: The maximum number of concurrent resources downloads per application
audit-log-capture-args:
  type: bool
  description: Determines if the audit log contains the arguments passed to API methods
audit-log-exclude-methods:
  type: list
  description: The list of Facade.Method names that aren't interesting for audit logging
    purposes.
audit-log-max-backups:
  type: int
  description: The number of old audit log files to keep (compressed)
audit-log-max-size:
  type: string
  description: The maximum size for the current controller audit log file
auditing-enabled:
  type: bool
  description: Determines if the controller records auditing information
batch-raft-fsm:
  type: bool
  description: Allow raft to use batch writing to the FSM.
caas-image-repo:
  type: string
  description: The docker repo to use for the jujud operator and mongo images
controller-api-port:
  type: int
  description: |-
    An optional port that may be set for controllers
    that have a very heavy load. If this port is set, this port is used by
    the controllers to talk to each other - used for the local API connection
    as well as the pubsub forwarders, and the raft workers. If this value is
    set, the api-port isn't opened until the controllers have started properly.
controller-resource-download-limit:
  type: int
  description: The maximum number of concurrent resources downloads across all the
    applications on the controller
features:
  type: list
  description: A list of runtime changeable features to be updated
juju-ha-space:
  type: string
  description: The network space within which the MongoDB replica-set should communicate
juju-mgmt-space:
  type: string
  description: The network space that agents should use to communicate with controllers
max-agent-state-size:
  type: int
  description: The maximum size (in bytes) of internal state data that agents can
    store to the controller
max-charm-state-size:
  type: int
  description: The maximum size (in bytes) of charm-specific state that units can
    store to the controller
max-debug-log-duration:
  type: string
  description: The maximum duration that a debug-log session is allowed to run
max-prune-txn-batch-size:
  type: int
  description: (deprecated) The maximum number of transactions evaluated in one go
    when pruning
max-prune-txn-passes:
  type: int
  description: (deprecated) The maximum number of batches processed when pruning
migration-agent-wait-time:
  type: string
  description: The maximum during model migrations that the migration worker will
    wait for agents to report on phases of the migration
model-logfile-max-backups:
  type: int
  description: The number of old model log files to keep (compressed)
model-logfile-max-size:
  type: string
  description: The maximum size of the log file written out by the controller on behalf
    of workers running for a model
model-logs-size:
  type: string
  description: The size of the capped collections used to hold the logs for the models
mongo-memory-profile:
  type: string
  description: Sets mongo memory profile
non-synced-writes-to-raft-log:
  type: bool
  description: Do not perform fsync calls after appending entries to the raft log.
    Disabling sync improves performance at the cost of reliability
prune-txn-query-count:
  type: int
  description: The number of transactions to read in a single query
prune-txn-sleep-time:
  type: string
  description: The amount of time to sleep between processing each batch query
public-dns-address:
  type: string
  description: Public DNS address (with port) of the controller.


Examples:

    juju controller-config
    juju controller-config api-port
    juju controller-config -c mycontroller
    juju controller-config auditing-enabled=true audit-log-max-backups=5
    juju controller-config auditing-enabled=true path/to/file.yaml
    juju controller-config path/to/file.yaml

See also:
    controllers
    model-config
    show-cloud
```
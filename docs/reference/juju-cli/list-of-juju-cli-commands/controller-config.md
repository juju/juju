(command-juju-controller-config)=
# `juju controller-config`
> See also: [controllers](#controllers), [model-config](#model-config), [show-cloud](#show-cloud)

## Summary
Displays or sets configuration settings for a controller.

## Usage
```juju controller-config [options] [<attribute key>[=<value>] ...]```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `-c`, `--controller` |  | Controller to operate in |
| `--color` | false | Use ANSI color codes in output |
| `--file` |  | path to yaml-formatted configuration file |
| `--format` | tabular | Specify output format (json&#x7c;tabular&#x7c;yaml) |
| `--ignore-read-only-fields` | false | Ignore read-only fields that might cause errors to be emitted while processing yaml documents |
| `--no-color` | false | Disable ANSI color codes in tabular output |
| `-o`, `--output` |  | Specify an output file |

## Examples


To view the value of a single config key for the current controller:

    juju controller-config <key>

To view the value of all config keys for the current controller in the json format:

    juju controller-config --format json

To view the values of all config keys for a different controller:

    juju controller-config -c <controller>

To set two keys in the current controller to a different value:

    juju controller-config <key>=<value> <key>=<value>

To save a controller's current config to a yaml file:

    juju controller-config --format=yaml > cfg.yaml

To set the current controller's config from a yaml file ignoring read-only fields,
then override the value for one key:

    juju controller-config --file path/to/file.yaml --ignore-read-only-fields <key>=<value>

To view all the configs from one file in yaml, then apply the same config values
to another controller from stdin using `|` and `-` (in `--file=-`):

    juju controller-config -c <controller> --format=yaml \
      | juju controller-config -c <controller> --file=- --ignore-read-only-fields


## Details


Controller configuration keys:

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
    prune-txn-query-count:
      type: int
      description: The number of transactions to read in a single query
    prune-txn-sleep-time:
      type: string
      description: The amount of time to sleep between processing each batch query
    public-dns-address:
      type: string
      description: Public DNS address (with port) of the controller.
    query-tracing-enabled:
      type: bool
      description: Enable query tracing for the dqlite driver
    query-tracing-threshold:
      type: string
      description: "The minimum duration of a query for it to be traced. The lower the
        \nthreshold, the more queries will be output. A value of 0 means all queries \nwill
        be output if tracing is enabled."
    ssh-max-concurrent-connections:
      type: int
      description: The maximum number of concurrent ssh connections to the controller
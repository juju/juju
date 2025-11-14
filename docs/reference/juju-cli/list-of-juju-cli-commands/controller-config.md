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
| `--file` |  | Path to yaml-formatted configuration file |
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

    juju controller-config --format=yaml > <configuration-filename>.yaml

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
    application-resource-download-limit:
      type: int
      description: The maximum number of concurrent resources downloads per application
    audit-log-capture-args:
      type: bool
      description: Determines if the audit log contains the arguments passed to API methods
    audit-log-exclude-methods:
      type: string
      description: A comma-delimited list of Facade.Method names that aren't interesting
        for audit logging purposes.
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
    controller-resource-download-limit:
      type: int
      description: The maximum number of concurrent resources downloads across all the
        applications on the controller
    dqlite-busy-timeout:
      type: string
      description: |-
        The timeout for how long a database operation will wait for a lock
        to be released before returning an error, that is the amount of
        time a writer will wait for others to finish writing on the
        same database.
    features:
      type: string
      description: A comma-delimited list of runtime changeable features to be updated
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
    object-store-s3-endpoint:
      type: string
      description: The s3 endpoint for the object store backend
    object-store-s3-static-key:
      type: string
      description: The s3 static key for the object store backend
    object-store-s3-static-secret:
      type: string
      description: The s3 static secret for the object store backend
    object-store-s3-static-session:
      type: string
      description: The s3 static session for the object store backend
    object-store-type:
      type: string
      description: The type of object store backend to use for storing blobs
    open-telemetry-enabled:
      type: bool
      description: Enable open telemetry tracing
    open-telemetry-endpoint:
      type: string
      description: Endpoint open telemetry tracing
    open-telemetry-insecure:
      type: bool
      description: Allows insecure endpoint for open telemetry tracing
    open-telemetry-sample-ratio:
      type: string
      description: Allows defining a sample ratio open telemetry tracing
    open-telemetry-stack-traces:
      type: bool
      description: Allows stack traces open telemetry tracing per span
    open-telemetry-tail-sampling-threshold:
      type: string
      description: Allows defining a tail sampling threshold open telemetry tracing
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
      description: |-
        The minimum duration of a query for it to be traced. The lower the
        threshold, the more queries will be output. A value of 0 means all
        queries will be output if tracing is enabled.
    ssh-max-concurrent-connections:
      type: int
      description: The maximum number of concurrent ssh connections to the controller
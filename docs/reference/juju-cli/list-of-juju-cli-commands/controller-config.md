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
| `--ignore-read-only-fields` | false | Ignore read only fields that might cause errors to be emitted while processing yaml documents |
| `--no-color` | false | Disable ANSI color codes in tabular output |
| `-o`, `--output` |  | Specify an output file |

## Examples

Print all config values for the current controller:

    juju controller-config

Print the value of "api-port" for the current controller:

    juju controller-config api-port

Print all config values for the controller "mycontroller":

    juju controller-config -c mycontroller

Set the "auditing-enabled" and "audit-log-max-backups" keys:

    juju controller-config auditing-enabled=true audit-log-max-backups=5

Set the current controller's config from a yaml file:

    juju controller-config --file path/to/file.yaml


## Details

To view all configuration values for the current controller, run
    juju controller-config
You can target a specific controller using the -c flag:
    juju controller-config -c <controller>
By default, the config will be printed in a tabular format. You can instead
print it in json or yaml format using the --format flag:
    juju controller-config --format json
    juju controller-config --format yaml

To view the value of a single config key, run
    juju controller-config key
To set config values, run
    juju controller-config key1=val1 key2=val2 ...

Config values can be imported from a yaml file using the --file flag:
    juju controller-config --file=path/to/cfg.yaml
This allows you to e.g. save a controller's config to a file:
    juju controller-config --format=yaml > cfg.yaml
and then import the config later. Note that the output of controller-config
may include read-only values, which will cause an error when importing later.
To prevent the error, use the --ignore-read-only-fields flag:
    juju controller-config --file=cfg.yaml --ignore-read-only-fields

You can also read from stdin using "-", which allows you to pipe config values
from one controller to another:
    juju controller-config -c c1 --format=yaml \
      | juju controller-config -c c2 --file=- --ignore-read-only-fields
You can simultaneously read config from a yaml file and set config keys
as above. The command-line args will override any values specified in the file.

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
      description: |-
        The duration that the controller will wait
        between when the controller has been deemed to be ready to open
        the api-port and when the api-port is actually opened
        (only used when a controller-api-port value is set).
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
    features:
      type: string
      description: A comma-delimited list of runtime changeable features to be updated
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
        threshold, the more queries will be output. A value of 0 means all queries
        will be output if tracing is enabled.
    ssh-max-concurrent-connections:
      type: int
      description: The maximum number of concurrent ssh connections to the controller
    ssh-server-port:
      type: int
      description: The port used for ssh connections to the controller
(list-of-controller-configuration-keys)=
# List of controller configuration keys

```{toctree}
:hidden:

controller-config-audit-log-exclude-methods
controller-config-juju-ha-space
controller-config-juju-mgmt-space
```

This document gives a list of all the configuration keys that can be applied to a Juju controller.
(controller-config-agent-logfile-max-backups)=
## `agent-logfile-max-backups`

`agent-logfile-max-backups` is the maximum number of old agent log files
to keep (compressed; saved on each unit, synced to the controller).

**Type:** integer

**Default value:** 2

**Can be changed after bootstrap:** yes


(controller-config-agent-logfile-max-size)=
## `agent-logfile-max-size`

`agent-logfile-max-size` is the maximum file size of each agent log file,
in MB.

**Type:** string

**Default value:** 100M

**Can be changed after bootstrap:** yes


(controller-config-agent-ratelimit-max)=
## `agent-ratelimit-max`

`agent-ratelimit-max` is the maximum size of the token bucket used to
ratelimit the agent connections to the API server.

**Type:** integer

**Default value:** 10

**Can be changed after bootstrap:** yes


(controller-config-agent-ratelimit-rate)=
## `agent-ratelimit-rate`

`agent-ratelimit-rate` is the interval at which a new token is added to
the token bucket, in milliseconds (ms).

**Type:** duration

**Default value:** 250ms

**Can be changed after bootstrap:** yes


(controller-config-allow-model-access)=
## `allow-model-access`

`allow-model-access` sets whether the controller will allow users to
connect to models they have been authorized for, even when
they don't have any access rights to the controller itself.

**Type:** boolean

**Default value:** false

**Can be changed after bootstrap:** no


(controller-config-api-port)=
## `api-port`

`api-port` is the port used for api connections.

**Type:** integer

**Default value:** 17070

**Can be changed after bootstrap:** no


(controller-config-api-port-open-delay)=
## `api-port-open-delay`

`api-port-open-delay` is a duration that the controller will wait
between when the controller has been deemed to be ready to open
the api-port and when the api-port is actually opened. This value
is only used when a controller-api-port value is set.

**Type:** duration

**Default value:** 2s

**Can be changed after bootstrap:** yes


(controller-config-application-resource-download-limit)=
## `application-resource-download-limit`

`application-resource-download-limit` limits the number of concurrent resource download
requests from unit agents which will be served. The limit is per application.
Use a value of 0 to disable the limit.

**Type:** integer

**Default value:** 0

**Can be changed after bootstrap:** yes


(controller-config-audit-log-capture-args)=
## `audit-log-capture-args`

`audit-log-capture-args` determines whether the audit log will
contain the arguments passed to API methods.

**Type:** boolean

**Default value:** false

**Can be changed after bootstrap:** yes


(controller-config-audit-log-exclude-methods)=
## `audit-log-exclude-methods`

`audit-log-exclude-methods` is a list of Facade.Method names that
aren't interesting for audit logging purposes. A conversation
with only calls to these will be excluded from the
log. (They'll still appear in conversations that have other
interesting calls though.).

**Type:** list[string]

**Can be changed after bootstrap:** yes


(controller-config-audit-log-max-backups)=
## `audit-log-max-backups`

`audit-log-max-backups` is the number of old audit log files to keep
(compressed).

**Type:** integer

**Default value:** 10

**Can be changed after bootstrap:** yes


(controller-config-audit-log-max-size)=
## `audit-log-max-size`

`audit-log-max-size` is the maximum size for the current audit log
file, eg "250M".

**Type:** string

**Default value:** 300M

**Can be changed after bootstrap:** yes


(controller-config-auditing-enabled)=
## `auditing-enabled`

`auditing-enabled` determines whether the controller will record
auditing information.

**Type:** boolean

**Default value:** true

**Can be changed after bootstrap:** yes


(controller-config-autocert-dns-name)=
## `autocert-dns-name`

`autocert-dns-name` sets the DNS name of the controller. If a
client connects to this name, an official certificate will be
automatically requested. Connecting to any other host name
will use the usual self-generated certificate.

**Type:** string

**Can be changed after bootstrap:** no


(controller-config-autocert-url)=
## `autocert-url`

`autocert-url` sets the URL used to obtain official TLS
certificates when a client connects to the API. By default,
certficates are obtains from LetsEncrypt. A good value for
testing is
"https://acme-staging.api.letsencrypt.org/directory".

**Type:** string

**Can be changed after bootstrap:** no


(controller-config-ca-cert)=
## `ca-cert`

`ca-cert` is the key for the controller's CA certificate attribute.

**Can be changed after bootstrap:** no


(controller-config-caas-image-repo)=
## `caas-image-repo`

`caas-image-repo` sets the docker repo to use
for the jujud operator and mongo images.

**Type:** string

**Can be changed after bootstrap:** yes


(controller-config-caas-operator-image-path)=
## `caas-operator-image-path`
> This key is deprecated.

`caas-operator-image-path` sets the URL of the docker image
used for the application operator.
Deprecated: use `caas-image-repo`.

**Type:** string

**Can be changed after bootstrap:** no


(controller-config-controller-api-port)=
## `controller-api-port`

`controller-api-port` is an optional port that may be set for controllers
that have a very heavy load. If this port is set, this port is used by
the controllers to talk to each other - used for the local API connection
as well as the pubsub forwarders, and the raft workers. If this value is
set, the api-port isn't opened until the controllers have started
properly.

**Type:** integer

**Can be changed after bootstrap:** yes


(controller-config-controller-name)=
## `controller-name`

`controller-name` is the canonical name for the controller.

**Type:** non-empty string

**Can be changed after bootstrap:** no


(controller-config-controller-resource-download-limit)=
## `controller-resource-download-limit`

`controller-resource-download-limit` limits the number of concurrent resource download
requests from unit agents which will be served. The limit is for the combined total
of all applications on the controller.
Use a value of 0 to disable the limit.

**Type:** integer

**Default value:** 0

**Can be changed after bootstrap:** yes


(controller-config-controller-uuid)=
## `controller-uuid`

`controller-uuid` is the key for the controller UUID attribute.

**Can be changed after bootstrap:** no


(controller-config-features)=
## `features`

`features` allows a list of runtime changeable features to be updated.

**Type:** list[string]

**Can be changed after bootstrap:** yes


(controller-config-identity-public-key)=
## `identity-public-key`

`identity-public-key` sets the public key of the identity manager.
Use this when users should be managed externally rather than
created locally on the controller.

**Type:** string

**Can be changed after bootstrap:** no


(controller-config-identity-url)=
## `identity-url`

`identity-url` sets the URL of the identity manager.
Use this when users should be managed externally rather than
created locally on the controller.

**Type:** string

**Can be changed after bootstrap:** no


(controller-config-juju-db-snap-channel)=
## `juju-db-snap-channel`

`juju-db-snap-channel` selects the channel to use when installing Mongo
snaps for focal or later. The value is ignored for older releases.

**Type:** string

**Default value:** 4.4/stable

**Can be changed after bootstrap:** no


(controller-config-juju-ha-space)=
## `juju-ha-space`

`juju-ha-space` is the network space within which the MongoDB replica-set
should communicate.

**Type:** string

**Can be changed after bootstrap:** yes


(controller-config-juju-mgmt-space)=
## `juju-mgmt-space`

`juju-mgmt-space` is the network space that agents should use to
communicate with controllers.

**Type:** string

**Can be changed after bootstrap:** yes


(controller-config-jujud-controller-snap-source)=
## `jujud-controller-snap-source`

`jujud-controller-snap-source` returns the source for the controller snap.
Can be set to "legacy", "snapstore", "local" or "local-dangerous".
Cannot be changed.

**Type:** string

**Default value:** legacy

**Can be changed after bootstrap:** no


(controller-config-login-token-refresh-url)=
## `login-token-refresh-url`

`login-token-refresh-url` sets the URL of the login JWT well-known endpoint.
Use this when authentication/authorisation is done using a JWT in the
login request rather than a username/password or macaroon and a local
permissions model.

**Type:** string

**Can be changed after bootstrap:** no


(controller-config-max-agent-state-size)=
## `max-agent-state-size`

`max-agent-state-size` is the maximum allowed size of internal state
data that agents can store to the controller in bytes. A value of 0
disables the quota checks although in principle, mongo imposes a
hard (but configurable) limit of 16M.

**Type:** integer

**Default value:** 524288

**Can be changed after bootstrap:** yes


(controller-config-max-charm-state-size)=
## `max-charm-state-size`

`max-charm-state-size` is the maximum allowed size of charm-specific
per-unit state data that charms can store to the controller in
bytes. A value of 0 disables the quota checks although in
principle, mongo imposes a hard (but configurable) limit of 16M.

**Type:** integer

**Default value:** 2097152

**Can be changed after bootstrap:** yes


(controller-config-max-debug-log-duration)=
## `max-debug-log-duration`

`max-debug-log-duration` is used to provide a backstop to the execution of a
debug-log command. If someone starts a debug-log session in a remote
screen for example, it is very easy to disconnect from the screen while
leaving the debug-log process running. This causes unnecessary load on
the API server. The max debug-log duration has a default of 24 hours,
which should be more than enough time for a debugging session.

**Type:** duration

**Default value:** 24h0m0s

**Can be changed after bootstrap:** yes


(controller-config-max-prune-txn-batch-size)=
## `max-prune-txn-batch-size`
> This key is deprecated.

`max-prune-txn-batch-size` (deprecated) is the maximum number of transactions
we will evaluate in one go when pruning. Default is 1M transactions.
A value <= 0 indicates to do all transactions at once.

**Type:** integer

**Default value:** 1000000

**Can be changed after bootstrap:** yes


(controller-config-max-prune-txn-passes)=
## `max-prune-txn-passes`
> This key is deprecated.

`max-prune-txn-passes` (deprecated) is the maximum number of batches that
we will process. So total number of transactions that can be processed
is `max-prune-txn-batch-size` * `max-prune-txn-passes`. A value <= 0 implies
'do a single pass'. If both `max-prune-txn-batch-size` and `max-prune-txn-passes`
are 0, then the default value of 1M BatchSize and 100 passes
will be used instead.

**Type:** integer

**Default value:** 100

**Can be changed after bootstrap:** yes


(controller-config-max-txn-log-size)=
## `max-txn-log-size`

`max-txn-log-size` is the maximum size the of capped txn log collection, eg "10M".

**Type:** string

**Default value:** 10M

**Can be changed after bootstrap:** no


(controller-config-metering-url)=
## `metering-url`

`metering-url` is the URL to use for metrics.

**Type:** string

**Default value:** https://api.jujucharms.com/omnibus/v3

**Can be changed after bootstrap:** no


(controller-config-migration-agent-wait-time)=
## `migration-agent-wait-time`

`migration-agent-wait-time` is the maximum time that the migration-master
worker will wait for agents to report for a migration phase when
executing a model migration.

**Type:** duration

**Default value:** 15m0s

**Can be changed after bootstrap:** yes


(controller-config-model-logfile-max-backups)=
## `model-logfile-max-backups`

`model-logfile-max-backups` is the number of old model
log files to keep (compressed).

**Type:** integer

**Default value:** 2

**Can be changed after bootstrap:** yes


(controller-config-model-logfile-max-size)=
## `model-logfile-max-size`

`model-logfile-max-size` is the maximum size of the log file written out by the
controller on behalf of workers running for a model.

**Type:** string

**Default value:** 10M

**Can be changed after bootstrap:** yes


(controller-config-model-logs-size)=
## `model-logs-size`

`model-logs-size` is the size of the capped collections used to hold the
logs for the models, eg "20M". Size is per model.

**Type:** string

**Default value:** 20M

**Can be changed after bootstrap:** yes


(controller-config-mongo-memory-profile)=
## `mongo-memory-profile`

`mongo-memory-profile` sets the memory profile for MongoDB. Valid values are:
- "low": use the least possible memory
- "default": use the default memory profile.

**Type:** string

**Default value:** default

**Can be changed after bootstrap:** yes


(controller-config-prune-txn-query-count)=
## `prune-txn-query-count`

`prune-txn-query-count` is the number of transactions to read in a single query.
Minimum of 10, a value of 0 will indicate to use the default value (1000).

**Type:** integer

**Default value:** 1000

**Can be changed after bootstrap:** yes


(controller-config-prune-txn-sleep-time)=
## `prune-txn-sleep-time`

`prune-txn-sleep-time` is the amount of time to sleep between processing each
batch query. This is used to reduce load on the system, allowing other
queries to time to operate. On large controllers, processing 1000 txs
seems to take about 100ms, so a sleep time of 10ms represents a 10%
slowdown, but allows other systems to operate concurrently.
A negative number will indicate to use the default, a value of 0
indicates to not sleep at all.

**Type:** duration

**Default value:** 10ms

**Can be changed after bootstrap:** yes


(controller-config-public-dns-address)=
## `public-dns-address`

`public-dns-address` is the public DNS address (and port) of the controller.

**Type:** string

**Can be changed after bootstrap:** yes


(controller-config-query-tracing-enabled)=
## `query-tracing-enabled`

`query-tracing-enabled` returns whether query tracing is enabled. If so, any
queries which take longer than `query-tracing-threshold` will be logged.

**Type:** boolean

**Default value:** false

**Can be changed after bootstrap:** yes


(controller-config-query-tracing-threshold)=
## `query-tracing-threshold`

`query-tracing-threshold` returns the "threshold" for query tracing. Any
queries which take longer than this value will be logged (if query tracing
is enabled). The lower the threshold, the more queries will be output. A
value of 0 means all queries will be output.

**Type:** duration

**Default value:** 1s

**Can be changed after bootstrap:** yes


(controller-config-set-numa-control-policy)=
## `set-numa-control-policy`
> This key is deprecated.

`set-numa-control-policy` (true/false) is deprecated.
Use to configure whether mongo is started with NUMA
controller policy turned on.

**Type:** boolean

**Default value:** false

**Can be changed after bootstrap:** no


(controller-config-ssh-max-concurrent-connections)=
## `ssh-max-concurrent-connections`

`ssh-max-concurrent-connections` is the maximum number of concurrent SSH
connections to the controller.

**Type:** integer

**Default value:** 100

**Can be changed after bootstrap:** yes


(controller-config-ssh-server-port)=
## `ssh-server-port`

`ssh-server-port` is the port used for the embedded SSH server.

**Type:** integer

**Default value:** 17022

**Can be changed after bootstrap:** no


(controller-config-state-port)=
## `state-port`

`state-port` is the port used for mongo connections.

**Type:** integer

**Default value:** 37017

**Can be changed after bootstrap:** no



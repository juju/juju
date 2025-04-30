// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/utils/v4"
	"gopkg.in/yaml.v2"

	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/internal/configschema"
	"github.com/juju/juju/internal/pki"
)

const (
	// MongoProfLow represents the most conservative mongo memory profile.
	MongoProfLow = "low"
	// MongoProfDefault represents the mongo memory profile shipped by default.
	MongoProfDefault = "default"
)

// docs:controller-config-keys
const (
	// APIPort is the port used for api connections.
	APIPort = "api-port"

	// ControllerAPIPort is an optional port that may be set for controllers
	// that have a very heavy load. If this port is set, this port is used by
	// the controllers to talk to each other - used for the local API connection
	// as well as the pubsub forwarders. If this value is set, the api-port
	// isn't opened until the controllers have started properly.
	ControllerAPIPort = "controller-api-port"

	// ControllerName is the canonical name for the controller.
	ControllerName = "controller-name"

	// ApplicationResourceDownloadLimit limits the number of concurrent resource download
	// requests from unit agents which will be served. The limit is per application.
	// Use a value of 0 to disable the limit.
	ApplicationResourceDownloadLimit = "application-resource-download-limit"

	// ControllerResourceDownloadLimit limits the number of concurrent resource download
	// requests from unit agents which will be served. The limit is for the combined total
	// of all applications on the controller.
	// Use a value of 0 to disable the limit.
	ControllerResourceDownloadLimit = "controller-resource-download-limit"

	// AgentRateLimitMax is the maximum size of the token bucket used to
	// ratelimit the agent connections to the API server.
	AgentRateLimitMax = "agent-ratelimit-max"

	// AgentRateLimitRate is the interval at which a new token is added to
	// the token bucket, in milliseconds (ms).
	AgentRateLimitRate = "agent-ratelimit-rate"

	// APIPortOpenDelay is a duration that the controller will wait
	// between when the controller has been deemed to be ready to open
	// the api-port and when the api-port is actually opened. This value
	// is only used when a controller-api-port value is set.
	APIPortOpenDelay = "api-port-open-delay"

	// AuditingEnabled determines whether the controller will record
	// auditing information.
	AuditingEnabled = "auditing-enabled"

	// AuditLogCaptureArgs determines whether the audit log will
	// contain the arguments passed to API methods.
	AuditLogCaptureArgs = "audit-log-capture-args"

	// AuditLogMaxSize is the maximum size for the current audit log
	// file, eg "250M".
	AuditLogMaxSize = "audit-log-max-size"

	// AuditLogMaxBackups is the number of old audit log files to keep
	// (compressed).
	AuditLogMaxBackups = "audit-log-max-backups"

	// AuditLogExcludeMethods is a list of Facade.Method names that
	// aren't interesting for audit logging purposes. A conversation
	// with only calls to these will be excluded from the
	// log. (They'll still appear in conversations that have other
	// interesting calls though.)
	AuditLogExcludeMethods = "audit-log-exclude-methods"

	// ReadOnlyMethodsWildcard is the special value that can be added
	// to the exclude-methods list that represents all of the read
	// only methods (see apiserver/observer/auditfilter.go). This
	// value will be stored in the DB (rather than being expanded at
	// write time) so any changes to the set of read-only methods in
	// new versions of Juju will be honoured.
	ReadOnlyMethodsWildcard = "ReadOnlyMethods"

	// StatePort is the port used for mongo connections.
	StatePort = "state-port"

	// CACertKey is the key for the controller's CA certificate attribute.
	CACertKey = "ca-cert"

	// ControllerUUIDKey is the key for the controller UUID attribute.
	ControllerUUIDKey = "controller-uuid"

	// LoginTokenRefreshURL sets the URL of the login JWT well-known endpoint.
	// Use this when authentication/authorisation is done using a JWT in the
	// login request rather than a username/password or macaroon and a local
	// permissions model.
	LoginTokenRefreshURL = "login-token-refresh-url"

	// IdentityURL sets the URL of the identity manager.
	// Use this when users should be managed externally rather than
	// created locally on the controller.
	IdentityURL = "identity-url"

	// IdentityPublicKey sets the public key of the identity manager.
	// Use this when users should be managed externally rather than
	// created locally on the controller.
	IdentityPublicKey = "identity-public-key"

	// SetNUMAControlPolicyKey (true/false) is deprecated.
	// Use to configure whether mongo is started with NUMA
	// controller policy turned on.
	SetNUMAControlPolicyKey = "set-numa-control-policy"

	// AutocertDNSNameKey sets the DNS name of the controller. If a
	// client connects to this name, an official certificate will be
	// automatically requested. Connecting to any other host name
	// will use the usual self-generated certificate.
	AutocertDNSNameKey = "autocert-dns-name"

	// AutocertURLKey sets the URL used to obtain official TLS
	// certificates when a client connects to the API. By default,
	// certficates are obtains from LetsEncrypt. A good value for
	// testing is
	// "https://acme-staging.api.letsencrypt.org/directory".
	AutocertURLKey = "autocert-url"

	// AllowModelAccessKey sets whether the controller will allow users to
	// connect to models they have been authorized for, even when
	// they don't have any access rights to the controller itself.
	AllowModelAccessKey = "allow-model-access"

	// JujuDBSnapChannel selects the channel to use when installing Mongo
	// snaps for focal or later. The value is ignored for older releases.
	JujuDBSnapChannel = "juju-db-snap-channel"

	// MaxDebugLogDuration is used to provide a backstop to the execution of a
	// debug-log command. If someone starts a debug-log session in a remote
	// screen for example, it is very easy to disconnect from the screen while
	// leaving the debug-log process running. This causes unnecessary load on
	// the API server. The max debug-log duration has a default of 24 hours,
	// which should be more than enough time for a debugging session.
	MaxDebugLogDuration = "max-debug-log-duration"

	// AgentLogfileMaxSize is the maximum file size of each agent log file,
	// in MB.
	AgentLogfileMaxSize = "agent-logfile-max-size"

	// AgentLogfileMaxBackups is the maximum number of old agent log files
	// to keep (compressed; saved on each unit, synced to the controller).
	AgentLogfileMaxBackups = "agent-logfile-max-backups"

	// ModelLogfileMaxSize is the maximum size of the log file written out by the
	// controller on behalf of workers running for a model.
	ModelLogfileMaxSize = "model-logfile-max-size"

	// ModelLogfileMaxBackups is the number of old model
	// log files to keep (compressed).
	ModelLogfileMaxBackups = "model-logfile-max-backups"

	// MaxTxnLogSize is the maximum size the of capped txn log collection, eg "10M"
	MaxTxnLogSize = "max-txn-log-size"

	// MaxPruneTxnBatchSize (deprecated) is the maximum number of transactions
	// we will evaluate in one go when pruning. Default is 1M transactions.
	// A value <= 0 indicates to do all transactions at once.
	MaxPruneTxnBatchSize = "max-prune-txn-batch-size"

	// MaxPruneTxnPasses (deprecated) is the maximum number of batches that
	// we will process. So total number of transactions that can be processed
	// is MaxPruneTxnBatchSize * MaxPruneTxnPasses. A value <= 0 implies
	// 'do a single pass'. If both MaxPruneTxnBatchSize and MaxPruneTxnPasses
	// are 0, then the default value of 1M BatchSize and 100 passes
	// will be used instead.
	MaxPruneTxnPasses = "max-prune-txn-passes"

	// PruneTxnQueryCount is the number of transactions to read in a single query.
	// Minimum of 10, a value of 0 will indicate to use the default value (1000)
	PruneTxnQueryCount = "prune-txn-query-count"

	// PruneTxnSleepTime is the amount of time to sleep between processing each
	// batch query. This is used to reduce load on the system, allowing other
	// queries to time to operate. On large controllers, processing 1000 txs
	// seems to take about 100ms, so a sleep time of 10ms represents a 10%
	// slowdown, but allows other systems to operate concurrently.
	// A negative number will indicate to use the default, a value of 0
	// indicates to not sleep at all.
	PruneTxnSleepTime = "prune-txn-sleep-time"

	// MaxCharmStateSize is the maximum allowed size of charm-specific
	// per-unit state data that charms can store to the controller in
	// bytes. A value of 0 disables the quota checks although in
	// principle, mongo imposes a hard (but configurable) limit of 16M.
	MaxCharmStateSize = "max-charm-state-size"

	// MaxAgentStateSize is the maximum allowed size of internal state
	// data that agents can store to the controller in bytes. A value of 0
	// disables the quota checks although in principle, mongo imposes a
	// hard (but configurable) limit of 16M.
	MaxAgentStateSize = "max-agent-state-size"

	// MigrationMinionWaitMax is the maximum time that the migration-master
	// worker will wait for agents to report for a migration phase when
	// executing a model migration.
	MigrationMinionWaitMax = "migration-agent-wait-time"

	// JujuHASpace is the network space within which the MongoDB replica-set
	// should communicate.
	JujuHASpace = "juju-ha-space"

	// JujuManagementSpace is the network space that agents should use to
	// communicate with controllers.
	JujuManagementSpace = "juju-mgmt-space"

	// CAASOperatorImagePath sets the URL of the docker image
	// used for the application operator.
	// Deprecated: use CAASImageRepo
	CAASOperatorImagePath = "caas-operator-image-path"

	// CAASImageRepo sets the docker repo to use
	// for the jujud operator and mongo images.
	CAASImageRepo = "caas-image-repo"

	// Features allows a list of runtime changeable features to be updated.
	Features = "features"

	// PublicDNSAddress is the public DNS address (and port) of the controller.
	PublicDNSAddress = "public-dns-address"

	// QueryTracingEnabled returns whether query tracing is enabled. If so, any
	// queries which take longer than QueryTracingThreshold will be logged.
	QueryTracingEnabled = "query-tracing-enabled"

	// QueryTracingThreshold returns the "threshold" for query tracing. Any
	// queries which take longer than this value will be logged (if query tracing
	// is enabled). The lower the threshold, the more queries will be output. A
	// value of 0 means all queries will be output.
	QueryTracingThreshold = "query-tracing-threshold"

	// OpenTelemetryEnabled returns whether open telemetry is enabled.
	OpenTelemetryEnabled = "open-telemetry-enabled"

	// OpenTelemetryEndpoint returns the endpoint at which the telemetry will
	// be pushed to.
	OpenTelemetryEndpoint = "open-telemetry-endpoint"

	// OpenTelemetryInsecure returns if the telemetry collector endpoint is
	// insecure or not. Useful for debug or local testing.
	OpenTelemetryInsecure = "open-telemetry-insecure"

	// OpenTelemetryStackTraces return whether stack traces should be added per
	// span.
	OpenTelemetryStackTraces = "open-telemetry-stack-traces"

	// OpenTelemetrySampleRatio returns the sample ratio for open telemetry.
	OpenTelemetrySampleRatio = "open-telemetry-sample-ratio"

	// OpenTelemetryTailSamplingThreshold returns the tail sampling threshold
	// for open telemetry as a duration.
	OpenTelemetryTailSamplingThreshold = "open-telemetry-tail-sampling-threshold"

	// ObjectStoreType is the type of object store to use for storing blobs.
	// This isn't currently allowed to be changed dynamically, that will come
	// when we support multiple object store types (not including state).
	ObjectStoreType = "object-store-type"

	// ObjectStoreS3Endpoint is the endpoint to use for S3 object stores.
	ObjectStoreS3Endpoint = "object-store-s3-endpoint"

	// ObjectStoreS3StaticKey is the static key to use for S3 object stores.
	ObjectStoreS3StaticKey = "object-store-s3-static-key"

	// ObjectStoreS3StaticSecret is the static secret to use for S3 object
	// stores.
	ObjectStoreS3StaticSecret = "object-store-s3-static-secret"

	// ObjectStoreS3StaticSession is the static session token to use for S3
	// object stores.
	ObjectStoreS3StaticSession = "object-store-s3-static-session"

	// SystemSSHKeys returns the set of ssh keys that should be trusted by
	// agents of this controller regardless of the model.
	SystemSSHKeys = "system-ssh-keys"

	// JujudControllerSnapSource returns the source for the controller snap.
	// Can be set to "legacy", "snapstore", "local" or "local-dangerous".
	// Cannot be changed.
	JujudControllerSnapSource = "jujud-controller-snap-source"

	// SSHServerPort is the port used for the embedded SSH server.
	SSHServerPort = "ssh-server-port"

	// SSHMaxConcurrentConnections is the maximum number of concurrent SSH
	// connections to the controller.
	SSHMaxConcurrentConnections = "ssh-max-concurrent-connections"
)

// Attribute Defaults
const (
	// DefaultSSHMaxConcurrentConnections is the default maximum number of
	// concurrent SSH connections to the controller.
	DefaultSSHMaxConcurrentConnections = 100

	// DefaultSSHServerPort is the default port used for the embedded SSH server.
	DefaultSSHServerPort = 17022

	// DefaultApplicationResourceDownloadLimit allows unlimited
	// resource download requests initiated by a unit agent per application.
	DefaultApplicationResourceDownloadLimit = 0

	// DefaultControllerResourceDownloadLimit allows unlimited concurrent resource
	// download requests initiated by unit agents for any application on the controller.
	DefaultControllerResourceDownloadLimit = 0

	// DefaultAgentRateLimitMax allows the first 10 agents to connect without
	// any issue. After that the rate limiting kicks in.
	DefaultAgentRateLimitMax = 10

	// DefaultAgentRateLimitRate will allow four agents to connect every
	// second. A token is added to the ratelimit token bucket every 250ms.
	DefaultAgentRateLimitRate = 250 * time.Millisecond

	// DefaultAuditingEnabled contains the default value for the
	// AuditingEnabled config value.
	DefaultAuditingEnabled = true

	// DefaultAuditLogCaptureArgs is the default for the
	// AuditLogCaptureArgs setting (which is not to capture them).
	DefaultAuditLogCaptureArgs = false

	// DefaultAuditLogMaxSizeMB is the default size in MB at which we
	// roll the audit log file.
	DefaultAuditLogMaxSizeMB = 300

	// DefaultAuditLogMaxBackups is the default number of files to
	// keep.
	DefaultAuditLogMaxBackups = 10

	// DefaultNUMAControlPolicy should not be used by default.
	// Only use numactl if user specifically requests it
	DefaultNUMAControlPolicy = false

	// DefaultStatePort is the default port the controller is listening on.
	DefaultStatePort int = 37017

	// DefaultAPIPort is the default port the API server is listening on.
	DefaultAPIPort int = 17070

	// DefaultAPIPortOpenDelay is the default value for api-port-open-delay.
	DefaultAPIPortOpenDelay = 2 * time.Second

	// DefaultJujuDBSnapChannel is the default snap channel for installing
	// mongo in focal or later.
	DefaultJujuDBSnapChannel = "4.4/stable"

	// DefaultMaxDebugLogDuration is the default duration that debug-log
	// commands can run before being terminated by the API server.
	DefaultMaxDebugLogDuration = 24 * time.Hour

	// DefaultMaxTxnLogCollectionMB is the maximum size the txn log collection.
	DefaultMaxTxnLogCollectionMB = 10 // 10 MB

	// DefaultMaxPruneTxnBatchSize is the normal number of transaction
	// we will prune in a given pass (1M) (deprecated)
	DefaultMaxPruneTxnBatchSize = 1 * 1000 * 1000

	// DefaultMaxPruneTxnPasses is the default number of
	// batches we will process. (deprecated)
	DefaultMaxPruneTxnPasses = 100

	// DefaultAgentLogfileMaxSize is the maximum file size in MB of each
	// agent/controller log file.
	DefaultAgentLogfileMaxSize = 100

	// DefaultAgentLogfileMaxBackups is the number of old agent/controller log
	// files to keep (compressed).
	DefaultAgentLogfileMaxBackups = 2

	// DefaultModelLogfileMaxSize is the maximum file size in MB of
	// the log file written out by the controller on behalf of workers
	// running for a model.
	DefaultModelLogfileMaxSize = 10

	// DefaultModelLogfileMaxBackups is the number of old model
	// log files to keep (compressed).
	DefaultModelLogfileMaxBackups = 2

	// DefaultPruneTxnQueryCount is the number of transactions
	// to read in a single query.
	DefaultPruneTxnQueryCount = 1000

	// DefaultPruneTxnSleepTime is the amount of time to sleep between
	// processing each batch query. This is used to reduce load on the system,
	// allowing other queries to time to operate. On large controllers,
	// processing 1000 txs seems to take about 100ms, so a sleep time of 10ms
	// represents a 10% slowdown, but allows other systems to
	// operate concurrently.
	DefaultPruneTxnSleepTime = 10 * time.Millisecond

	// DefaultMaxCharmStateSize is the maximum size (in bytes) of charm
	// state data that each unit can store to the controller.
	DefaultMaxCharmStateSize = 2 * 1024 * 1024

	// DefaultMaxAgentStateSize is the maximum size (in bytes) of internal
	// state data that agents can store to the controller.
	DefaultMaxAgentStateSize = 512 * 1024

	// DefaultMigrationMinionWaitMax is the default value for how long a
	// migration minion will wait for the migration to complete.
	DefaultMigrationMinionWaitMax = 15 * time.Minute

	// DefaultQueryTracingEnabled is the default value for if query tracing
	// is enabled.
	DefaultQueryTracingEnabled = false

	// DefaultQueryTracingThreshold is the default value for the threshold
	// for query tracing. If a query takes longer than this to complete
	// it will be logged if query tracing is enabled.
	DefaultQueryTracingThreshold = time.Second

	// DefaultAuditLogExcludeMethods is the default list of methods to
	// exclude from the audit log.
	// This special value means we exclude any methods in the set
	// listed in apiserver/observer/auditfilter.go
	DefaultAuditLogExcludeMethods = ReadOnlyMethodsWildcard

	// DefaultOpenTelemetryEnabled is the default value for if the open
	// telemetry tracing is enabled or not.
	DefaultOpenTelemetryEnabled = false

	// DefaultOpenTelemetryInsecure is the default value for it the open
	// telemetry tracing endpoint is insecure or not.
	DefaultOpenTelemetryInsecure = false

	// DefaultOpenTelemetryStackTraces is the default value for it the open
	// telemetry tracing has stack traces or not.
	DefaultOpenTelemetryStackTraces = false

	// DefaultOpenTelemetrySampleRatio is the default value for the sample
	// ratio for open telemetry.
	// By default we only want to trace 10% of the requests.
	DefaultOpenTelemetrySampleRatio = 0.1

	// DefaultOpenTelemetryTailSamplingThreshold is the default value for the
	// tail sampling threshold for open telemetry.
	DefaultOpenTelemetryTailSamplingThreshold = 1 * time.Millisecond

	// JujudControllerSnapSource is the default value for the jujud controller
	// snap source, which is the snapstore.
	// TODO(jujud-controller-snap): change this to "snapstore" once it is implemented.
	DefaultJujudControllerSnapSource = "legacy"

	// DefaultObjectStoreType is the default type of object store to use for
	// storing blobs.
	DefaultObjectStoreType = objectstore.FileBackend
)

var (
	// ControllerOnlyConfigAttributes lists all the controller config keys, so we
	// can distinguish these from model config keys when bootstrapping.
	ControllerOnlyConfigAttributes = []string{
		AllowModelAccessKey,
		AgentRateLimitMax,
		AgentRateLimitRate,
		APIPort,
		APIPortOpenDelay,
		AutocertDNSNameKey,
		AutocertURLKey,
		CACertKey,
		ControllerAPIPort,
		ControllerName,
		ControllerUUIDKey,
		LoginTokenRefreshURL,
		IdentityPublicKey,
		IdentityURL,
		SetNUMAControlPolicyKey,
		StatePort,
		JujuDBSnapChannel,
		MaxDebugLogDuration,
		MaxTxnLogSize,
		MaxPruneTxnBatchSize,
		MaxPruneTxnPasses,
		AgentLogfileMaxBackups,
		AgentLogfileMaxSize,
		ModelLogfileMaxBackups,
		ModelLogfileMaxSize,
		PruneTxnQueryCount,
		PruneTxnSleepTime,
		PublicDNSAddress,
		JujuHASpace,
		JujuManagementSpace,
		AuditingEnabled,
		AuditLogCaptureArgs,
		AuditLogMaxSize,
		AuditLogMaxBackups,
		AuditLogExcludeMethods,
		CAASOperatorImagePath,
		CAASImageRepo,
		Features,
		MaxCharmStateSize,
		MaxAgentStateSize,
		MigrationMinionWaitMax,
		ApplicationResourceDownloadLimit,
		ControllerResourceDownloadLimit,
		QueryTracingEnabled,
		QueryTracingThreshold,
		OpenTelemetryEnabled,
		OpenTelemetryEndpoint,
		OpenTelemetryInsecure,
		OpenTelemetryStackTraces,
		OpenTelemetrySampleRatio,
		OpenTelemetryTailSamplingThreshold,
		ObjectStoreType,
		ObjectStoreS3Endpoint,
		ObjectStoreS3StaticKey,
		ObjectStoreS3StaticSecret,
		ObjectStoreS3StaticSession,
		SystemSSHKeys,
		JujudControllerSnapSource,
		SSHMaxConcurrentConnections,
		SSHServerPort,
	}

	// For backwards compatibility, we must include "anything", "juju-apiserver"
	// and "juju-mongodb" as hostnames as that is what clients specify
	// as the hostname for verification (this certificate is used both
	// for serving MongoDB and API server connections).  We also
	// explicitly include localhost.
	DefaultDNSNames = []string{
		"localhost",
		"juju-apiserver",
		"juju-mongodb",
		"anything",
	}

	// AllowedUpdateConfigAttributes contains all of the controller
	// config attributes that are allowed to be updated after the
	// controller has been created.
	AllowedUpdateConfigAttributes = set.NewStrings(
		AgentLogfileMaxBackups,
		AgentLogfileMaxSize,
		AgentRateLimitMax,
		AgentRateLimitRate,
		APIPortOpenDelay,
		ApplicationResourceDownloadLimit,
		AuditingEnabled,
		AuditLogCaptureArgs,
		AuditLogExcludeMethods,
		AuditLogMaxBackups,
		AuditLogMaxSize,
		CAASImageRepo,
		ControllerResourceDownloadLimit,
		Features,
		JujuHASpace,
		JujuManagementSpace,
		MaxAgentStateSize,
		MaxCharmStateSize,
		MaxDebugLogDuration,
		MaxPruneTxnBatchSize,
		MaxPruneTxnPasses,
		MigrationMinionWaitMax,
		ModelLogfileMaxBackups,
		ModelLogfileMaxSize,
		OpenTelemetryEnabled,
		OpenTelemetryEndpoint,
		OpenTelemetryInsecure,
		OpenTelemetryStackTraces,
		OpenTelemetrySampleRatio,
		OpenTelemetryTailSamplingThreshold,
		PruneTxnQueryCount,
		PruneTxnSleepTime,
		PublicDNSAddress,
		QueryTracingEnabled,
		QueryTracingThreshold,
		ObjectStoreType,
		ObjectStoreS3Endpoint,
		ObjectStoreS3StaticKey,
		ObjectStoreS3StaticSecret,
		ObjectStoreS3StaticSession,
		SSHMaxConcurrentConnections,
	)

	methodNameRE = regexp.MustCompile(`[[:alpha:]][[:alnum:]]*\.[[:alpha:]][[:alnum:]]*`)
)

// ControllerOnlyAttribute returns true if the specified attribute name
// is a controller config key (as opposed to, say, a model config key).
func ControllerOnlyAttribute(attr string) bool {
	for _, a := range ControllerOnlyConfigAttributes {
		if attr == a {
			return true
		}
	}
	return false
}

// Config is a string-keyed map of controller configuration attributes.
type Config map[string]interface{}

// Validate validates the controller configuration.
func (c Config) Validate() error {
	return Validate(c)
}

// NewConfig creates a new Config from the supplied attributes.
// Default values will be used where defaults are available.
//
// The controller UUID and CA certificate must be passed in.
// The UUID is typically generated by the immediate caller,
// and the CA certificate generated by environs/bootstrap.NewConfig.
func NewConfig(controllerUUID, caCert string, attrs map[string]interface{}) (Config, error) {
	// TODO(wallyworld) - use core/config when it supports duration types
	for k, v := range attrs {
		field, ok := ConfigSchema[k]
		if !ok || field.Type != configschema.Tlist {
			continue
		}
		str, ok := v.(string)
		if !ok {
			continue
		}
		var coerced interface{}
		err := yaml.Unmarshal([]byte(str), &coerced)
		if err != nil {
			return Config{}, errors.NewNotValid(err, fmt.Sprintf("value %q for attribute %q not valid", str, k))
		}
		attrs[k] = coerced
	}
	coerced, err := configChecker.Coerce(attrs, nil)
	if err != nil {
		return Config{}, errors.Trace(err)
	}
	attrs = coerced.(map[string]interface{})
	attrs[ControllerUUIDKey] = controllerUUID
	attrs[CACertKey] = caCert
	config := Config(attrs)
	return config, config.Validate()
}

// mustInt returns the named attribute as an integer, panicking if
// it is not found or is zero. Zero values should have been
// diagnosed at Validate time.
func (c Config) mustInt(name string) int {
	// Values obtained over the api are encoded as float64.
	if value, ok := c[name].(float64); ok {
		return int(value)
	}
	value, _ := c[name].(int)
	if value == 0 {
		panic(errors.Errorf("empty value for %q found in configuration", name))
	}
	return value
}

func (c Config) intOrDefault(name string, defaultVal int) int {
	if _, ok := c[name]; ok {
		return c.mustInt(name)
	}
	return defaultVal
}

func (c Config) boolOrDefault(name string, defaultVal bool) bool {
	if value, ok := c[name]; ok {
		// Value has already been validated.
		return value.(bool)
	}
	return defaultVal
}

func (c Config) sizeMBOrDefault(name string, defaultVal int) int {
	size := c.asString(name)
	if size != "" {
		// Value has already been validated.
		value, _ := utils.ParseSize(size)
		return int(value)
	}
	return defaultVal
}

// asString is a private helper method to keep the ugly string casting
// in once place. It returns the given named attribute as a string,
// returning "" if it isn't found.
func (c Config) asString(name string) string {
	value, _ := c[name].(string)
	return value
}

// mustString returns the named attribute as an string, panicking if
// it is not found or is empty.
func (c Config) mustString(name string) string {
	value, _ := c[name].(string)
	if value == "" {
		panic(errors.Errorf("empty value for %q found in configuration (type %T, val %v)", name, c[name], c[name]))
	}
	return value
}

func (c Config) durationOrDefault(name string, defaultVal time.Duration) time.Duration {
	switch v := c[name].(type) {
	case string:
		if v != "" {
			// Value has already been validated.
			value, _ := time.ParseDuration(v)
			return value
		}
	case time.Duration:
		return v
	default:
		// nil type shows up here
	}
	return defaultVal
}

// StatePort returns the mongo server port for the environment.
func (c Config) StatePort() int {
	return c.mustInt(StatePort)
}

// APIPort returns the API server port for the environment.
func (c Config) APIPort() int {
	return c.mustInt(APIPort)
}

// APIPortOpenDelay returns the duration to wait before opening
// the APIPort once the controller has started up. Only used when
// the ControllerAPIPort is non-zero.
func (c Config) APIPortOpenDelay() time.Duration {
	return c.durationOrDefault(APIPortOpenDelay, DefaultAPIPortOpenDelay)
}

// ControllerAPIPort returns the optional API port to be used for
// the controllers to talk to each other. A zero value means that
// it is not set.
func (c Config) ControllerAPIPort() int {
	if value, ok := c[ControllerAPIPort].(float64); ok {
		return int(value)
	}
	// If the value isn't an int, this conversion will fail and value
	// will be 0, which is what we want here.
	value, _ := c[ControllerAPIPort].(int)
	return value
}

// ApplicationResourceDownloadLimit limits the number of concurrent resource download
// requests from unit agents which will be served. The limit is per application.
func (c Config) ApplicationResourceDownloadLimit() int {
	switch v := c[ApplicationResourceDownloadLimit].(type) {
	case float64:
		return int(v)
	case int:
		return v
	default:
		// nil type shows up here
	}
	return DefaultApplicationResourceDownloadLimit
}

// ControllerResourceDownloadLimit limits the number of concurrent resource download
// requests from unit agents which will be served. The limit is for the combined total
// of all applications on the controller.
func (c Config) ControllerResourceDownloadLimit() int {
	switch v := c[ControllerResourceDownloadLimit].(type) {
	case float64:
		return int(v)
	case int:
		return v
	default:
		// nil type shows up here
	}
	return DefaultControllerResourceDownloadLimit
}

// AgentRateLimitMax is the initial size of the token bucket that is used to
// rate limit agent connections.
func (c Config) AgentRateLimitMax() int {
	switch v := c[AgentRateLimitMax].(type) {
	case float64:
		return int(v)
	case int:
		return v
	default:
		// nil type shows up here
	}
	return DefaultAgentRateLimitMax
}

// AgentRateLimitRate is the time taken to add a token into the token bucket
// that is used to rate limit agent connections.
func (c Config) AgentRateLimitRate() time.Duration {
	return c.durationOrDefault(AgentRateLimitRate, DefaultAgentRateLimitRate)
}

// AuditingEnabled returns whether or not auditing has been enabled
// for the environment. The default is false.
func (c Config) AuditingEnabled() bool {
	if v, ok := c[AuditingEnabled]; ok {
		return v.(bool)
	}
	return DefaultAuditingEnabled
}

// AuditLogCaptureArgs returns whether audit logging should capture
// the arguments to API methods. The default is false.
func (c Config) AuditLogCaptureArgs() bool {
	if v, ok := c[AuditLogCaptureArgs]; ok {
		return v.(bool)
	}
	return DefaultAuditLogCaptureArgs
}

// AuditLogMaxSizeMB returns the maximum size for an audit log file in
// MB.
func (c Config) AuditLogMaxSizeMB() int {
	return c.sizeMBOrDefault(AuditLogMaxSize, DefaultAuditLogMaxSizeMB)
}

// AuditLogMaxBackups returns the maximum number of backup audit log
// files to keep.
func (c Config) AuditLogMaxBackups() int {
	return c.intOrDefault(AuditLogMaxBackups, DefaultAuditLogMaxBackups)
}

// AuditLogExcludeMethods returns the set of method names that are
// considered uninteresting for audit logging. Conversations
// containing only these will be excluded from the audit log.
func (c Config) AuditLogExcludeMethods() set.Strings {
	v := c.asString(AuditLogExcludeMethods)
	if v == "" {
		return set.NewStrings()
	}
	return set.NewStrings(strings.Split(v, ",")...)
}

// Features returns the controller config set features flags.
func (c Config) Features() set.Strings {
	v := c.asString(Features)
	if v == "" {
		return set.NewStrings()
	}
	return set.NewStrings(strings.Split(v, ",")...)
}

// ControllerName returns the name for the controller
func (c Config) ControllerName() string {
	return c.asString(ControllerName)
}

// ControllerUUID returns the uuid for the controller.
func (c Config) ControllerUUID() string {
	return c.mustString(ControllerUUIDKey)
}

// CACert returns the certificate of the CA that signed the controller
// certificate, in PEM format, and whether the setting is available.
//
// TODO(axw) once the controller config is completely constructed,
// there will always be a CA certificate. Get rid of the bool result.
func (c Config) CACert() (string, bool) {
	if s, ok := c[CACertKey]; ok {
		return s.(string), true
	}
	return "", false
}

// IdentityURL returns the URL of the identity manager.
func (c Config) IdentityURL() string {
	return c.asString(IdentityURL)
}

// AutocertURL returns the URL used to obtain official TLS certificates
// when a client connects to the API. See AutocertURLKey
// for more details.
func (c Config) AutocertURL() string {
	return c.asString(AutocertURLKey)
}

// AutocertDNSName returns the DNS name of the controller.
// See AutocertDNSNameKey for more details.
func (c Config) AutocertDNSName() string {
	return c.asString(AutocertDNSNameKey)
}

// IdentityPublicKey returns the public key of the identity manager.
func (c Config) IdentityPublicKey() *bakery.PublicKey {
	key := c.asString(IdentityPublicKey)
	if key == "" {
		return nil
	}
	var pubKey bakery.PublicKey
	err := pubKey.UnmarshalText([]byte(key))
	if err != nil {
		// We check if the key string can be unmarshalled into a PublicKey in the
		// Validate function, so we really do not expect this to fail.
		panic(err)
	}
	return &pubKey
}

// LoginTokenRefreshURL returns the URL of the login jwt well known endpoint.
func (c Config) LoginTokenRefreshURL() string {
	return c.asString(LoginTokenRefreshURL)
}

// JujuDBSnapChannel returns the channel for installing mongo snaps.
func (c Config) JujuDBSnapChannel() string {
	return c.asString(JujuDBSnapChannel)
}

// JujudControllerSnapSource returns the source of the jujud-controller snap.
func (c Config) JujudControllerSnapSource() string {
	if src, ok := c[JujudControllerSnapSource]; ok {
		return src.(string)
	}
	return DefaultJujudControllerSnapSource
}

// NUMACtlPreference returns if numactl is preferred.
func (c Config) NUMACtlPreference() bool {
	if numa, ok := c[SetNUMAControlPolicyKey]; ok {
		return numa.(bool)
	}
	return DefaultNUMAControlPolicy
}

// AllowModelAccess reports whether users are allowed to access models
// they have been granted permission for even when they can't access
// the controller.
func (c Config) AllowModelAccess() bool {
	value, _ := c[AllowModelAccessKey].(bool)
	return value
}

// AgentLogfileMaxSizeMB is the maximum file size in MB of each
// agent/controller log file.
func (c Config) AgentLogfileMaxSizeMB() int {
	return c.sizeMBOrDefault(AgentLogfileMaxSize, DefaultAgentLogfileMaxSize)
}

// AgentLogfileMaxBackups is the number of old agent/controller log files to
// keep (compressed).
func (c Config) AgentLogfileMaxBackups() int {
	return c.intOrDefault(AgentLogfileMaxBackups, DefaultAgentLogfileMaxBackups)
}

// ModelLogfileMaxBackups is the number of old model log files to keep (compressed).
func (c Config) ModelLogfileMaxBackups() int {
	return c.intOrDefault(ModelLogfileMaxBackups, DefaultModelLogfileMaxBackups)
}

// ModelLogfileMaxSizeMB is the maximum size of the log file written out by the
// controller on behalf of workers running for a model.
func (c Config) ModelLogfileMaxSizeMB() int {
	return c.sizeMBOrDefault(ModelLogfileMaxSize, DefaultModelLogfileMaxSize)
}

// MaxDebugLogDuration is the maximum time a debug-log session is allowed
// to run before it is terminated by the server.
func (c Config) MaxDebugLogDuration() time.Duration {
	return c.durationOrDefault(MaxDebugLogDuration, DefaultMaxDebugLogDuration)
}

// MaxTxnLogSizeMB is the maximum size in MiB of the txn log collection.
func (c Config) MaxTxnLogSizeMB() int {
	return c.sizeMBOrDefault(MaxTxnLogSize, DefaultMaxTxnLogCollectionMB)
}

// MaxPruneTxnBatchSize is the maximum size of the txn log collection.
func (c Config) MaxPruneTxnBatchSize() int {
	return c.intOrDefault(MaxPruneTxnBatchSize, DefaultMaxPruneTxnBatchSize)
}

// MaxPruneTxnPasses is the maximum number of batches of the txn log collection we will process at a time.
func (c Config) MaxPruneTxnPasses() int {
	return c.intOrDefault(MaxPruneTxnPasses, DefaultMaxPruneTxnPasses)
}

// PruneTxnQueryCount is the size of small batches for pruning
func (c Config) PruneTxnQueryCount() int {
	return c.intOrDefault(PruneTxnQueryCount, DefaultPruneTxnQueryCount)
}

// PruneTxnSleepTime is the amount of time to sleep between batches.
func (c Config) PruneTxnSleepTime() time.Duration {
	return c.durationOrDefault(PruneTxnSleepTime, DefaultPruneTxnSleepTime)
}

// PublicDNSAddress returns the DNS name of the controller.
func (c Config) PublicDNSAddress() string {
	return c.asString(PublicDNSAddress)
}

// JujuHASpace is the network space within which the MongoDB replica-set
// should communicate.
func (c Config) JujuHASpace() string {
	return c.asString(JujuHASpace)
}

// SystemSSHKeys returns the trusted ssh keys that agents of this controller
// should trust.
func (c Config) SystemSSHKeys() string {
	return c.asString(SystemSSHKeys)
}

// JujuManagementSpace is the network space that agents should use to
// communicate with controllers.
func (c Config) JujuManagementSpace() string {
	return c.asString(JujuManagementSpace)
}

// CAASOperatorImagePath sets the URL of the docker image
// used for the application operator.
// Deprecated: use CAASImageRepo
func (c Config) CAASOperatorImagePath() string {
	return c.asString(CAASOperatorImagePath)
}

// CAASImageRepo sets the URL of the docker repo
// used for the jujud operator and mongo images.
func (c Config) CAASImageRepo() string {
	return c.asString(CAASImageRepo)
}

// MaxCharmStateSize returns the max size (in bytes) of charm-specific state
// that each unit can store to the controller. A value of zero indicates no
// limit.
func (c Config) MaxCharmStateSize() int {
	return c.intOrDefault(MaxCharmStateSize, DefaultMaxCharmStateSize)
}

// MaxAgentStateSize returns the max size (in bytes) of state data that agents
// can store to the controller. A value of zero indicates no limit.
func (c Config) MaxAgentStateSize() int {
	return c.intOrDefault(MaxAgentStateSize, DefaultMaxAgentStateSize)
}

// MigrationMinionWaitMax returns a duration for the maximum time that the
// migration-master worker should wait for migration-minion reports during
// phases of a model migration.
func (c Config) MigrationMinionWaitMax() time.Duration {
	return c.durationOrDefault(MigrationMinionWaitMax, DefaultMigrationMinionWaitMax)
}

// QueryTracingEnabled returns whether query tracing is enabled.
func (c Config) QueryTracingEnabled() bool {
	return c.boolOrDefault(QueryTracingEnabled, DefaultQueryTracingEnabled)
}

// QueryTracingThreshold returns the threshold for query tracing. The
// lower the threshold, the more queries will be output. A value of 0
// means all queries will be output.
func (c Config) QueryTracingThreshold() time.Duration {
	return c.durationOrDefault(QueryTracingThreshold, DefaultQueryTracingThreshold)
}

// OpenTelemetryEnabled returns whether open telemetry tracing is enabled.
func (c Config) OpenTelemetryEnabled() bool {
	return c.boolOrDefault(OpenTelemetryEnabled, DefaultOpenTelemetryEnabled)
}

// OpenTelemetryEndpoint returns the open telemetry endpoint.
func (c Config) OpenTelemetryEndpoint() string {
	return c.asString(OpenTelemetryEndpoint)
}

// OpenTelemetryInsecure returns whether open telemetry tracing endpoint is
// insecure or not.
func (c Config) OpenTelemetryInsecure() bool {
	return c.boolOrDefault(OpenTelemetryInsecure, DefaultOpenTelemetryInsecure)
}

// OpenTelemetryStackTraces returns whether open telemetry tracing spans
// requires to have stack traces.
func (c Config) OpenTelemetryStackTraces() bool {
	return c.boolOrDefault(OpenTelemetryStackTraces, DefaultOpenTelemetryStackTraces)
}

// OpenTelemetrySampleRatio returns whether open telemetry tracing spans
// requires to have stack traces.
func (c Config) OpenTelemetrySampleRatio() float64 {
	f, err := parseRatio(c, OpenTelemetrySampleRatio)
	if err == nil {
		return f
	}
	return DefaultOpenTelemetrySampleRatio
}

// OpenTelemetryTailSamplingThreshold returns the tail sampling threshold
// for open telemetry tracing spans.
func (c Config) OpenTelemetryTailSamplingThreshold() time.Duration {
	return c.durationOrDefault(OpenTelemetryTailSamplingThreshold, DefaultOpenTelemetryTailSamplingThreshold)
}

// ObjectStoreType returns the type of object store to use for storing blobs.
func (c Config) ObjectStoreType() objectstore.BackendType {
	return objectstore.BackendType(c.asString(ObjectStoreType))
}

// ObjectStoreS3Endpoint returns the endpoint to use for S3 object stores.
func (c Config) ObjectStoreS3Endpoint() string {
	return c.asString(ObjectStoreS3Endpoint)
}

// ObjectStoreS3StaticKey returns the static key to use for S3 object stores.
func (c Config) ObjectStoreS3StaticKey() string {
	return c.asString(ObjectStoreS3StaticKey)
}

// ObjectStoreS3StaticSecret returns the static secret to use for S3 object
// stores.
func (c Config) ObjectStoreS3StaticSecret() string {
	return c.asString(ObjectStoreS3StaticSecret)
}

// ObjectStoreS3StaticSession returns the static session token to use for S3
// object stores.
func (c Config) ObjectStoreS3StaticSession() string {
	return c.asString(ObjectStoreS3StaticSession)
}

// SSHServerPort returns the port the SSH server listens on.
func (c Config) SSHServerPort() int {
	return c.intOrDefault(SSHServerPort, DefaultSSHServerPort)
}

// SSHMaxConcurrentConnections returns the maximum number of concurrent
// SSH connections that the controller will allow.
func (c Config) SSHMaxConcurrentConnections() int {
	return c.intOrDefault(SSHMaxConcurrentConnections, DefaultSSHMaxConcurrentConnections)
}

// Validate ensures that config is a valid configuration.
func Validate(c Config) error {
	if v, ok := c[IdentityPublicKey].(string); ok {
		var key bakery.PublicKey
		if err := key.UnmarshalText([]byte(v)); err != nil {
			return errors.Annotate(err, "invalid identity public key")
		}
	}

	if v, ok := c[IdentityURL].(string); ok {
		u, err := url.Parse(v)
		if err != nil {
			return errors.Annotate(err, "invalid identity URL")
		}
		// If we've got an identity public key, we allow an HTTP
		// scheme for the identity server because we won't need
		// to rely on insecure transport to obtain the public
		// key.
		if _, ok := c[IdentityPublicKey]; !ok && u.Scheme != "https" {
			return errors.Errorf("URL needs to be https when %s not provided", IdentityPublicKey)
		}
	}

	if v, ok := c[LoginTokenRefreshURL].(string); ok {
		u, err := url.Parse(v)
		if err != nil {
			return errors.Annotate(err, "invalid login token refresh URL")
		}
		if u.Scheme == "" || u.Host == "" {
			return errors.NotValidf("logic token refresh URL %q", v)
		}
	}

	caCert, caCertOK := c.CACert()
	if !caCertOK {
		return errors.Errorf("missing CA certificate")
	}
	if ok, err := pki.IsPemCA([]byte(caCert)); err != nil {
		return errors.Annotate(err, "bad CA certificate in configuration")
	} else if !ok {
		return errors.New("ca certificate in configuration is not a CA")
	}

	if uuid, ok := c[ControllerUUIDKey].(string); ok && !utils.IsValidUUIDString(uuid) {
		return errors.Errorf("controller-uuid: expected UUID, got string(%q)", uuid)
	}

	if v, ok := c[ApplicationResourceDownloadLimit].(int); ok {
		if v < 0 {
			return errors.Errorf("negative %s (%d) not valid, use 0 to disable the limit", ApplicationResourceDownloadLimit, v)
		}
	}
	if v, ok := c[ControllerResourceDownloadLimit].(int); ok {
		if v < 0 {
			return errors.Errorf("negative %s (%d) not valid, use 0 to disable the limit", ControllerResourceDownloadLimit, v)
		}
	}
	if v, ok := c[AgentRateLimitMax].(int); ok {
		if v < 0 {
			return errors.NotValidf("negative %s (%d)", AgentRateLimitMax, v)
		}
	}

	if v, err := parseDuration(c, AgentRateLimitRate); err != nil && !errors.Is(err, errors.NotFound) {
		return errors.Trace(err)
	} else if err == nil {
		if v == 0 {
			return errors.Errorf("%s cannot be zero", AgentRateLimitRate)
		}
		if v < 0 {
			return errors.Errorf("%s cannot be negative", AgentRateLimitRate)
		}
		if v > time.Minute {
			return errors.Errorf("%s must be between 0..1m", AgentRateLimitRate)
		}
	}

	if v, err := parseDuration(c, MaxDebugLogDuration); err != nil && !errors.Is(err, errors.NotFound) {
		return errors.Trace(err)
	} else if err == nil {
		if v == 0 {
			return errors.Errorf("%s cannot be zero", MaxDebugLogDuration)
		}
	}

	if v, ok := c[AgentLogfileMaxBackups].(int); ok {
		if v < 0 {
			return errors.NotValidf("negative %s", AgentLogfileMaxBackups)
		}
	}
	if v, ok := c[AgentLogfileMaxSize].(string); ok {
		mb, err := utils.ParseSize(v)
		if err != nil {
			return errors.Annotatef(err, "invalid %s in configuration", AgentLogfileMaxSize)
		}
		if mb < 1 {
			return errors.NotValidf("%s less than 1 MB", AgentLogfileMaxSize)
		}
	}

	if v, ok := c[ModelLogfileMaxBackups].(int); ok {
		if v < 0 {
			return errors.NotValidf("negative %s", ModelLogfileMaxBackups)
		}
	}
	if v, ok := c[ModelLogfileMaxSize].(string); ok {
		mb, err := utils.ParseSize(v)
		if err != nil {
			return errors.Annotatef(err, "invalid %s in configuration", ModelLogfileMaxSize)
		}
		if mb < 1 {
			return errors.NotValidf("%s less than 1 MB", ModelLogfileMaxSize)
		}
	}

	if v, ok := c[MaxTxnLogSize].(string); ok {
		if _, err := utils.ParseSize(v); err != nil {
			return errors.Annotate(err, "invalid max txn log size in configuration")
		}
	}

	if v, ok := c[PruneTxnSleepTime].(string); ok {
		if _, err := time.ParseDuration(v); err != nil {
			return errors.Annotatef(err, `%s must be a valid duration (eg "10ms")`, PruneTxnSleepTime)
		}
	}

	if err := c.validateSpaceConfig(JujuHASpace, "juju HA"); err != nil {
		return errors.Trace(err)
	}

	if err := c.validateSpaceConfig(JujuManagementSpace, "juju mgmt"); err != nil {
		return errors.Trace(err)
	}

	var auditLogMaxSize int
	if v, ok := c[AuditLogMaxSize].(string); ok {
		if size, err := utils.ParseSize(v); err != nil {
			return errors.Annotate(err, "invalid audit log max size in configuration")
		} else {
			auditLogMaxSize = int(size)
		}
	}

	if v, ok := c[AuditingEnabled].(bool); ok {
		if v && auditLogMaxSize == 0 {
			return errors.Errorf("invalid audit log max size: can't be 0 if auditing is enabled")
		}
	}

	if v, ok := c[AuditLogMaxBackups].(int); ok {
		if v < 0 {
			return errors.Errorf("invalid audit log max backups: should be a number of files (or 0 to keep all), got %d", v)
		}
	}

	if v, ok := c[AuditLogExcludeMethods].(string); ok {
		if v != "" {
			for i, name := range strings.Split(v, ",") {
				if name != ReadOnlyMethodsWildcard && !methodNameRE.MatchString(name) {
					return errors.Errorf(
						`invalid audit log exclude methods: should be a list of "Facade.Method" names (or "ReadOnlyMethods"), got %q at position %d`,
						name,
						i+1,
					)
				}
			}
		}
	}

	if v, ok := c[ControllerAPIPort].(int); ok {
		// TODO: change the validation so 0 is invalid and --reset is used.
		// However that doesn't exist yet.
		if v < 0 {
			return errors.NotValidf("non-positive integer for controller-api-port")
		}
		if v == c.APIPort() {
			return errors.NotValidf("controller-api-port matching api-port")
		}
		if v == c.StatePort() {
			return errors.NotValidf("controller-api-port matching state-port")
		}
	}

	if v, ok := c[ControllerName].(string); ok {
		if !names.IsValidControllerName(v) {
			return errors.Errorf("%s value must be a valid controller name (lowercase or digit with non-leading hyphen), got %q", ControllerName, v)
		}
	}

	if v, ok := c[APIPortOpenDelay].(string); ok {
		_, err := time.ParseDuration(v)
		if err != nil {
			return errors.Errorf("%s value %q must be a valid duration", APIPortOpenDelay, v)
		}
	}

	// Each unit stores the charm and uniter state in a single document.
	// Given that mongo by default enforces a 16M limit for documents we
	// should also verify that the combined limits don't exceed 16M.
	var maxUnitStateSize int
	if v, ok := c[MaxCharmStateSize].(int); ok {
		if v < 0 {
			return errors.Errorf("invalid max charm state size: should be a number of bytes (or 0 to disable limit), got %d", v)
		}
		maxUnitStateSize += v
	} else {
		maxUnitStateSize += DefaultMaxCharmStateSize
	}

	if v, ok := c[MaxAgentStateSize].(int); ok {
		if v < 0 {
			return errors.Errorf("invalid max agent state size: should be a number of bytes (or 0 to disable limit), got %d", v)
		}
		maxUnitStateSize += v
	} else {
		maxUnitStateSize += DefaultMaxAgentStateSize
	}

	if mongoMax := 16 * 1024 * 1024; maxUnitStateSize > mongoMax {
		return errors.Errorf("invalid max charm/agent state sizes: combined value should not exceed mongo's 16M per-document limit, got %d", maxUnitStateSize)
	}

	if v, ok := c[MigrationMinionWaitMax].(string); ok {
		_, err := time.ParseDuration(v)
		if err != nil {
			return errors.Errorf("%s value %q must be a valid duration", MigrationMinionWaitMax, v)
		}
	}

	if v, err := parseDuration(c, QueryTracingThreshold); err != nil && !errors.Is(err, errors.NotFound) {
		return errors.Trace(err)
	} else if err == nil {
		if v < 0 {
			return errors.Errorf("%s value %q must be a positive duration", QueryTracingThreshold, v)
		}
	}

	if v, err := parseRatio(c, OpenTelemetrySampleRatio); err != nil && !errors.Is(err, errors.NotFound) {
		return errors.Annotatef(err, "%s", OpenTelemetrySampleRatio)
	} else if err == nil {
		if v < 0 || v > 1 {
			return errors.Errorf("%s value %f must be a ratio between 0 and 1", OpenTelemetrySampleRatio, v)
		}
	}

	if v, err := parseDuration(c, OpenTelemetryTailSamplingThreshold); err != nil && !errors.Is(err, errors.NotFound) {
		return errors.Trace(err)
	} else if err == nil {
		if v < 0 {
			return errors.Errorf("%s value %q must be a positive duration", OpenTelemetryTailSamplingThreshold, v)
		}
	}

	if v, ok := c[ObjectStoreType].(string); ok {
		if v == "" {
			return errors.NotValidf("empty object store type")
		}
		if _, err := objectstore.ParseObjectStoreType(v); err != nil {
			return errors.NotValidf("invalid object store type %q", v)
		}
	}

	if v, ok := c[JujudControllerSnapSource].(string); ok {
		switch v {
		case "legacy": // TODO(jujud-controller-snap): remove once jujud-controller snap is fully implemented.
		case "snapstore", "local", "local-dangerous":
		default:
			return errors.Errorf("%s value %q must be one of legacy, snapstore, local or local-dangerous.", JujudControllerSnapSource, v)
		}
	}

	if v, ok := c[SSHServerPort].(int); ok {
		if v <= 0 {
			return errors.NotValidf("non-positive integer for ssh-server-port")
		}
		if v == c.APIPort() {
			return errors.NotValidf("ssh-server-port matching api-port")
		}
		if v == c.StatePort() {
			return errors.NotValidf("ssh-server-port matching state-port")
		}
		if v == c.ControllerAPIPort() {
			return errors.NotValidf("ssh-server-port matching controller-api-port")
		}
	}

	if v, ok := c[SSHMaxConcurrentConnections].(int); ok {
		if v <= 0 {
			return errors.NotValidf("non-positive integer for ssh-max-concurrent-connections")
		}
	}

	return nil
}

func (c Config) validateSpaceConfig(key, topic string) error {
	val := c[key]
	if val == nil {
		return nil
	}
	if v, ok := val.(string); ok {
		// NOTE(nvinuesa): We also check for the case where the passed
		// value is the empty string, this is needed to un-set the
		// controller config value and not added in the regexp to
		// validate the space.
		if !names.IsValidSpace(v) && v != "" {
			return errors.NotValidf("%s space name %q", topic, val)
		}
	} else {
		return errors.NotValidf("type for %s space name %v", topic, val)
	}

	return nil
}

// AsSpaceConstraints checks to see whether config has spaces names populated
// for management and/or HA (Mongo).
// Non-empty values are merged with any input spaces and returned as a new
// slice reference.
// A slice pointer is used for congruence with the Spaces member in
// constraints.Value.
func (c Config) AsSpaceConstraints(spaces *[]string) *[]string {
	newSpaces := set.NewStrings()
	if spaces != nil {
		for _, s := range *spaces {
			newSpaces.Add(s)
		}
	}

	for _, c := range []string{c.JujuManagementSpace(), c.JujuHASpace()} {
		// NOTE (hml) 2019-10-30
		// This can cause issues in deployment and/or enabling HA if
		// c == AlphaSpaceName as the provisioner expects any space
		// listed to have subnets. Which is only AWS today.
		if c != "" {
			newSpaces.Add(c)
		}
	}

	// Preserve a nil pointer if there is no change. This conveys information
	// in constraints.Value (not set vs. deliberately set as empty).
	if spaces == nil && len(newSpaces) == 0 {
		return nil
	}
	ns := newSpaces.SortedValues()
	return &ns
}

func parseDuration(c Config, name string) (time.Duration, error) {
	if _, ok := c[name]; !ok {
		return 0, errors.NotFoundf("config key %q", name)
	}

	switch t := c[name].(type) {
	case string:
		value, err := time.ParseDuration(t)
		return value, err
	case time.Duration:
		return t, nil
	case nil:
		return 0, nil
	default:
		return 0, errors.Errorf("unexpected type %T", c[name])
	}
}

func parseRatio(c Config, name string) (float64, error) {
	if _, ok := c[name]; !ok {
		return 0, errors.NotFoundf("config key %q", name)
	}

	switch t := c[name].(type) {
	case float64:
		return t, nil
	case float32:
		return float64(t), nil
	case string:
		value, err := strconv.ParseFloat(t, 64)
		return value, err
	case nil:
		return 0, nil
	default:
		return 0, errors.Errorf("unexpected type %T", c[name])
	}
}

// HasCompleteS3ControllerConfig returns true if the controller has a complete
// S3 configuration. This includes an endpoint, static key, and static secret.
func HasCompleteS3ControllerConfig(cfg Config) error {
	endpoint := cfg.ObjectStoreS3Endpoint()
	staticKey := cfg.ObjectStoreS3StaticKey()
	staticSecret := cfg.ObjectStoreS3StaticSecret()
	return HasCompleteS3Config(endpoint, staticKey, staticSecret)
}

// HasCompleteS3Config returns true if the S3 configuration is complete.
func HasCompleteS3Config(endpoint, staticKey, staticSecret string) error {
	if endpoint == "" {
		return errors.New("missing S3 endpoint")
	}
	if staticKey == "" {
		return errors.New("missing S3 static key")
	}
	if staticSecret == "" {
		return errors.New("missing S3 static secret")
	}
	return nil
}

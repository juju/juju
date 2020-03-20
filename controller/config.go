// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"fmt"
	"net/url"
	"regexp"
	"time"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/romulus"
	"github.com/juju/schema"
	"github.com/juju/utils"
	"gopkg.in/juju/charmrepo.v4/csclient"
	"gopkg.in/juju/environschema.v1"
	"gopkg.in/juju/names.v3"
	"gopkg.in/macaroon-bakery.v2/bakery"

	"github.com/juju/juju/core/resources"
	"github.com/juju/juju/pki"
)

const (
	// MongoProfLow represents the most conservative mongo memory profile.
	MongoProfLow = "low"
	// MongoProfDefault represents the mongo memory profile shipped by default.
	MongoProfDefault = "default"
)

const (
	// APIPort is the port used for api connections.
	APIPort = "api-port"

	// ControllerAPIPort is an optional port that may be set for controllers
	// that have a very heavy load. If this port is set, this port is used by
	// the controllers to talk to each other - used for the local API connection
	// as well as the pubsub forwarders, and the raft workers. If this value is
	// set, the api-port isn't opened until the controllers have started
	// properly.
	ControllerAPIPort = "controller-api-port"

	// Canonical name for the controller
	ControllerName = "controller-name"

	// AgentRateLimitMax is the maximum size of the token bucket used to
	// ratelimit the agent connections.
	AgentRateLimitMax = "agent-ratelimit-max"

	// AgentRateLimitRate is the time taken to add a new token to the bucket.
	// This effectively says that we can have a new agent connect per duration specified.
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

	// CharmStoreURL is the key for the url to use for charmstore API calls
	CharmStoreURL = "charmstore-url"

	// ControllerUUIDKey is the key for the controller UUID attribute.
	ControllerUUIDKey = "controller-uuid"

	// IdentityURL sets the url of the identity manager.
	IdentityURL = "identity-url"

	// IdentityPublicKey sets the public key of the identity manager.
	IdentityPublicKey = "identity-public-key"

	// SetNUMAControlPolicyKey stores the value for this setting
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
	// connect to models they have been authorized for even when
	// they don't have any access rights to the controller itself.
	AllowModelAccessKey = "allow-model-access"

	// MongoMemoryProfile sets whether mongo uses the least possible memory or the
	// detault
	MongoMemoryProfile = "mongo-memory-profile"

	// MaxDebugLogDuration is used to provide a backstop to the execution of a debug-log
	// command. If someone starts a debug-log session in a remote screen for example, it
	// is very easy to disconnect from the screen while leaving the debug-log process
	// running. This causes unnecessary load on the API Server. The max debug-log duration
	// has a default of 24 hours, which should be more than enough time for a debugging
	// session. If the user needs more information, perhaps debug-log isn't the right source.
	MaxDebugLogDuration = "max-debug-log-duration"

	// ModelLogfileMaxSize is the maximum size of the log file written out by the
	// controller on behalf of workers running for a model.
	ModelLogfileMaxSize = "model-logfile-max-size"

	// ModelLogfileMaxBackups is the number of old model log files to keep (compressed).
	ModelLogfileMaxBackups = "model-logfile-max-backups"

	// ModelLogsSize is the size of the capped collections used to hold the
	// logs for the models, eg "20M". Size is per model.
	ModelLogsSize = "model-logs-size"

	// MaxTxnLogSize is the maximum size the of capped txn log collection, eg "10M"
	MaxTxnLogSize = "max-txn-log-size"

	// MaxPruneTxnBatchSize (deprecated) is the maximum number of transactions
	// we will evaluate in one go when pruning. Default is 1M transactions.
	// A value <= 0 indicates to do all transactions at once.
	MaxPruneTxnBatchSize = "max-prune-txn-batch-size"

	// MaxPruneTxnPasses (deprecated) is the maximum number of batches that we will process.
	// So total number of transactions that can be processed is MaxPruneTxnBatchSize * MaxPruneTxnPasses.
	// A value <= 0 implies 'do a single pass'. If both MaxPruneTxnBatchSize and MaxPruneTxnPasses are 0, then the
	// default value of 1M BatchSize and 100 passes will be used instead.
	MaxPruneTxnPasses = "max-prune-txn-passes"

	// PruneTxnQueryCount is the number of transactions to read in a single query.
	// Minimum of 10, a value of 0 will indicate to use the default value (1000)
	PruneTxnQueryCount = "prune-txn-query-count"

	// PruneTxnSleepTime is the amount of time to sleep between processing each
	// batch query. This is used to reduce load on the system, allowing other queries
	// to time to operate. On large controllers, processing 1000 txs seems to take
	// about 100ms, so a sleep time of 10ms represents a 10% slowdown, but allows
	// other systems to operate concurrently.
	// A negative number will indicate to use the default, a value of 0 indicates
	// to not sleep at all.
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

	// Attribute Defaults

	// DefaultAgentRateLimitMax allows the first 10 agents to connect without any
	// issue. After that the rate limiting kicks in.
	DefaultAgentRateLimitMax = 10

	// DefaultAgentRateLimitRate will allow four agents to connect every second.
	// A token is added to the ratelimit token bucket every 250ms.
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
	// It is a string representation of a time.Duration.
	DefaultAPIPortOpenDelay = "2s"

	// DefaultMongoMemoryProfile is the default profile used by mongo.
	DefaultMongoMemoryProfile = MongoProfDefault

	// DefaultMaxDebugLogDuration is the default duration that debug-log commands
	// can run before being terminated by the API server.
	DefaultMaxDebugLogDuration = 24 * time.Hour

	// DefaultMaxTxnLogCollectionMB is the maximum size the txn log collection.
	DefaultMaxTxnLogCollectionMB = 10 // 10 MB

	// DefaultMaxPruneTxnBatchSize is the normal number of transaction we will prune in a given pass (1M) (deprecated)
	DefaultMaxPruneTxnBatchSize = 1 * 1000 * 1000

	// DefaultMaxPruneTxnPasses is the default number of batches we will process (deprecated)
	DefaultMaxPruneTxnPasses = 100

	// DefaultModelLogfileMaxSize is the maximum file size in MB of the log file written out by the
	// controller on behalf of workers running for a model.
	DefaultModelLogfileMaxSize = 10

	// DefaultModelLogfileMaxBackups is the number of old model log files to keep (compressed).
	DefaultModelLogfileMaxBackups = 2

	// DefaultModelLogsSizeMB is the size in MB of the capped logs collection
	// for each model.
	DefaultModelLogsSizeMB = 20

	// DefaultPruneTxnQueryCount is the number of transactions to read in a single query.
	DefaultPruneTxnQueryCount = 1000

	// DefaultPruneTxnSleepTime is the amount of time to sleep between processing each
	// batch query. This is used to reduce load on the system, allowing other queries
	// to time to operate. On large controllers, processing 1000 txs seems to take
	// about 100ms, so a sleep time of 10ms represents a 10% slowdown, but allows
	// other systems to operate concurrently.
	DefaultPruneTxnSleepTime = "10ms"

	// DefaultMaxCharmStateSize is the maximum size (in bytes) of charm
	// state data that each unit can store to the controller.
	DefaultMaxCharmStateSize = 2 * 1024 * 1024

	// DefaultMaxAgentStateSize is the maximum size (in bytes) of internal
	// state data that agents can store to the controller.
	DefaultMaxAgentStateSize = 512 * 1024

	// JujuHASpace is the network space within which the MongoDB replica-set
	// should communicate.
	JujuHASpace = "juju-ha-space"

	// JujuManagementSpace is the network space that agents should use to
	// communicate with controllers.
	JujuManagementSpace = "juju-mgmt-space"

	// CAASOperatorImagePath sets the url of the docker image
	// used for the application operator.
	// Deprecated: use CAASImageRepo
	CAASOperatorImagePath = "caas-operator-image-path"

	// CAASImageRepo sets the docker repo to use
	// for the jujud operator and mongo images.
	CAASImageRepo = "caas-image-repo"

	// Features allows a list of runtime changeable features to be updated.
	Features = "features"

	// MeteringURL is the key for the url to use for metrics
	MeteringURL = "metering-url"
)

var (
	// ControllerOnlyConfigAttributes are attributes which are only relevant
	// for a controller, never a model.
	ControllerOnlyConfigAttributes = []string{
		AllowModelAccessKey,
		AgentRateLimitMax,
		AgentRateLimitRate,
		APIPort,
		APIPortOpenDelay,
		AutocertDNSNameKey,
		AutocertURLKey,
		CACertKey,
		CharmStoreURL,
		ControllerAPIPort,
		ControllerName,
		ControllerUUIDKey,
		IdentityPublicKey,
		IdentityURL,
		SetNUMAControlPolicyKey,
		StatePort,
		MongoMemoryProfile,
		MaxDebugLogDuration,
		MaxTxnLogSize,
		MaxPruneTxnBatchSize,
		MaxPruneTxnPasses,
		ModelLogfileMaxBackups,
		ModelLogfileMaxSize,
		ModelLogsSize,
		PruneTxnQueryCount,
		PruneTxnSleepTime,
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
		MeteringURL,
		MaxCharmStateSize,
		MaxAgentStateSize,
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
		AgentRateLimitMax,
		AgentRateLimitRate,
		APIPortOpenDelay,
		AuditingEnabled,
		AuditLogCaptureArgs,
		AuditLogExcludeMethods,
		// TODO Juju 3.0: ControllerAPIPort should be required and treated
		// more like api-port.
		ControllerAPIPort,
		ControllerName,
		MaxDebugLogDuration,
		MaxPruneTxnBatchSize,
		MaxPruneTxnPasses,
		ModelLogfileMaxBackups,
		ModelLogfileMaxSize,
		ModelLogsSize,
		MongoMemoryProfile,
		PruneTxnQueryCount,
		PruneTxnSleepTime,
		JujuHASpace,
		JujuManagementSpace,
		CAASOperatorImagePath,
		CAASImageRepo,
		Features,
		MaxCharmStateSize,
		MaxAgentStateSize,
	)

	// DefaultAuditLogExcludeMethods is the default list of methods to
	// exclude from the audit log.
	DefaultAuditLogExcludeMethods = []string{
		// This special value means we exclude any methods in the set
		// listed in apiserver/observer/auditfilter.go
		ReadOnlyMethodsWildcard,
	}

	methodNameRE = regexp.MustCompile(`[[:alpha:]][[:alnum:]]*\.[[:alpha:]][[:alnum:]]*`)
)

// ControllerOnlyAttribute returns true if the specified attribute name
// is only relevant for a controller.
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
	v := c.asString(APIPortOpenDelay)
	// We know that v must be a parseable time.Duration for the config
	// to be valid.
	d, _ := time.ParseDuration(v)
	return d
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
	if value, ok := c[AuditLogExcludeMethods]; ok {
		value := value.([]interface{})
		items := set.NewStrings()
		for _, item := range value {
			items.Add(item.(string))
		}
		return items
	}
	return set.NewStrings(DefaultAuditLogExcludeMethods...)
}

// Features returns the controller config set features flags.
func (c Config) Features() set.Strings {
	features := set.NewStrings()
	if value, ok := c[Features]; ok {
		value := value.([]interface{})
		for _, item := range value {
			features.Add(item.(string))
		}
	}
	return features
}

// CharmStoreURL returns the URL to use for charmstore api calls.
func (c Config) CharmStoreURL() string {
	url := c.asString(CharmStoreURL)
	if url == "" {
		return csclient.ServerURL
	}
	return url
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

// IdentityURL returns the url of the identity manager.
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

// MongoMemoryProfile returns the selected profile or low.
func (c Config) MongoMemoryProfile() string {
	if profile, ok := c[MongoMemoryProfile]; ok {
		return profile.(string)
	}
	return DefaultMongoMemoryProfile
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

// ModelLogfileMaxBackups is the number of old model log files to keep (compressed).
func (c Config) ModelLogfileMaxBackups() int {
	return c.intOrDefault(ModelLogfileMaxBackups, DefaultModelLogfileMaxBackups)
}

// ModelLogfileMaxSizeMB is the maximum size of the log file written out by the
// controller on behalf of workers running for a model.
func (c Config) ModelLogfileMaxSizeMB() int {
	return c.sizeMBOrDefault(ModelLogfileMaxSize, DefaultModelLogfileMaxSize)
}

// ModelLogsSizeMB is the size of the capped collection used to store the model
// logs. Total size on disk will be ModelLogsSizeMB * number of models.
func (c Config) ModelLogsSizeMB() int {
	return c.sizeMBOrDefault(ModelLogsSize, DefaultModelLogsSizeMB)
}

// MaxDebugLogDuration is the maximum time a debug-log session is allowed
// to run before it is terminated by the server.
func (c Config) MaxDebugLogDuration() time.Duration {
	duration, ok := c[MaxDebugLogDuration].(time.Duration)
	if !ok {
		duration = DefaultMaxDebugLogDuration
	}
	return duration
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
	asInterface, ok := c[PruneTxnSleepTime]
	if !ok {
		asInterface = DefaultPruneTxnSleepTime
	}
	asStr, ok := asInterface.(string)
	if !ok {
		asStr = DefaultPruneTxnSleepTime
	}
	val, _ := time.ParseDuration(asStr)
	return val
}

// JujuHASpace is the network space within which the MongoDB replica-set
// should communicate.
func (c Config) JujuHASpace() string {
	return c.asString(JujuHASpace)
}

// JujuManagementSpace is the network space that agents should use to
// communicate with controllers.
func (c Config) JujuManagementSpace() string {
	return c.asString(JujuManagementSpace)
}

// CAASOperatorImagePath sets the url of the docker image
// used for the application operator.
func (c Config) CAASOperatorImagePath() string {
	return c.asString(CAASOperatorImagePath)
}

// CAASImageRepo sets the url of the docker repo
// used for the jujud operator and mongo images.
func (c Config) CAASImageRepo() string {
	return c.asString(CAASImageRepo)
}

// MeteringURL returns the URL to use for metering api calls.
func (c Config) MeteringURL() string {
	url := c.asString(MeteringURL)
	if url == "" {
		return romulus.DefaultAPIRoot
	}
	return url
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

	if v, ok := c[AgentRateLimitMax].(int); ok {
		if v < 0 {
			return errors.NotValidf("negative %s (%d)", AgentRateLimitMax, v)
		}
	}
	if v, ok := c[AgentRateLimitRate].(time.Duration); ok {
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

	if mgoMemProfile, ok := c[MongoMemoryProfile].(string); ok {
		if mgoMemProfile != MongoProfLow && mgoMemProfile != MongoProfDefault {
			return errors.Errorf("mongo-memory-profile: expected one of %q or %q got string(%q)", MongoProfLow, MongoProfDefault, mgoMemProfile)
		}
	}

	if v, ok := c[MaxDebugLogDuration].(time.Duration); ok {
		if v == 0 {
			return errors.Errorf("%s cannot be zero", MaxDebugLogDuration)
		}
	}

	if v, ok := c[ModelLogsSize].(string); ok {
		mb, err := utils.ParseSize(v)
		if err != nil {
			return errors.Annotate(err, "invalid model logs size in configuration")
		}
		if mb < 1 {
			return errors.NotValidf("model logs size less than 1 MB")
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

	if v, ok := c[CAASOperatorImagePath].(string); ok && v != "" {
		if err := resources.ValidateDockerRegistryPath(v); err != nil {
			return errors.Trace(err)
		}
	}

	if v, ok := c[CAASImageRepo].(string); ok && v != "" {
		if err := resources.ValidateDockerRegistryPath(v); err != nil {
			return errors.Trace(err)
		}
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

	if v, ok := c[AuditLogExcludeMethods].([]interface{}); ok {
		for i, name := range v {
			name := name.(string)
			if name != ReadOnlyMethodsWildcard && !methodNameRE.MatchString(name) {
				return errors.Errorf(
					`invalid audit log exclude methods: should be a list of "Facade.Method" names (or "ReadOnlyMethods"), got %q at position %d`,
					name,
					i+1,
				)
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

	return nil
}

func (c Config) validateSpaceConfig(key, topic string) error {
	val := c[key]
	if val == nil {
		return nil
	}
	if v, ok := val.(string); ok {
		if !names.IsValidSpace(v) {
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

var configChecker = schema.FieldMap(schema.Fields{
	AgentRateLimitMax:       schema.ForceInt(),
	AgentRateLimitRate:      schema.TimeDuration(),
	AuditingEnabled:         schema.Bool(),
	AuditLogCaptureArgs:     schema.Bool(),
	AuditLogMaxSize:         schema.String(),
	AuditLogMaxBackups:      schema.ForceInt(),
	AuditLogExcludeMethods:  schema.List(schema.String()),
	APIPort:                 schema.ForceInt(),
	APIPortOpenDelay:        schema.String(),
	ControllerAPIPort:       schema.ForceInt(),
	ControllerName:          schema.String(),
	StatePort:               schema.ForceInt(),
	IdentityURL:             schema.String(),
	IdentityPublicKey:       schema.String(),
	SetNUMAControlPolicyKey: schema.Bool(),
	AutocertURLKey:          schema.String(),
	AutocertDNSNameKey:      schema.String(),
	AllowModelAccessKey:     schema.Bool(),
	MongoMemoryProfile:      schema.String(),
	MaxDebugLogDuration:     schema.TimeDuration(),
	MaxTxnLogSize:           schema.String(),
	MaxPruneTxnBatchSize:    schema.ForceInt(),
	MaxPruneTxnPasses:       schema.ForceInt(),
	ModelLogfileMaxBackups:  schema.ForceInt(),
	ModelLogfileMaxSize:     schema.String(),
	ModelLogsSize:           schema.String(),
	PruneTxnQueryCount:      schema.ForceInt(),
	PruneTxnSleepTime:       schema.String(),
	JujuHASpace:             schema.String(),
	JujuManagementSpace:     schema.String(),
	CAASOperatorImagePath:   schema.String(),
	CAASImageRepo:           schema.String(),
	Features:                schema.List(schema.String()),
	CharmStoreURL:           schema.String(),
	MeteringURL:             schema.String(),
	MaxCharmStateSize:       schema.ForceInt(),
	MaxAgentStateSize:       schema.ForceInt(),
}, schema.Defaults{
	AgentRateLimitMax:       schema.Omit,
	AgentRateLimitRate:      schema.Omit,
	APIPort:                 DefaultAPIPort,
	APIPortOpenDelay:        DefaultAPIPortOpenDelay,
	ControllerAPIPort:       schema.Omit,
	ControllerName:          schema.Omit,
	AuditingEnabled:         DefaultAuditingEnabled,
	AuditLogCaptureArgs:     DefaultAuditLogCaptureArgs,
	AuditLogMaxSize:         fmt.Sprintf("%vM", DefaultAuditLogMaxSizeMB),
	AuditLogMaxBackups:      DefaultAuditLogMaxBackups,
	AuditLogExcludeMethods:  DefaultAuditLogExcludeMethods,
	StatePort:               DefaultStatePort,
	IdentityURL:             schema.Omit,
	IdentityPublicKey:       schema.Omit,
	SetNUMAControlPolicyKey: DefaultNUMAControlPolicy,
	AutocertURLKey:          schema.Omit,
	AutocertDNSNameKey:      schema.Omit,
	AllowModelAccessKey:     schema.Omit,
	MongoMemoryProfile:      DefaultMongoMemoryProfile,
	MaxDebugLogDuration:     DefaultMaxDebugLogDuration,
	MaxTxnLogSize:           fmt.Sprintf("%vM", DefaultMaxTxnLogCollectionMB),
	MaxPruneTxnBatchSize:    DefaultMaxPruneTxnBatchSize,
	MaxPruneTxnPasses:       DefaultMaxPruneTxnPasses,
	ModelLogfileMaxBackups:  DefaultModelLogfileMaxBackups,
	ModelLogfileMaxSize:     fmt.Sprintf("%vM", DefaultModelLogfileMaxSize),
	ModelLogsSize:           fmt.Sprintf("%vM", DefaultModelLogsSizeMB),
	PruneTxnQueryCount:      DefaultPruneTxnQueryCount,
	PruneTxnSleepTime:       DefaultPruneTxnSleepTime,
	JujuHASpace:             schema.Omit,
	JujuManagementSpace:     schema.Omit,
	CAASOperatorImagePath:   schema.Omit,
	CAASImageRepo:           schema.Omit,
	Features:                schema.Omit,
	CharmStoreURL:           csclient.ServerURL,
	MeteringURL:             romulus.DefaultAPIRoot,
	MaxCharmStateSize:       DefaultMaxCharmStateSize,
	MaxAgentStateSize:       DefaultMaxAgentStateSize,
})

// ConfigSchema holds information on all the fields defined by
// the config package.
var ConfigSchema = environschema.Fields{

	AgentRateLimitMax: {
		Description: "The maximum size of the token bucket used to ratelimit agent connections",
		Type:        environschema.Tint,
	},
	AgentRateLimitRate: {
		Description: "The time taken to add a new token to the ratelimit bucket",
		Type:        environschema.Tstring,
	},
	AuditingEnabled: {
		Description: "Determines if the controller records auditing information",
		Type:        environschema.Tbool,
	},
	AuditLogCaptureArgs: {
		Description: `Determines if the audit log contains the arguments passed to API methods`,
		Type:        environschema.Tbool,
	},
	AuditLogMaxSize: {
		Description: "The maximum size for the current controller audit log file",
		Type:        environschema.Tstring,
	},
	AuditLogMaxBackups: {
		Type:        environschema.Tint,
		Description: "The number of old audit log files to keep (compressed)",
	},
	AuditLogExcludeMethods: {
		Type:        environschema.FieldType("list of strings"),
		Description: "The list of Facade.Method names that aren't interesting for audit logging purposes.",
	},
	APIPort: {
		Type:        environschema.Tint,
		Description: "The port used for api connections",
	},
	APIPortOpenDelay: {
		Type: environschema.Tstring,
		Description: `The duration that the controller will wait 
between when the controller has been deemed to be ready to open 
the api-port and when the api-port is actually opened 
(only used when a controller-api-port value is set).`,
	},
	ControllerAPIPort: {
		Type: environschema.Tint,
		Description: `An optional port that may be set for controllers
that have a very heavy load. If this port is set, this port is used by
the controllers to talk to each other - used for the local API connection
as well as the pubsub forwarders, and the raft workers. If this value is
set, the api-port isn't opened until the controllers have started properly.`,
	},
	StatePort: {
		Type:        environschema.Tint,
		Description: `The port used for mongo connections`,
	},
	IdentityURL: {
		Type:        environschema.Tstring,
		Description: `The url of the identity manager`,
	},
	IdentityPublicKey: {
		Type:        environschema.Tstring,
		Description: `The public key of the identity manager`,
	},
	SetNUMAControlPolicyKey: {
		Type:        environschema.Tbool,
		Description: `Determines if the NUMA control policy is set`,
	},
	AutocertURLKey: {
		Type:        environschema.Tstring,
		Description: `The URL used to obtain official TLS certificates when a client connects to the API`,
	},
	AutocertDNSNameKey: {
		Type:        environschema.Tstring,
		Description: `The DNS name of the controller`,
	},
	AllowModelAccessKey: {
		Type: environschema.Tbool,
		Description: `Determines if the controller allows users to 
connect to models they have been authorized for even when 
they don't have any access rights to the controller itself`,
	},
	MongoMemoryProfile: {
		Type:        environschema.Tstring,
		Description: `Sets mongo memory profile`,
	},
	MaxDebugLogDuration: {
		Type:        environschema.Tstring,
		Description: `The maximum amout of time a debug-log session is allowed to run`,
	},
	MaxTxnLogSize: {
		Type:        environschema.Tstring,
		Description: `The maximum size the of capped txn log collection`,
	},
	MaxPruneTxnBatchSize: {
		Type:        environschema.Tint,
		Description: `(deprecated) The maximum number of transactions evaluated in one go when pruning`,
	},
	MaxPruneTxnPasses: {
		Type:        environschema.Tint,
		Description: `(deprecated) The maximum number of batches processed when pruning`,
	},
	ModelLogfileMaxBackups: {
		Type:        environschema.Tint,
		Description: "The number of old model log files to keep (compressed)",
	},
	ModelLogfileMaxSize: {
		Type:        environschema.Tstring,
		Description: `The maximum size of the log file written out by the controller on behalf of workers running for a model`,
	},
	ModelLogsSize: {
		Type:        environschema.Tstring,
		Description: `The size of the capped collections used to hold the logs for the models`,
	},
	PruneTxnQueryCount: {
		Type:        environschema.Tint,
		Description: `The number of transactions to read in a single query`,
	},
	PruneTxnSleepTime: {
		Type:        environschema.Tstring,
		Description: `The amount of time to sleep between processing each batch query`,
	},
	JujuHASpace: {
		Type:        environschema.Tstring,
		Description: `The network space within which the MongoDB replica-set should communicate`,
	},
	JujuManagementSpace: {
		Type:        environschema.Tstring,
		Description: `The network space that agents should use to communicate with controllers`,
	},
	CAASOperatorImagePath: {
		Type: environschema.Tstring,
		Description: `(deprected) The url of the docker image used for the application operator.
Use "caas-image-repo" instead.`,
	},
	CAASImageRepo: {
		Type:        environschema.Tstring,
		Description: `The docker repo to use for the jujud operator and mongo images`,
	},
	Features: {
		Type:        environschema.FieldType("list of strings"),
		Description: `A list of runtime changeable features to be updated`,
	},
	CharmStoreURL: {
		Type:        environschema.Tstring,
		Description: `The url for charmstore API calls`,
	},
	MeteringURL: {
		Type:        environschema.Tstring,
		Description: `The url for metrics`,
	},
	MaxCharmStateSize: {
		Type:        environschema.Tint,
		Description: `The maximum size (in bytes) of charm-specific state that units can store to the controller`,
	},
	MaxAgentStateSize: {
		Type:        environschema.Tint,
		Description: `The maximum size (in bytes) of internal state data that agents can store to the controller`,
	},
}

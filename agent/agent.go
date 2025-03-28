// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"time"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/utils/v4"
	"github.com/juju/utils/v4/shell"

	"github.com/juju/juju/agent/constants"
	"github.com/juju/juju/api"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/machinelock"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/paths"
	"github.com/juju/juju/core/semversion"
	internallogger "github.com/juju/juju/internal/logger"
	"github.com/juju/juju/internal/mongo"
)

var logger = internallogger.GetLogger("juju.agent")

const (
	// BootstrapNonce is used as a nonce for the initial controller machine.
	BootstrapNonce = "user-admin:bootstrap"

	// BootstrapControllerId is the ID of the initial controller.
	BootstrapControllerId = "0"
)

// These are base values used for the corresponding defaults.
var (
	logDir           = paths.LogDir(paths.CurrentOS())
	dataDir          = paths.DataDir(paths.CurrentOS())
	transientDataDir = paths.TransientDataDir(paths.CurrentOS())
	confDir          = paths.ConfDir(paths.CurrentOS())
	metricsSpoolDir  = paths.MetricsSpoolDir(paths.CurrentOS())
)

// Agent exposes the agent's configuration to other components. This
// interface should probably be segregated (agent.ConfigGetter and
// agent.ConfigChanger?) but YAGNI *currently* advises against same.
type Agent interface {

	// CurrentConfig returns a copy of the agent's configuration. No
	// guarantees regarding ongoing correctness are made.
	CurrentConfig() Config

	// ChangeConfig allows clients to change the agent's configuration
	// by supplying a callback that applies the changes.
	ChangeConfig(ConfigMutator) error
}

// APIHostPortsSetter trivially wraps an Agent to implement
// worker/apiaddressupdater/APIAddressSetter.
type APIHostPortsSetter struct {
	Agent
}

// SetAPIHostPorts is the APIAddressSetter interface.
func (s APIHostPortsSetter) SetAPIHostPorts(servers []network.HostPorts) error {
	return s.ChangeConfig(func(c ConfigSetter) error {
		return c.SetAPIHostPorts(servers)
	})
}

// Paths holds the directory paths used by the agent.
type Paths struct {
	// DataDir is the data directory where each agent has a subdirectory
	// containing the configuration files.
	DataDir string
	// TransientDataDir is a directory where each agent can store data that
	// is not expected to survive a reboot.
	TransientDataDir string
	// LogDir is the log directory where all logs from all agents on
	// the machine are written.
	LogDir string
	// MetricsSpoolDir is the spool directory where workloads store
	// collected metrics.
	MetricsSpoolDir string
	// ConfDir is the directory where all  config file for
	// Juju agents are stored.
	ConfDir string
}

// Migrate assigns the directory locations specified from the new path configuration.
func (p *Paths) Migrate(newPaths Paths) {
	if newPaths.DataDir != "" {
		p.DataDir = newPaths.DataDir
	}
	if newPaths.TransientDataDir != "" {
		p.TransientDataDir = newPaths.TransientDataDir
	}
	if newPaths.LogDir != "" {
		p.LogDir = newPaths.LogDir
	}
	if newPaths.MetricsSpoolDir != "" {
		p.MetricsSpoolDir = newPaths.MetricsSpoolDir
	}
	if newPaths.ConfDir != "" {
		p.ConfDir = newPaths.ConfDir
	}
}

// NewPathsWithDefaults returns a Paths struct initialized with default locations if not otherwise specified.
func NewPathsWithDefaults(p Paths) Paths {
	paths := DefaultPaths
	if p.DataDir != "" {
		paths.DataDir = p.DataDir
	}
	if p.TransientDataDir != "" {
		paths.TransientDataDir = p.TransientDataDir
	}
	if p.LogDir != "" {
		paths.LogDir = p.LogDir
	}
	if p.MetricsSpoolDir != "" {
		paths.MetricsSpoolDir = p.MetricsSpoolDir
	}
	if p.ConfDir != "" {
		paths.ConfDir = p.ConfDir
	}
	return paths
}

var (
	// DefaultPaths defines the default paths for an agent.
	DefaultPaths = Paths{
		DataDir:          dataDir,
		TransientDataDir: transientDataDir,
		LogDir:           path.Join(logDir, "juju"),
		MetricsSpoolDir:  metricsSpoolDir,
		ConfDir:          confDir,
	}
)

// SystemIdentity is the name of the file where the environment SSH key is kept.
const SystemIdentity = "system-identity"

const (
	ProviderType      = "PROVIDER_TYPE"
	ContainerType     = "CONTAINER_TYPE"
	Namespace         = "NAMESPACE"
	AgentServiceName  = "AGENT_SERVICE_NAME"
	MongoOplogSize    = "MONGO_OPLOG_SIZE"
	NUMACtlPreference = "NUMA_CTL_PREFERENCE"

	MgoStatsEnabled = "MGO_STATS_ENABLED"

	// LoggingOverride will set the logging for this agent to the value
	// specified. Model configuration will be ignored and this value takes
	// precidence for the agent.
	LoggingOverride = "LOGGING_OVERRIDE"

	LogSinkLoggerBufferSize    = "LOGSINK_LOGGER_BUFFER_SIZE"
	LogSinkLoggerFlushInterval = "LOGSINK_LOGGER_FLUSH_INTERVAL"
	LogSinkRateLimitBurst      = "LOGSINK_RATELIMIT_BURST"
	LogSinkRateLimitRefill     = "LOGSINK_RATELIMIT_REFILL"

	// These values are used to override various aspects of worker behaviour.
	// They are used for debugging or testing purposes.

	// CharmRevisionUpdateInterval controls how often the
	// charm revision update worker runs.
	CharmRevisionUpdateInterval = "CHARM_REVISION_UPDATE_INTERVAL"
)

// The Config interface is the sole way that the agent gets access to the
// configuration information for the machine and unit agents.  There should
// only be one instance of a config object for any given agent, and this
// interface is passed between multiple go routines.  The mutable methods are
// protected by a mutex, and it is expected that the caller doesn't modify any
// slice that may be returned.
//
// NOTE: should new mutating methods be added to this interface, consideration
// is needed around the synchronisation as a single instance is used in
// multiple go routines.
type Config interface {
	// DataDir returns the data directory. Each agent has a subdirectory
	// containing the configuration files.
	DataDir() string

	// TransientDataDir returns the directory where this agent should store
	// any data that is not expected to survive a reboot.
	TransientDataDir() string

	// LogDir returns the log directory. All logs from all agents on
	// the machine are written to this directory.
	LogDir() string

	// SystemIdentityPath returns the path of the file where the environment
	// SSH key is kept.
	SystemIdentityPath() string

	// Jobs returns a list of MachineJobs that need to run.
	Jobs() []model.MachineJob

	// Tag returns the tag of the entity on whose behalf the state connection
	// will be made.
	Tag() names.Tag

	// Dir returns the agent's directory.
	Dir() string

	// Nonce returns the nonce saved when the machine was provisioned
	// TODO: make this one of the key/value pairs.
	Nonce() string

	// CACert returns the CA certificate that is used to validate the state or
	// API server's certificate.
	CACert() string

	// APIAddresses returns the addresses needed to connect to the api server
	APIAddresses() ([]string, error)

	// WriteCommands returns shell commands to write the agent configuration.
	// It returns an error if the configuration does not have all the right
	// elements.
	WriteCommands(renderer shell.Renderer) ([]string, error)

	// StateServingInfo returns the details needed to run
	// a controller and reports whether those details
	// are available
	StateServingInfo() (controller.StateServingInfo, bool)

	// APIInfo returns details for connecting to the API server and
	// reports whether the details are available.
	APIInfo() (*api.Info, bool)

	// MongoInfo returns details for connecting to the controller's mongo
	// database and reports whether those details are available
	MongoInfo() (*mongo.MongoInfo, bool)

	// OldPassword returns the fallback password when connecting to the
	// API server.
	OldPassword() string

	// UpgradedToVersion returns the version for which all upgrade steps have been
	// successfully run, which is also the same as the initially deployed version.
	UpgradedToVersion() semversion.Number

	// LoggingConfig returns the logging config for this agent. Initially this
	// value is empty, but as the agent gets notified of model agent config
	// changes this value is saved.
	LoggingConfig() string

	// Value returns the value associated with the key, or an empty string if
	// the key is not found.
	Value(key string) string

	// Model returns the tag for the model that the agent belongs to.
	Model() names.ModelTag

	// Controller returns the tag for the controller that the agent belongs to.
	Controller() names.ControllerTag

	// MetricsSpoolDir returns the spool directory where workloads store
	// collected metrics.
	MetricsSpoolDir() string

	// JujuDBSnapChannel returns the channel for installing mongo snaps in
	// focal or later.
	JujuDBSnapChannel() string

	// AgentLogfileMaxSizeMB returns the maximum file size in MB of each
	// agent/controller log file.
	AgentLogfileMaxSizeMB() int

	// AgentLogfileMaxBackups returns the number of old agent/controller log
	// files to keep (compressed).
	AgentLogfileMaxBackups() int

	// QueryTracingEnabled returns whether query tracing is enabled.
	QueryTracingEnabled() bool

	// QueryTracingThreshold returns the threshold for query tracing. The
	// lower the threshold, the more queries will be output. A value of 0
	// means all queries will be output.
	QueryTracingThreshold() time.Duration

	// OpenTelemetryEnabled returns whether the open telemetry is enabled.
	OpenTelemetryEnabled() bool

	// OpenTelemetryEndpoint returns the endpoint to use for open telemetry
	// collection.
	OpenTelemetryEndpoint() string

	// OpenTelemetryInsecure returns if the endpoint is insecure. This is useful
	// for local/development testing
	OpenTelemetryInsecure() bool

	// OpenTelemetryStackTraces return if debug stack traces should be enabled
	// for each span.
	OpenTelemetryStackTraces() bool

	// OpenTelemetrySampleRatio returns the sample ratio to use for open
	// telemetry collection.
	OpenTelemetrySampleRatio() float64

	// OpenTelemetryTailSamplingThreshold returns the threshold for tail-based
	// sampling. The lower the threshold, the more spans will be sampled.
	OpenTelemetryTailSamplingThreshold() time.Duration

	// ObjectStoreType returns the type of object store to use.
	ObjectStoreType() objectstore.BackendType

	// DqlitePort returns the port that should be used by Dqlite. This should
	// only be set during testing.
	DqlitePort() (int, bool)
}

type configSetterOnly interface {
	// Clone returns a copy of the configuration that
	// is unaffected by subsequent calls to the Set*
	// methods
	Clone() Config

	// SetOldPassword sets the password that is currently
	// valid but needs to be changed. This is used as
	// a fallback.
	SetOldPassword(oldPassword string)

	// SetPassword sets the password to be used when
	// connecting to the state.
	SetPassword(newPassword string)

	// SetValue updates the value for the specified key.
	SetValue(key, value string)

	// SetUpgradedToVersion sets the version that
	// the agent has successfully upgraded to.
	SetUpgradedToVersion(newVersion semversion.Number)

	// SetAPIHostPorts sets the API host/port addresses to connect to.
	SetAPIHostPorts(servers []network.HostPorts) error

	// SetCACert sets the CA cert used for validating API connections.
	SetCACert(string)

	// SetStateServingInfo sets the information needed
	// to run a controller
	SetStateServingInfo(info controller.StateServingInfo)

	// SetControllerAPIPort sets the controller API port in the config.
	SetControllerAPIPort(port int)

	// SetJujuDBSnapChannel sets the channel for installing mongo snaps
	// when bootstrapping focal or later.
	SetJujuDBSnapChannel(string)

	// SetLoggingConfig sets the logging config value for the agent.
	SetLoggingConfig(string)

	// SetQueryTracingEnabled sets whether query tracing is enabled.
	SetQueryTracingEnabled(bool)

	// SetQueryTracingThreshold sets the threshold for query tracing.
	SetQueryTracingThreshold(time.Duration)

	// SetOpenTelemetryEnabled sets whether open telemetry is enabled.
	SetOpenTelemetryEnabled(bool)

	// SetOpenTelemetryEndpoint sets the endpoint to use for open telemetry
	// collection.
	SetOpenTelemetryEndpoint(string)

	// SetOpenTelemetryInsecure sets if the endpoint is insecure. This is
	// useful for local/development testing
	SetOpenTelemetryInsecure(bool)

	// SetOpenTelemetryStackTraces sets the debug stack traces should be
	// enabled for each span.
	SetOpenTelemetryStackTraces(bool)

	// SetOpenTelemetrySampleRatio sets the sample ratio to use for open
	// telemetry collection.
	SetOpenTelemetrySampleRatio(float64)

	// SetOpenTelemetryTailSamplingThreshold sets the threshold for tail-based
	// sampling. The lower the threshold, the more spans will be sampled.
	SetOpenTelemetryTailSamplingThreshold(time.Duration)

	// SetObjectStoreType sets the type of object store to use.
	SetObjectStoreType(objectstore.BackendType)
}

// LogFileName returns the filename for the Agent's log file.
func LogFilename(c Config) string {
	return filepath.Join(c.LogDir(), c.Tag().String()+".log")
}

// MachineLockLogFilename returns the filename for the machine lock log file.
func MachineLockLogFilename(c Config) string {
	return filepath.Join(c.LogDir(), machinelock.Filename)
}

type ConfigMutator func(ConfigSetter) error

type ConfigRenderer interface {
	// Render generates the agent configuration
	// as a byte array.
	Render() ([]byte, error)
}

type ConfigWriter interface {
	ConfigRenderer

	// Write writes the agent configuration.
	Write() error
}

type ConfigSetter interface {
	Config
	configSetterOnly
}

type ConfigSetterWriter interface {
	Config
	configSetterOnly
	ConfigWriter
}

// Ensure that the configInternal struct implements the Config interface.
var _ Config = (*configInternal)(nil)

type apiDetails struct {
	addresses []string
	password  string
}

func (d *apiDetails) clone() *apiDetails {
	if d == nil {
		return nil
	}
	newd := *d
	newd.addresses = append([]string{}, d.addresses...)
	return &newd
}

type configInternal struct {
	configFilePath                     string
	paths                              Paths
	tag                                names.Tag
	nonce                              string
	controller                         names.ControllerTag
	model                              names.ModelTag
	jobs                               []model.MachineJob
	upgradedToVersion                  semversion.Number
	caCert                             string
	apiDetails                         *apiDetails
	statePassword                      string
	oldPassword                        string
	servingInfo                        *controller.StateServingInfo
	loggingConfig                      string
	values                             map[string]string
	jujuDBSnapChannel                  string
	agentLogfileMaxSizeMB              int
	agentLogfileMaxBackups             int
	queryTracingEnabled                bool
	queryTracingThreshold              time.Duration
	openTelemetryEnabled               bool
	openTelemetryEndpoint              string
	openTelemetryInsecure              bool
	openTelemetryStackTraces           bool
	openTelemetrySampleRatio           float64
	openTelemetryTailSamplingThreshold time.Duration
	objectStoreType                    objectstore.BackendType
	dqlitePort                         int
}

// AgentConfigParams holds the parameters required to create
// a new AgentConfig.
type AgentConfigParams struct {
	Paths                              Paths
	Jobs                               []model.MachineJob
	UpgradedToVersion                  semversion.Number
	Tag                                names.Tag
	Password                           string
	Nonce                              string
	Controller                         names.ControllerTag
	Model                              names.ModelTag
	APIAddresses                       []string
	CACert                             string
	Values                             map[string]string
	JujuDBSnapChannel                  string
	AgentLogfileMaxSizeMB              int
	AgentLogfileMaxBackups             int
	QueryTracingEnabled                bool
	QueryTracingThreshold              time.Duration
	OpenTelemetryEnabled               bool
	OpenTelemetryEndpoint              string
	OpenTelemetryInsecure              bool
	OpenTelemetryStackTraces           bool
	OpenTelemetrySampleRatio           float64
	OpenTelemetryTailSamplingThreshold time.Duration
	ObjectStoreType                    objectstore.BackendType
	DqlitePort                         int
}

// NewAgentConfig returns a new config object suitable for use for a
// machine or unit agent.
func NewAgentConfig(configParams AgentConfigParams) (ConfigSetterWriter, error) {
	if configParams.Paths.DataDir == "" {
		return nil, errors.Trace(requiredError("data directory"))
	}
	if configParams.Tag == nil {
		return nil, errors.Trace(requiredError("entity tag"))
	}
	switch configParams.Tag.(type) {
	case names.MachineTag,
		names.ModelTag,
		names.UnitTag,
		names.ApplicationTag,
		names.ControllerAgentTag:
		// These are the only five type of tags that can represent an agent
		// IAAS - machine and unit
		// CAAS - application, controller agent, model
	default:
		return nil, errors.Errorf("entity tag must be MachineTag, UnitTag, ApplicationTag or ControllerAgentTag, got %T", configParams.Tag)
	}
	if configParams.UpgradedToVersion == semversion.Zero {
		return nil, errors.Trace(requiredError("upgradedToVersion"))
	}
	if configParams.Password == "" {
		return nil, errors.Trace(requiredError("password"))
	}
	if uuid := configParams.Controller.Id(); uuid == "" {
		return nil, errors.Trace(requiredError("controller"))
	} else if !names.IsValidController(uuid) {
		return nil, errors.Errorf("%q is not a valid controller uuid", uuid)
	}
	if uuid := configParams.Model.Id(); uuid == "" {
		return nil, errors.Trace(requiredError("model"))
	} else if !names.IsValidModel(uuid) {
		return nil, errors.Errorf("%q is not a valid model uuid", uuid)
	}
	if len(configParams.CACert) == 0 {
		return nil, errors.Trace(requiredError("CA certificate"))
	}
	// Note that the password parts of the state and api information are
	// blank.  This is by design: we want to generate a secure password
	// for new agents. So, we create this config without a current password
	// which signals to apicaller worker that it should try to connect using old password.
	// When/if this connection is successful, apicaller worker will generate
	// a new secure password and update this agent's config.
	config := &configInternal{
		paths:                              NewPathsWithDefaults(configParams.Paths),
		jobs:                               configParams.Jobs,
		upgradedToVersion:                  configParams.UpgradedToVersion,
		tag:                                configParams.Tag,
		nonce:                              configParams.Nonce,
		controller:                         configParams.Controller,
		model:                              configParams.Model,
		caCert:                             configParams.CACert,
		oldPassword:                        configParams.Password,
		values:                             configParams.Values,
		jujuDBSnapChannel:                  configParams.JujuDBSnapChannel,
		agentLogfileMaxSizeMB:              configParams.AgentLogfileMaxSizeMB,
		agentLogfileMaxBackups:             configParams.AgentLogfileMaxBackups,
		queryTracingEnabled:                configParams.QueryTracingEnabled,
		queryTracingThreshold:              configParams.QueryTracingThreshold,
		openTelemetryEnabled:               configParams.OpenTelemetryEnabled,
		openTelemetryEndpoint:              configParams.OpenTelemetryEndpoint,
		openTelemetryInsecure:              configParams.OpenTelemetryInsecure,
		openTelemetryStackTraces:           configParams.OpenTelemetryStackTraces,
		openTelemetrySampleRatio:           configParams.OpenTelemetrySampleRatio,
		openTelemetryTailSamplingThreshold: configParams.OpenTelemetryTailSamplingThreshold,
		objectStoreType:                    configParams.ObjectStoreType,
		dqlitePort:                         configParams.DqlitePort,
	}
	if len(configParams.APIAddresses) > 0 {
		config.apiDetails = &apiDetails{
			addresses: configParams.APIAddresses,
		}
	}
	if err := config.check(); err != nil {
		return nil, err
	}
	if config.values == nil {
		config.values = make(map[string]string)
	}
	config.configFilePath = ConfigPath(config.paths.DataDir, config.tag)
	return config, nil
}

// NewStateMachineConfig returns a configuration suitable for
// a machine running the controller.
func NewStateMachineConfig(configParams AgentConfigParams, serverInfo controller.StateServingInfo) (ConfigSetterWriter, error) {
	if serverInfo.Cert == "" {
		return nil, errors.Trace(requiredError("controller cert"))
	}
	if serverInfo.PrivateKey == "" {
		return nil, errors.Trace(requiredError("controller key"))
	}
	if serverInfo.CAPrivateKey == "" {
		return nil, errors.Trace(requiredError("ca cert key"))
	}
	if serverInfo.StatePort == 0 {
		return nil, errors.Trace(requiredError("state port"))
	}
	if serverInfo.APIPort == 0 {
		return nil, errors.Trace(requiredError("api port"))
	}
	config, err := NewAgentConfig(configParams)
	if err != nil {
		return nil, err
	}
	config.SetStateServingInfo(serverInfo)
	return config, nil
}

// BaseDir returns the directory containing the data directories for
// all the agents.
func BaseDir(dataDir string) string {
	// Note: must use path, not filepath, as this function is
	// (indirectly) used by the client on Windows.
	return path.Join(dataDir, "agents")
}

// Dir returns the agent-specific data directory.
func Dir(dataDir string, tag names.Tag) string {
	// Note: must use path, not filepath, as this
	// function is used by the client on Windows.
	return path.Join(BaseDir(dataDir), tag.String())
}

// ConfigPath returns the full path to the agent config file.
// NOTE: Delete this once all agents accept --config instead
// of --data-dir - it won't be needed anymore.
func ConfigPath(dataDir string, tag names.Tag) string {
	return filepath.Join(Dir(dataDir, tag), constants.AgentConfigFilename)
}

// ReadConfig reads configuration data from the given location.
func ReadConfig(configFilePath string) (ConfigSetterWriter, error) {
	var (
		format formatter
		config *configInternal
	)
	configData, err := os.ReadFile(configFilePath)
	if err != nil {
		return nil, errors.Annotatef(err, "cannot read agent config %q", configFilePath)
	}
	format, config, err = parseConfigData(configData)
	if err != nil {
		return nil, err
	}
	logger.Debugf(context.TODO(), "read agent config, format %q", format.version())
	config.configFilePath = configFilePath
	return config, nil
}

// ParseConfigData parses configuration data.
func ParseConfigData(configData []byte) (ConfigSetterWriter, error) {
	format, config, err := parseConfigData(configData)
	if err != nil {
		return nil, errors.Trace(err)
	}
	logger.Debugf(context.TODO(), "parsing agent config, format %q", format.version())
	config.configFilePath = ConfigPath(config.paths.DataDir, config.tag)
	return config, nil
}

func (c0 *configInternal) Clone() Config {
	c1 := *c0
	// Deep copy only fields which may be affected
	// by ConfigSetter methods.
	c1.apiDetails = c0.apiDetails.clone()
	c1.jobs = append([]model.MachineJob{}, c0.jobs...)
	c1.values = make(map[string]string, len(c0.values))
	for key, val := range c0.values {
		c1.values[key] = val
	}
	if c0.servingInfo != nil {
		info := *c0.servingInfo
		c1.servingInfo = &info
	}
	return &c1
}

func (c *configInternal) SetUpgradedToVersion(newVersion semversion.Number) {
	c.upgradedToVersion = newVersion
}

func (c *configInternal) SetAPIHostPorts(servers []network.HostPorts) error {
	if len(servers) == 0 {
		return errors.BadRequestf("servers not provided")
	}
	if c.apiDetails == nil {
		// This shouldn't happen, NewAgentConfig checks valid addresses.
		c.apiDetails = &apiDetails{}
	}
	var addrs []string
	for _, serverHostPorts := range servers {
		hps := serverHostPorts.PrioritizedForScope(network.ScopeMatchCloudLocal)
		addrs = append(addrs, hps...)
	}
	c.apiDetails.addresses = addrs
	logger.Debugf(context.TODO(), "API server address details %q written to agent config as %q", servers, addrs)
	return nil
}

func (c *configInternal) SetCACert(cert string) {
	c.caCert = cert
}

func (c *configInternal) SetValue(key, value string) {
	if value == "" {
		delete(c.values, key)
	} else {
		c.values[key] = value
	}
}

// LoggingConfig implements Config.
func (c *configInternal) LoggingConfig() string {
	return c.loggingConfig
}

// SetLoggingConfig implements configSetterOnly.
func (c *configInternal) SetLoggingConfig(value string) {
	c.loggingConfig = value
}

func (c *configInternal) SetOldPassword(oldPassword string) {
	c.oldPassword = oldPassword
}

func (c *configInternal) SetPassword(newPassword string) {
	if c.servingInfo != nil {
		c.statePassword = newPassword
	}
	if c.apiDetails != nil {
		c.apiDetails.password = newPassword
	}
}

func (c *configInternal) Write() error {
	data, err := c.Render()
	if err != nil {
		return err
	}
	// Make sure the config dir gets created.
	configDir := filepath.Dir(c.configFilePath)
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("cannot create agent config dir %q: %v", configDir, err)
	}
	return utils.AtomicWriteFile(c.configFilePath, data, 0600)
}

func requiredError(what string) error {
	return fmt.Errorf("%s not found in configuration", what)
}

func (c *configInternal) File(name string) string {
	return path.Join(c.Dir(), name)
}

func (c *configInternal) DataDir() string {
	return c.paths.DataDir
}

func (c *configInternal) TransientDataDir() string {
	return c.paths.TransientDataDir
}

func (c *configInternal) MetricsSpoolDir() string {
	return c.paths.MetricsSpoolDir
}

func (c *configInternal) LogDir() string {
	return c.paths.LogDir
}

func (c *configInternal) SystemIdentityPath() string {
	return filepath.Join(c.paths.DataDir, SystemIdentity)
}

func (c *configInternal) Jobs() []model.MachineJob {
	return c.jobs
}

func (c *configInternal) Nonce() string {
	return c.nonce
}

func (c *configInternal) UpgradedToVersion() semversion.Number {
	return c.upgradedToVersion
}

func (c *configInternal) CACert() string {
	return c.caCert
}

func (c *configInternal) Value(key string) string {
	return c.values[key]
}

func (c *configInternal) StateServingInfo() (controller.StateServingInfo, bool) {
	if c.servingInfo == nil {
		return controller.StateServingInfo{}, false
	}
	return *c.servingInfo, true
}

func (c *configInternal) SetStateServingInfo(info controller.StateServingInfo) {
	c.servingInfo = &info
	if c.statePassword == "" && c.apiDetails != nil {
		c.statePassword = c.apiDetails.password
	}
}

func (c *configInternal) SetControllerAPIPort(port int) {
	if c.servingInfo != nil {
		c.servingInfo.ControllerAPIPort = port
	}
}

func (c *configInternal) APIAddresses() ([]string, error) {
	if c.apiDetails == nil {
		return []string{}, errors.New("No apidetails in config")
	}
	return append([]string{}, c.apiDetails.addresses...), nil
}

func (c *configInternal) OldPassword() string {
	return c.oldPassword
}

func (c *configInternal) Tag() names.Tag {
	return c.tag
}

func (c *configInternal) Model() names.ModelTag {
	return c.model
}

func (c *configInternal) Controller() names.ControllerTag {
	return c.controller
}

func (c *configInternal) Dir() string {
	return Dir(c.paths.DataDir, c.tag)
}

func (c *configInternal) check() error {
	if c.apiDetails == nil {
		return errors.Trace(requiredError("API addresses"))
	}
	if c.apiDetails != nil {
		if err := checkAddrs(c.apiDetails.addresses, "API server address"); err != nil {
			return err
		}
	}
	return nil
}

// JujuDBSnapChannel implements Config.
func (c *configInternal) JujuDBSnapChannel() string {
	return c.jujuDBSnapChannel
}

// SetJujuDBSnapChannel implements configSetterOnly.
func (c *configInternal) SetJujuDBSnapChannel(snapChannel string) {
	c.jujuDBSnapChannel = snapChannel
}

// AgentLogfileMaxSizeMB implements Config.
func (c *configInternal) AgentLogfileMaxSizeMB() int {
	return c.agentLogfileMaxSizeMB
}

// AgentLogfileMaxBackups implements Config.
func (c *configInternal) AgentLogfileMaxBackups() int {
	return c.agentLogfileMaxBackups
}

// QueryTracingEnabled implements Config.
func (c *configInternal) QueryTracingEnabled() bool {
	return c.queryTracingEnabled
}

// SetQueryTracingEnabled implements configSetterOnly.
func (c *configInternal) SetQueryTracingEnabled(v bool) {
	c.queryTracingEnabled = v
}

// QueryTracingThreshold implements Config.
func (c *configInternal) QueryTracingThreshold() time.Duration {
	return c.queryTracingThreshold
}

// SetQueryTracingThreshold implements configSetterOnly.
func (c *configInternal) SetQueryTracingThreshold(v time.Duration) {
	c.queryTracingThreshold = v
}

// OpenTelemetryEnabled implements Config.
func (c *configInternal) OpenTelemetryEnabled() bool {
	return c.openTelemetryEnabled
}

// SetOpenTelemetryEnabled implements configSetterOnly.
func (c *configInternal) SetOpenTelemetryEnabled(v bool) {
	c.openTelemetryEnabled = v
}

// OpenTelemetryEndpoint implements Config.
func (c *configInternal) OpenTelemetryEndpoint() string {
	return c.openTelemetryEndpoint
}

// SetOpenTelemetryEndpoint implements configSetterOnly.
func (c *configInternal) SetOpenTelemetryEndpoint(v string) {
	c.openTelemetryEndpoint = v
}

// OpenTelemetryInsecure implements Config.
func (c *configInternal) OpenTelemetryInsecure() bool {
	return c.openTelemetryInsecure
}

// SetopenTelemetryInsecure implements configSetterOnly.
func (c *configInternal) SetOpenTelemetryInsecure(v bool) {
	c.openTelemetryInsecure = v
}

// OpenTelemetryStackTraces implements Config.
func (c *configInternal) OpenTelemetryStackTraces() bool {
	return c.openTelemetryStackTraces
}

// SetOpenTelemetryStackTraces implements configSetterOnly.
func (c *configInternal) SetOpenTelemetryStackTraces(v bool) {
	c.openTelemetryStackTraces = v
}

// OpenTelemetrySampleRatio implements Config.
func (c *configInternal) OpenTelemetrySampleRatio() float64 {
	return c.openTelemetrySampleRatio
}

// SetOpenTelemetryStackTraces implements configSetterOnly.
func (c *configInternal) SetOpenTelemetrySampleRatio(v float64) {
	c.openTelemetrySampleRatio = v
}

// OpenTelemetryTailSamplingThreshold implements Config.
func (c *configInternal) OpenTelemetryTailSamplingThreshold() time.Duration {
	return c.openTelemetryTailSamplingThreshold
}

// SetOpenTelemetryTailSamplingThreshold implements configSetterOnly.
func (c *configInternal) SetOpenTelemetryTailSamplingThreshold(v time.Duration) {
	c.openTelemetryTailSamplingThreshold = v
}

// ObjectStoreType implements Config.
func (c *configInternal) ObjectStoreType() objectstore.BackendType {
	return c.objectStoreType
}

// SetObjectStoreType implements configSetterOnly.
func (c *configInternal) SetObjectStoreType(v objectstore.BackendType) {
	c.objectStoreType = v
}

var validAddr = regexp.MustCompile("^.+:[0-9]+$")

func checkAddrs(addrs []string, what string) error {
	if len(addrs) == 0 {
		return errors.Trace(requiredError(what))
	}
	for _, a := range addrs {
		if !validAddr.MatchString(a) {
			return errors.Errorf("invalid %s %q", what, a)
		}
	}
	return nil
}

func (c *configInternal) Render() ([]byte, error) {
	data, err := currentFormat.marshal(c)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "%s%s\n", formatPrefix, currentFormat.version())
	buf.Write(data)
	return buf.Bytes(), nil
}

// WriteCommands is defined on Config interface.
func (c *configInternal) WriteCommands(renderer shell.Renderer) ([]string, error) {
	data, err := c.Render()
	if err != nil {
		return nil, errors.Trace(err)
	}
	commands := renderer.MkdirAll(c.Dir())
	filename := c.File(constants.AgentConfigFilename)
	commands = append(commands, renderer.WriteFile(filename, data)...)
	commands = append(commands, renderer.Chmod(filename, 0600)...)
	return commands, nil
}

// APIInfo is defined on Config interface.
func (c *configInternal) APIInfo() (*api.Info, bool) {
	if c.apiDetails == nil || c.apiDetails.addresses == nil {
		return nil, false
	}
	servingInfo, isController := c.StateServingInfo()
	addrs := c.apiDetails.addresses
	// For controllers, we return only localhost - we should not connect
	// to other controllers if we can talk locally.
	if isController {
		port := servingInfo.APIPort
		// If the controller has been configured with a controller api port,
		// we return that instead of the normal api port.
		if servingInfo.ControllerAPIPort != 0 {
			port = servingInfo.ControllerAPIPort
		}
		// TODO(macgreagoir) IPv6. Ubuntu still always provides IPv4
		// loopback, and when/if this changes localhost should resolve
		// to IPv6 loopback in any case (lp:1644009). Review.
		localAPIAddr := net.JoinHostPort("localhost", strconv.Itoa(port))

		// TODO (manadart 2023-03-27): This is a temporary change from using
		// *only* the localhost address, to fix an issue where we can get the
		// configuration change that tells a new machine that it is a
		// controller *before* the machine agent has completed its first run
		// set its status to "running". When this happens we deadlock, because
		// the peergrouper has not joined the machine to replica-set, so there
		// will never be a working API available at localhost.
		if !set.NewStrings(addrs...).Contains(localAPIAddr) {
			addrs = append([]string{localAPIAddr}, addrs...)
		}
	}
	return &api.Info{
		Addrs:    addrs,
		Password: c.apiDetails.password,
		CACert:   c.caCert,
		Tag:      c.tag,
		Nonce:    c.nonce,
		ModelTag: c.model,
	}, true
}

// MongoInfo is defined on Config interface.
func (c *configInternal) MongoInfo() (info *mongo.MongoInfo, ok bool) {
	if c.apiDetails == nil || c.apiDetails.addresses == nil {
		return nil, false
	}
	ssi, ok := c.StateServingInfo()
	if !ok {
		return nil, false
	}
	addrs := c.apiDetails.addresses
	var netAddrs network.SpaceAddresses
	for _, addr := range addrs {
		host, _, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, false
		}
		if host == "localhost" {
			continue
		}
		netAddrs = append(netAddrs, network.NewSpaceAddress(host))
	}
	// We should only be connecting to mongo on cloud local addresses,
	// not fan or public etc.
	hostPorts := network.SpaceAddressesWithPort(netAddrs, ssi.StatePort)
	mongoAddrs := hostPorts.AllMatchingScope(network.ScopeMatchCloudLocal)

	// We return localhost first and then all addresses of known API
	// endpoints - this lets us connect to other Mongo instances and start
	// state even if our own Mongo has not started yet (see lp:1749383 #1).
	// TODO(macgreagoir) IPv6. Ubuntu still always provides IPv4 loopback,
	// and when/if this changes localhost should resolve to IPv6 loopback
	// in any case (lp:1644009). Review.
	local := net.JoinHostPort("localhost", strconv.Itoa(ssi.StatePort))
	mongoAddrs = append([]string{local}, mongoAddrs...)
	logger.Debugf(context.TODO(), "potential mongo addresses: %v", mongoAddrs)
	return &mongo.MongoInfo{
		Info: mongo.Info{
			Addrs:  mongoAddrs,
			CACert: c.caCert,
		},
		Password: c.statePassword,
		Tag:      c.tag,
	}, true
}

// DqlitePort is defined on Config interface.
func (c *configInternal) DqlitePort() (int, bool) {
	return c.dqlitePort, c.dqlitePort > 0
}

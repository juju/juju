// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils"
	"github.com/juju/utils/shell"
	"github.com/juju/version"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/api"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/juju/paths"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state/multiwatcher"
)

var logger = loggo.GetLogger("juju.agent")

const (
	// BootstrapNonce is used as a nonce for the initial controller machine.
	BootstrapNonce = "user-admin:bootstrap"

	// BootstrapMachineId is the ID of the initial controller machine.
	BootstrapMachineId = "0"

	// MachineLockName is the name of the mutex that the agent creates to
	// ensure serialization of tasks such as uniter hook executions, juju-run,
	// and others.
	MachineLockName = "machine-lock"
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
func (s APIHostPortsSetter) SetAPIHostPorts(servers [][]network.HostPort) error {
	return s.ChangeConfig(func(c ConfigSetter) error {
		c.SetAPIHostPorts(servers)
		return nil
	})
}

// StateServingInfoSetter trivially wraps an Agent to implement
// worker/certupdater/SetStateServingInfo.
type StateServingInfoSetter struct {
	Agent
}

// SetStateServingInfo is the SetStateServingInfo interface.
func (s StateServingInfoSetter) SetStateServingInfo(info params.StateServingInfo) error {
	return s.ChangeConfig(func(c ConfigSetter) error {
		c.SetStateServingInfo(info)
		return nil
	})
}

// Paths holds the directory paths used by the agent.
type Paths struct {
	// DataDir is the data directory where each agent has a subdirectory
	// containing the configuration files.
	DataDir string
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
		DataDir:         paths.Data,
		LogDir:          path.Join(paths.Log, "juju"),
		MetricsSpoolDir: paths.MetricsSpool,
		ConfDir:         paths.Conf,
	}
)

// SystemIdentity is the name of the file where the environment SSH key is kept.
const SystemIdentity = "system-identity"

const (
	LxcBridge         = "LXC_BRIDGE"
	LxdBridge         = "LXD_BRIDGE"
	ProviderType      = "PROVIDER_TYPE"
	ContainerType     = "CONTAINER_TYPE"
	Namespace         = "NAMESPACE"
	AgentServiceName  = "AGENT_SERVICE_NAME"
	MongoOplogSize    = "MONGO_OPLOG_SIZE"
	NUMACtlPreference = "NUMA_CTL_PREFERENCE"
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

	// LogDir returns the log directory. All logs from all agents on
	// the machine are written to this directory.
	LogDir() string

	// SystemIdentityPath returns the path of the file where the environment
	// SSH key is kept.
	SystemIdentityPath() string

	// Jobs returns a list of MachineJobs that need to run.
	Jobs() []multiwatcher.MachineJob

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
	StateServingInfo() (params.StateServingInfo, bool)

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
	UpgradedToVersion() version.Number

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

	// MongoVersion returns the version of mongo that the state server
	// is using.
	MongoVersion() mongo.Version
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
	SetUpgradedToVersion(newVersion version.Number)

	// SetAPIHostPorts sets the API host/port addresses to connect to.
	SetAPIHostPorts(servers [][]network.HostPort)

	// SetCACert sets the CA cert used for validating API connections.
	SetCACert(string)

	// SetStateServingInfo sets the information needed
	// to run a controller
	SetStateServingInfo(info params.StateServingInfo)

	// SetMongoVersion sets the passed version as currently in use.
	SetMongoVersion(mongo.Version)
}

// LogFileName returns the filename for the Agent's log file.
func LogFilename(c Config) string {
	return filepath.Join(c.LogDir(), c.Tag().String()+".log")
}

type ConfigMutator func(ConfigSetter) error

type ConfigWriter interface {
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

type connectionDetails struct {
	addresses []string
	password  string
}

func (d *connectionDetails) clone() *connectionDetails {
	if d == nil {
		return nil
	}
	newd := *d
	newd.addresses = append([]string{}, d.addresses...)
	return &newd
}

type configInternal struct {
	configFilePath    string
	paths             Paths
	tag               names.Tag
	nonce             string
	controller        names.ControllerTag
	model             names.ModelTag
	jobs              []multiwatcher.MachineJob
	upgradedToVersion version.Number
	caCert            string
	stateDetails      *connectionDetails
	apiDetails        *connectionDetails
	oldPassword       string
	servingInfo       *params.StateServingInfo
	values            map[string]string
	mongoVersion      string
}

// AgentConfigParams holds the parameters required to create
// a new AgentConfig.
type AgentConfigParams struct {
	Paths             Paths
	Jobs              []multiwatcher.MachineJob
	UpgradedToVersion version.Number
	Tag               names.Tag
	Password          string
	Nonce             string
	Controller        names.ControllerTag
	Model             names.ModelTag
	StateAddresses    []string
	APIAddresses      []string
	CACert            string
	Values            map[string]string
	MongoVersion      mongo.Version
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
	case names.MachineTag, names.UnitTag:
		// these are the only two type of tags that can represent an agent
	default:
		return nil, errors.Errorf("entity tag must be MachineTag or UnitTag, got %T", configParams.Tag)
	}
	if configParams.UpgradedToVersion == version.Zero {
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
		paths:             NewPathsWithDefaults(configParams.Paths),
		jobs:              configParams.Jobs,
		upgradedToVersion: configParams.UpgradedToVersion,
		tag:               configParams.Tag,
		nonce:             configParams.Nonce,
		controller:        configParams.Controller,
		model:             configParams.Model,
		caCert:            configParams.CACert,
		oldPassword:       configParams.Password,
		values:            configParams.Values,
		mongoVersion:      configParams.MongoVersion.String(),
	}

	if len(configParams.StateAddresses) > 0 {
		config.stateDetails = &connectionDetails{
			addresses: configParams.StateAddresses,
		}
	}
	if len(configParams.APIAddresses) > 0 {
		config.apiDetails = &connectionDetails{
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
func NewStateMachineConfig(configParams AgentConfigParams, serverInfo params.StateServingInfo) (ConfigSetterWriter, error) {
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
	return filepath.Join(Dir(dataDir, tag), agentConfigFilename)
}

// ReadConfig reads configuration data from the given location.
func ReadConfig(configFilePath string) (ConfigSetterWriter, error) {
	var (
		format formatter
		config *configInternal
	)
	configData, err := ioutil.ReadFile(configFilePath)
	if err != nil {
		return nil, fmt.Errorf("cannot read agent config %q: %v", configFilePath, err)
	}
	format, config, err = parseConfigData(configData)
	if err != nil {
		return nil, err
	}
	logger.Debugf("read agent config, format %q", format.version())
	config.configFilePath = configFilePath
	return config, nil
}

func (c0 *configInternal) Clone() Config {
	c1 := *c0
	// Deep copy only fields which may be affected
	// by ConfigSetter methods.
	c1.stateDetails = c0.stateDetails.clone()
	c1.apiDetails = c0.apiDetails.clone()
	c1.jobs = append([]multiwatcher.MachineJob{}, c0.jobs...)
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

func (c *configInternal) SetUpgradedToVersion(newVersion version.Number) {
	c.upgradedToVersion = newVersion
}

func (c *configInternal) SetAPIHostPorts(servers [][]network.HostPort) {
	if c.apiDetails == nil {
		return
	}
	var addrs []string
	for _, serverHostPorts := range servers {
		hps := network.PrioritizeInternalHostPorts(serverHostPorts, false)
		addrs = append(addrs, hps...)
	}
	c.apiDetails.addresses = addrs
	logger.Infof("API server address details %q written to agent config as %q", servers, addrs)
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

func (c *configInternal) SetOldPassword(oldPassword string) {
	c.oldPassword = oldPassword
}

func (c *configInternal) SetPassword(newPassword string) {
	if c.stateDetails != nil {
		c.stateDetails.password = newPassword
	}
	if c.apiDetails != nil {
		c.apiDetails.password = newPassword
	}
}

func (c *configInternal) Write() error {
	data, err := c.fileContents()
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

func (c *configInternal) MetricsSpoolDir() string {
	return c.paths.MetricsSpoolDir
}

func (c *configInternal) LogDir() string {
	return c.paths.LogDir
}

func (c *configInternal) SystemIdentityPath() string {
	return filepath.Join(c.paths.DataDir, SystemIdentity)
}

func (c *configInternal) Jobs() []multiwatcher.MachineJob {
	return c.jobs
}

func (c *configInternal) Nonce() string {
	return c.nonce
}

func (c *configInternal) UpgradedToVersion() version.Number {
	return c.upgradedToVersion
}

func (c *configInternal) CACert() string {
	return c.caCert
}

func (c *configInternal) Value(key string) string {
	return c.values[key]
}

func (c *configInternal) StateServingInfo() (params.StateServingInfo, bool) {
	if c.servingInfo == nil {
		return params.StateServingInfo{}, false
	}
	return *c.servingInfo, true
}

func (c *configInternal) SetStateServingInfo(info params.StateServingInfo) {
	c.servingInfo = &info
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
	if c.stateDetails == nil && c.apiDetails == nil {
		return errors.Trace(requiredError("state or API addresses"))
	}
	if c.stateDetails != nil {
		if err := checkAddrs(c.stateDetails.addresses, "controller address"); err != nil {
			return err
		}
	}
	if c.apiDetails != nil {
		if err := checkAddrs(c.apiDetails.addresses, "API server address"); err != nil {
			return err
		}
	}
	return nil
}

// MongoVersion implements Config.
func (c *configInternal) MongoVersion() mongo.Version {
	v, err := mongo.NewVersion(c.mongoVersion)
	if err != nil {
		return mongo.Mongo24
	}
	return v
}

// SetMongoVersion implements configSetterOnly.
func (c *configInternal) SetMongoVersion(v mongo.Version) {
	c.mongoVersion = v.String()
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

func (c *configInternal) fileContents() ([]byte, error) {
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
	data, err := c.fileContents()
	if err != nil {
		return nil, errors.Trace(err)
	}
	commands := renderer.MkdirAll(c.Dir())
	filename := c.File(agentConfigFilename)
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
	if isController {
		port := servingInfo.APIPort
		localAPIAddr := net.JoinHostPort("localhost", strconv.Itoa(port))
		addrInAddrs := false
		for _, addr := range addrs {
			if addr == localAPIAddr {
				addrInAddrs = true
				break
			}
		}
		if !addrInAddrs {
			addrs = append(addrs, localAPIAddr)
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
	ssi, ok := c.StateServingInfo()
	if !ok {
		return nil, false
	}
	addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(ssi.StatePort))
	return &mongo.MongoInfo{
		Info: mongo.Info{
			Addrs:  []string{addr},
			CACert: c.caCert,
		},
		Password: c.stateDetails.password,
		Tag:      c.tag,
	}, true
}

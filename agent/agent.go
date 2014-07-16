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
	"strings"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"
	"github.com/juju/utils"

	"github.com/juju/juju/environmentserver/authentication"
	"github.com/juju/juju/juju/paths"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state/api"
	"github.com/juju/juju/state/api/params"
	"github.com/juju/juju/version"
)

var logger = loggo.GetLogger("juju.agent")

// logDir returns a filesystem path to the location where juju
// may create a folder containing its logs
var logDir = paths.MustSucceed(paths.LogDir(version.Current.Series))

// dataDir returns the default data directory for this running system
var dataDir = paths.MustSucceed(paths.DataDir(version.Current.Series))

// DefaultLogDir defines the default log directory for juju agents.
// It's defined as a variable so it could be overridden in tests.
var DefaultLogDir = path.Join(logDir, "juju")

// DefaultDataDir defines the default data directory for juju agents.
// It's defined as a variable so it could be overridden in tests.
var DefaultDataDir = dataDir

// SystemIdentity is the name of the file where the environment SSH key is kept.
const SystemIdentity = "system-identity"

const (
	LxcBridge        = "LXC_BRIDGE"
	ProviderType     = "PROVIDER_TYPE"
	ContainerType    = "CONTAINER_TYPE"
	Namespace        = "NAMESPACE"
	StorageDir       = "STORAGE_DIR"
	StorageAddr      = "STORAGE_ADDR"
	AgentServiceName = "AGENT_SERVICE_NAME"
	MongoOplogSize   = "MONGO_OPLOG_SIZE"
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
	Jobs() []params.MachineJob

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
	WriteCommands() ([]string, error)

	// StateServingInfo returns the details needed to run
	// a state server and reports whether those details
	// are available
	StateServingInfo() (params.StateServingInfo, bool)

	// APIInfo returns details for connecting to the API server.
	APIInfo() *api.Info

	// MongoInfo returns details for connecting to the state server's mongo
	// database and reports whether those details are available
	MongoInfo() (*authentication.MongoInfo, bool)

	// OldPassword returns the fallback password when connecting to the
	// API server.
	OldPassword() string

	// UpgradedToVersion returns the version for which all upgrade steps have been
	// successfully run, which is also the same as the initially deployed version.
	UpgradedToVersion() version.Number

	// Value returns the value associated with the key, or an empty string if
	// the key is not found.
	Value(key string) string

	// PreferIPv6 returns whether to prefer using IPv6 addresses (if
	// available) when connecting to the state or API server.
	PreferIPv6() bool
}

type ConfigSetterOnly interface {
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

	// SetUpgradedToVerson sets the version that
	// the agent has successfully upgraded to.
	SetUpgradedToVersion(newVersion version.Number)

	// SetAPIHostPorts sets the API host/port addresses to connect to.
	SetAPIHostPorts(servers [][]network.HostPort)

	// Migrate takes an existing agent config and applies the given
	// parameters to change it.
	//
	// Only non-empty fields in newParams are used
	// to change existing config settings. All changes are written
	// atomically. UpgradedToVersion cannot be changed here, because
	// Migrate is most likely called during an upgrade, so it will be
	// changed at the end of the upgrade anyway, if successful.
	//
	// Migrate does not actually write the new configuration.
	//
	// Note that if the configuration file moves location,
	// (if DataDir is set), the the caller is responsible for removing
	// the old configuration.
	Migrate(MigrateParams) error

	// SetStateServingInfo sets the information needed
	// to run a state server
	SetStateServingInfo(info params.StateServingInfo)
}

type ConfigWriter interface {
	// Write writes the agent configuration.
	Write() error
}

type ConfigSetter interface {
	Config
	ConfigSetterOnly
}

type ConfigSetterWriter interface {
	Config
	ConfigSetterOnly
	ConfigWriter
}

// MigrateParams holds agent config values to change in a
// Migrate call. Empty fields will be ignored. DeleteValues
// specifies a list of keys to delete.
type MigrateParams struct {
	DataDir      string
	LogDir       string
	Jobs         []params.MachineJob
	DeleteValues []string
	Values       map[string]string
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
	dataDir           string
	logDir            string
	tag               names.Tag
	nonce             string
	jobs              []params.MachineJob
	upgradedToVersion version.Number
	caCert            string
	stateDetails      *connectionDetails
	apiDetails        *connectionDetails
	oldPassword       string
	servingInfo       *params.StateServingInfo
	values            map[string]string
	preferIPv6        bool
}

type AgentConfigParams struct {
	DataDir           string
	LogDir            string
	Jobs              []params.MachineJob
	UpgradedToVersion version.Number
	Tag               names.Tag
	Password          string
	Nonce             string
	StateAddresses    []string
	APIAddresses      []string
	CACert            string
	Values            map[string]string
	PreferIPv6        bool
}

// NewAgentConfig returns a new config object suitable for use for a
// machine or unit agent.
func NewAgentConfig(configParams AgentConfigParams) (ConfigSetterWriter, error) {
	if configParams.DataDir == "" {
		return nil, errors.Trace(requiredError("data directory"))
	}
	logDir := DefaultLogDir
	if configParams.LogDir != "" {
		logDir = configParams.LogDir
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
	if len(configParams.CACert) == 0 {
		return nil, errors.Trace(requiredError("CA certificate"))
	}
	// Note that the password parts of the state and api information are
	// blank.  This is by design.
	config := &configInternal{
		logDir:            logDir,
		dataDir:           configParams.DataDir,
		jobs:              configParams.Jobs,
		upgradedToVersion: configParams.UpgradedToVersion,
		tag:               configParams.Tag,
		nonce:             configParams.Nonce,
		caCert:            configParams.CACert,
		oldPassword:       configParams.Password,
		values:            configParams.Values,
		preferIPv6:        configParams.PreferIPv6,
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
	config.configFilePath = ConfigPath(config.dataDir, config.tag)
	return config, nil
}

// NewStateMachineConfig returns a configuration suitable for
// a machine running the state server.
func NewStateMachineConfig(configParams AgentConfigParams, serverInfo params.StateServingInfo) (ConfigSetterWriter, error) {
	if serverInfo.Cert == "" {
		return nil, errors.Trace(requiredError("state server cert"))
	}
	if serverInfo.PrivateKey == "" {
		return nil, errors.Trace(requiredError("state server key"))
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

// Dir returns the agent-specific data directory.
func Dir(dataDir string, tag names.Tag) string {
	// Note: must use path, not filepath, as this
	// function is used by the client on Windows.
	return path.Join(dataDir, "agents", tag.String())
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

	// Try to read the legacy format file.
	dir := filepath.Dir(configFilePath)
	legacyFormatPath := filepath.Join(dir, legacyFormatFilename)
	formatBytes, err := ioutil.ReadFile(legacyFormatPath)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("cannot read format file: %v", err)
	}
	formatData := string(formatBytes)
	if err == nil {
		// It exists, so unmarshal with a legacy formatter.
		// Drop the format prefix to leave the version only.
		if !strings.HasPrefix(formatData, legacyFormatPrefix) {
			return nil, fmt.Errorf("malformed agent config format %q", formatData)
		}
		format, err = getFormatter(strings.TrimPrefix(formatData, legacyFormatPrefix))
		if err != nil {
			return nil, err
		}
		config, err = format.unmarshal(configData)
	} else {
		// Does not exist, just parse the data.
		format, config, err = parseConfigData(configData)
	}
	if err != nil {
		return nil, err
	}
	logger.Debugf("read agent config, format %q", format.version())
	config.configFilePath = configFilePath
	if format != currentFormat {
		// Migrate from a legacy format to the new one.
		err := config.Write()
		if err != nil {
			return nil, fmt.Errorf("cannot migrate %s agent config to %s: %v", format.version(), currentFormat.version(), err)
		}
		logger.Debugf("migrated agent config from %s to %s", format.version(), currentFormat.version())
		err = os.Remove(legacyFormatPath)
		if err != nil && !os.IsNotExist(err) {
			return nil, fmt.Errorf("cannot remove legacy format file %q: %v", legacyFormatPath, err)
		}
	}
	return config, nil
}

func (c0 *configInternal) Clone() Config {
	c1 := *c0
	// Deep copy only fields which may be affected
	// by ConfigSetter methods.
	c1.stateDetails = c0.stateDetails.clone()
	c1.apiDetails = c0.apiDetails.clone()
	c1.jobs = append([]params.MachineJob{}, c0.jobs...)
	c1.values = make(map[string]string, len(c0.values))
	for key, val := range c0.values {
		c1.values[key] = val
	}
	return &c1
}

func (config *configInternal) Migrate(newParams MigrateParams) error {
	if newParams.DataDir != "" {
		config.dataDir = newParams.DataDir
		config.configFilePath = ConfigPath(config.dataDir, config.tag)
	}
	if newParams.LogDir != "" {
		config.logDir = newParams.LogDir
	}
	if len(newParams.Jobs) > 0 {
		config.jobs = make([]params.MachineJob, len(newParams.Jobs))
		copy(config.jobs, newParams.Jobs)
	}
	for _, key := range newParams.DeleteValues {
		delete(config.values, key)
	}
	for key, value := range newParams.Values {
		if config.values == nil {
			config.values = make(map[string]string)
		}
		config.values[key] = value
	}
	if err := config.check(); err != nil {
		return fmt.Errorf("migrated agent config is invalid: %v", err)
	}
	return nil
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
		addr := network.SelectInternalHostPort(serverHostPorts, false)
		if addr != "" {
			addrs = append(addrs, addr)
		}
	}
	c.apiDetails.addresses = addrs
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
	return c.dataDir
}

func (c *configInternal) LogDir() string {
	return c.logDir
}

func (c *configInternal) SystemIdentityPath() string {
	return filepath.Join(c.dataDir, SystemIdentity)
}

func (c *configInternal) Jobs() []params.MachineJob {
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

func (c *configInternal) PreferIPv6() bool {
	return c.preferIPv6
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

func (c *configInternal) Dir() string {
	return Dir(c.dataDir, c.tag)
}

func (c *configInternal) check() error {
	if c.stateDetails == nil && c.apiDetails == nil {
		return errors.Trace(requiredError("state or API addresses"))
	}
	if c.stateDetails != nil {
		if err := checkAddrs(c.stateDetails.addresses, "state server address"); err != nil {
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

func (c *configInternal) WriteCommands() ([]string, error) {
	data, err := c.fileContents()
	if err != nil {
		return nil, err
	}
	commands := []string{"mkdir -p " + utils.ShQuote(c.Dir())}
	commands = append(commands, writeFileCommands(c.File(agentConfigFilename), data, 0600)...)
	return commands, nil
}

func (c *configInternal) APIInfo() *api.Info {
	servingInfo, isStateServer := c.StateServingInfo()
	addrs := c.apiDetails.addresses
	if isStateServer {
		port := servingInfo.APIPort
		localAPIAddr := net.JoinHostPort("localhost", strconv.Itoa(port))
		if c.preferIPv6 {
			localAPIAddr = net.JoinHostPort("::1", strconv.Itoa(port))
		}
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
	}
}

func (c *configInternal) MongoInfo() (info *authentication.MongoInfo, ok bool) {
	ssi, ok := c.StateServingInfo()
	if !ok {
		return nil, false
	}
	addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(ssi.StatePort))
	if c.preferIPv6 {
		addr = net.JoinHostPort("::1", strconv.Itoa(ssi.StatePort))
	}
	return &authentication.MongoInfo{
		Info: mongo.Info{
			Addrs:  []string{addr},
			CACert: c.caCert,
		},
		Password: c.stateDetails.password,
		Tag:      c.tag,
	}, true
}

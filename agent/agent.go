// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/errgo/errgo"
	"github.com/juju/loggo"

	"launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/utils"
	"launchpad.net/juju-core/version"
)

var logger = loggo.GetLogger("juju.agent")

// DefaultLogDir defines the default log directory for juju agents.
// It's defined as a variable so it could be overridden in tests.
var DefaultLogDir = "/var/log/juju"

// DefaultDataDir defines the default data directory for juju agents.
// It's defined as a variable so it could be overridden in tests.
var DefaultDataDir = "/var/lib/juju"

const (
	LxcBridge        = "LXC_BRIDGE"
	ProviderType     = "PROVIDER_TYPE"
	ContainerType    = "CONTAINER_TYPE"
	Namespace        = "NAMESPACE"
	StorageDir       = "STORAGE_DIR"
	StorageAddr      = "STORAGE_ADDR"
	AgentServiceName = "AGENT_SERVICE_NAME"
	MongoServiceName = "MONGO_SERVICE_NAME"
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

	// Jobs returns a list of MachineJobs that need to run.
	Jobs() []params.MachineJob

	// Tag returns the tag of the entity on whose behalf the state connection
	// will be made.
	Tag() string

	// Password returns the agent's password.
	Password() string

	// Dir returns the agent's directory.
	Dir() string

	// Nonce returns the nonce saved when the machine was provisioned
	// TODO: make this one of the key/value pairs.
	Nonce() string

	// CACert returns the CA certificate that is used to validate the state or
	// API servier's certificate.
	CACert() []byte

	// OpenAPI tries to connect to an API end-point.  If a non-empty
	// newPassword is returned, OpenAPI will have written the configuration
	// with the new password; the caller should set the connecting entity's
	// password accordingly.
	OpenAPI(dialOpts api.DialOpts) (st *api.State, newPassword string, err error)

	// APIAddresses returns the addresses needed to connect to the api server
	APIAddresses() ([]string, error)

	// OpenState tries to open a direct connection to the state database using
	// the given Conf.
	OpenState(policy state.Policy) (*state.State, error)

	// Write writes the agent configuration.
	Write() error

	// WriteCommands returns shell commands to write the agent configuration.
	// It returns an error if the configuration does not have all the right
	// elements.
	WriteCommands() ([]string, error)

	// APIServerDetails returns the details needed to run an API server.
	APIServerDetails() (port int, cert, key []byte)

	// UpgradedToVersion returns the version for which all upgrade steps have been
	// successfully run, which is also the same as the initially deployed version.
	UpgradedToVersion() version.Number

	// WriteUpgradedToVersion updates the config's UpgradedToVersion and writes
	// the new agent configuration.
	WriteUpgradedToVersion(newVersion version.Number) error

	// Value returns the value associated with the key, or an empty string if
	// the key is not found.
	Value(key string) string

	// SetValue updates the value for the specified key.
	SetValue(key, value string)

	// StateManager reports if this config is for a machine that should manage
	// state.
	StateManager() bool

	// StatePort returns the port for connecting to the state db.
	StatePort() int

	// StateAddresses returns the list of addresses for connecting to the state db.
	StateAddresses() []string

	Clone() Config

	StateInitializer
}

// MigrateConfigParams holds agent config values to change in a
// MigrateConfig call. Empty fields will be ignored. DeleteValues
// specifies a list of keys to delete.
type MigrateConfigParams struct {
	DataDir      string
	LogDir       string
	Jobs         []params.MachineJob
	DeleteValues []string
	Values       map[string]string
}

// MigrateConfig takes an existing agent config and applies the given
// newParams selectively. Only non-empty fields in newParams are used
// to change existing config settings. All changes are written
// atomically. UpgradedToVersion cannot be changed here, because
// MigrateConfig is most likely called during an upgrade, so it will be
// changed at the end of the upgrade anyway, if successful.
func MigrateConfig(currentConfig Config, newParams MigrateConfigParams) error {
	configMutex.Lock()
	defer configMutex.Unlock()
	config := currentConfig.(*configInternal)

	if newParams.DataDir != "" {
		config.dataDir = newParams.DataDir
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
	oldConfigFile := config.configFilePath
	config.configFilePath = ConfigPath(config.dataDir, config.tag)
	if err := config.write(); err != nil {
		return fmt.Errorf("cannot migrate agent config: %v", err)
	}
	if oldConfigFile != config.configFilePath && oldConfigFile != "" {
		err := os.Remove(oldConfigFile)
		if err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("cannot remove old agent config %q: %v", oldConfigFile, err)
		}
	}
	return nil
}

// Ensure that the configInternal struct implements the Config interface.
var _ Config = (*configInternal)(nil)

// The configMutex should be locked before any writing to disk during
// the write commands, and unlocked when the writing is complete. This
// process wide lock should stop any unintended concurrent writes.
// This may happen when multiple go-routines may be adding things to
// the agent config, and wanting to persist them to disk. To ensure
// that the correct data is written to disk, the mutex should be
// locked prior to generating any disk state. This way calls that
// might get interleaved would always write the most recent state to
// disk. Since we have different agent configs for each agent, and
// there is only one process for each agent, a simple mutex is enough
// for concurrency. The mutex should also be locked around any access
// to mutable values, either setting or getting. The only mutable
// value is the values map. Retrieving and setting values here are
// protected by the mutex. New mutating methods should also be
// synchronized using this mutex. Config is essentially a singleton
// implementation, having a non-constructable-in-a-normal-way backing
// type configInternal.
var configMutex sync.Mutex

type connectionDetails struct {
	addresses []string
	password  string
}

type configInternal struct {
	configFilePath    string
	dataDir           string
	logDir            string
	tag               string
	nonce             string
	jobs              []params.MachineJob
	upgradedToVersion version.Number
	caCert            []byte
	stateDetails      *connectionDetails
	apiDetails        *connectionDetails
	oldPassword       string
	stateServerCert   []byte
	stateServerKey    []byte
	apiPort           int
	statePort         int
	values            map[string]string
}

type AgentConfigParams struct {
	DataDir           string
	LogDir            string
	Jobs              []params.MachineJob
	UpgradedToVersion version.Number
	Tag               string
	Password          string
	Nonce             string
	StateAddresses    []string
	APIAddresses      []string
	CACert            []byte
	Values            map[string]string

	// These are only used by agents that are going to be managing state.
	StateServerCert []byte
	StateServerKey  []byte
	StatePort       int
	APIPort         int
}

// NewAgentConfig returns a new config object suitable for use for a
// machine or unit agent.
func NewAgentConfig(configParams AgentConfigParams) (Config, error) {
	if configParams.DataDir == "" {
		return nil, errgo.Trace(requiredError("data directory"))
	}
	logDir := DefaultLogDir
	if configParams.LogDir != "" {
		logDir = configParams.LogDir
	}
	if configParams.Tag == "" {
		return nil, errgo.Trace(requiredError("entity tag"))
	}
	if configParams.UpgradedToVersion == version.Zero {
		return nil, errgo.Trace(requiredError("upgradedToVersion"))
	}
	if configParams.Password == "" {
		return nil, errgo.Trace(requiredError("password"))
	}
	if len(configParams.CACert) == 0 {
		return nil, errgo.Trace(requiredError("CA certificate"))
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
func NewStateMachineConfig(configParams AgentConfigParams) (Config, error) {
	if configParams.StateServerCert == nil {
		return nil, errgo.Trace(requiredError("state server cert"))
	}
	if configParams.StateServerKey == nil {
		return nil, errgo.Trace(requiredError("state server key"))
	}
	config0, err := NewAgentConfig(configParams)
	if err != nil {
		return nil, err
	}
	config := config0.(*configInternal)
	config.stateServerCert = configParams.StateServerCert
	config.stateServerKey = configParams.StateServerKey
	config.apiPort = configParams.APIPort
	config.statePort = configParams.StatePort
	return config, nil
}

// Dir returns the agent-specific data directory.
func Dir(dataDir, agentName string) string {
	// Note: must use path, not filepath, as this
	// function is used by the client on Windows.
	return path.Join(dataDir, "agents", agentName)
}

// ConfigPath returns the full path to the agent config file.
// NOTE: Delete this once all agents accept --config instead
// of --data-dir - it won't be needed anymore.
func ConfigPath(dataDir, agentName string) string {
	return filepath.Join(Dir(dataDir, agentName), agentConfigFilename)
}

// ReadConf reads configuration data from the given location.
func ReadConf(configFilePath string) (Config, error) {
	// Even though the ReadConf is done at the start of the agent loading, and
	// that this should not be called more than once by an agent, I feel that
	// not locking the mutex that is used to protect writes is wrong.
	configMutex.Lock()
	defer configMutex.Unlock()
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
		err := config.write()
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

func (c *configInternal) Jobs() []params.MachineJob {
	return c.jobs
}

func (c *configInternal) Nonce() string {
	return c.nonce
}

func (c *configInternal) UpgradedToVersion() version.Number {
	return c.upgradedToVersion
}

func (c *configInternal) CACert() []byte {
	// Give the caller their own copy of the cert to avoid any possibility of
	// modifying the config's copy.
	result := append([]byte{}, c.caCert...)
	return result
}

func (c *configInternal) Value(key string) string {
	configMutex.Lock()
	defer configMutex.Unlock()
	return c.values[key]
}

func (c *configInternal) SetValue(key, value string) {
	configMutex.Lock()
	defer configMutex.Unlock()
	if value == "" {
		delete(c.values, key)
	} else {
		c.values[key] = value
	}
}

func (c *configInternal) APIServerDetails() (port int, cert, key []byte) {
	return c.apiPort, c.stateServerCert, c.stateServerKey
}

func (c *configInternal) APIAddresses() ([]string, error) {
	if c.apiDetails == nil {
		return []string{}, errgo.New("No apidetails in config")
	}
	return append([]string{}, c.apiDetails.addresses...), nil
}

func (c *configInternal) Tag() string {
	return c.tag
}

func (c *configInternal) Dir() string {
	return Dir(c.dataDir, c.tag)
}

func (c *configInternal) StateManager() bool {
	return c.caCert != nil
}

func (c *configInternal) StatePort() int {
	return c.statePort
}

func (c *configInternal) StateAddresses() []string {
	return c.stateDetails.addresses
}

func (c *configInternal) Clone() Config {
	// copy the value
	c2 := *c

	// now overwrite all the pointer, slice, and map stuff inside with deep-copies
	copy(c2.caCert, c.caCert)
	stateDetails := *c.stateDetails
	c2.stateDetails = &stateDetails
	copy(c2.stateDetails.addresses, c.stateDetails.addresses)
	apiDetails := *c.apiDetails
	c2.apiDetails = &apiDetails
	copy(c2.apiDetails.addresses, c.apiDetails.addresses)
	copy(c2.stateServerCert, c.stateServerCert)
	copy(c2.stateServerKey, c.stateServerKey)
	c2.values = map[string]string{}
	for key, val := range c.values {
		c2.values[key] = val
	}
	return &c2
}

func (c *configInternal) check() error {
	if c.stateDetails == nil && c.apiDetails == nil {
		return errgo.Trace(requiredError("state or API addresses"))
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
		return errgo.Trace(requiredError(what))
	}
	for _, a := range addrs {
		if !validAddr.MatchString(a) {
			return errgo.New("invalid %s %q", what, a)
		}
	}
	return nil
}

// writeNewPassword generates a new password and writes
// the configuration with it in.
func (c *configInternal) writeNewPassword() (string, error) {
	newPassword, err := utils.RandomPassword()
	if err != nil {
		return "", err
	}
	// Make a copy of the configuration so that if we fail
	// to write the configuration file, the configuration will
	// still be valid.
	other := *c
	if c.stateDetails != nil {
		stateDetails := *c.stateDetails
		stateDetails.password = newPassword
		other.stateDetails = &stateDetails
	}
	if c.apiDetails != nil {
		apiDetails := *c.apiDetails
		apiDetails.password = newPassword
		other.apiDetails = &apiDetails
	}
	logger.Debugf("writing configuration file")
	if err := other.write(); err != nil {
		return "", err
	}
	*c = other
	return newPassword, nil
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

// write is the internal implementation of c.Write().
func (c *configInternal) write() error {
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

func (c *configInternal) Write() error {
	// Lock is taken prior to generating any content to write.
	configMutex.Lock()
	defer configMutex.Unlock()
	return c.write()
}

func (c *configInternal) WriteUpgradedToVersion(newVersion version.Number) error {
	configMutex.Lock()
	defer configMutex.Unlock()
	originalVersion := c.upgradedToVersion
	c.upgradedToVersion = newVersion
	err := c.write()
	if err != nil {
		// We don't want to retain the new version if there's been an error writing the file.
		c.upgradedToVersion = originalVersion
	}
	return err
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

func (c *configInternal) OpenAPI(dialOpts api.DialOpts) (st *api.State, newPassword string, err error) {
	configMutex.Lock()
	defer configMutex.Unlock()
	info := api.Info{
		Addrs:    c.apiDetails.addresses,
		Password: c.apiDetails.password,
		CACert:   c.caCert,
		Tag:      c.tag,
		Nonce:    c.nonce,
	}
	if info.Password != "" {
		st, err := api.Open(&info, dialOpts)
		if err == nil {
			return st, "", nil
		}
		if !params.IsCodeUnauthorized(err) {
			return nil, "", err
		}
		// Access isn't authorized even though we have a password
		// This can happen if we crash after saving the
		// password but before changing it, so we'll try again
		// with the old password.
	}
	info.Password = c.oldPassword
	st, err = api.Open(&info, dialOpts)
	if err != nil {
		return nil, "", err
	}

	// We've succeeded in connecting with the old password, so
	// we can now change it to something more private.
	password, err := c.writeNewPassword()
	if err != nil {
		st.Close()
		return nil, "", err
	}
	return st, password, nil
}

func (c *configInternal) Password() string {
	return c.stateDetails.password
}

func (c *configInternal) OpenState(policy state.Policy) (*state.State, error) {
	info := state.Info{
		Addrs:    c.stateDetails.addresses,
		Password: c.stateDetails.password,
		CACert:   c.caCert,
		Tag:      c.tag,
	}
	if info.Password != "" {
		st, err := state.Open(&info, state.DefaultDialOpts(), policy)
		if err == nil {
			return st, nil
		}
		// TODO(rog) remove this fallback behaviour when
		// all initial connections are via the API.
		if !errors.IsUnauthorizedError(err) {
			return nil, err
		}
	}
	info.Password = c.oldPassword
	return state.Open(&info, state.DefaultDialOpts(), policy)
}

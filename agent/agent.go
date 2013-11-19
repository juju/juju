// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	"fmt"
	"path"
	"regexp"
	"sync"

	"launchpad.net/loggo"

	"launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/utils"
)

var logger = loggo.GetLogger("juju.agent")

const (
	LxcBridge         = "LXC_BRIDGE"
	ProviderType      = "PROVIDER_TYPE"
	ContainerType     = "CONTAINER_TYPE"
	StorageDir        = "STORAGE_DIR"
	StorageAddr       = "STORAGE_ADDR"
	SharedStorageDir  = "SHARED_STORAGE_DIR"
	SharedStorageAddr = "SHARED_STORAGE_ADDR"
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

	// Tag returns the tag of the entity on whose behalf the state connection
	// will be made.
	Tag() string

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

	//APIAddresses returns the addresses needed to connect to the api server
	APIAddresses() []string

	// OpenState tries to open a direct connection to the state database using
	// the given Conf.
	OpenState() (*state.State, error)

	// Write writes the agent configuration.
	Write() error

	// WriteCommands returns shell commands to write the agent configuration.
	// It returns an error if the configuration does not have all the right
	// elements.
	WriteCommands() ([]string, error)

	// APIServerDetails returns the details needed to run an API server.
	APIServerDetails() (port int, cert, key []byte)

	// Value returns the value associated with the key, or an empty string if
	// the key is not found.
	Value(key string) string

	// SetValue updates the value for the specified key.
	SetValue(key, value string)

	StateInitializer
}

// Ensure that the configInternal struct implements the Config interface.
var _ Config = (*configInternal)(nil)

// The configMutex should be locked before any writing to disk during the
// write commands, and unlocked when the writing is complete.  This process
// wide lock should stop any unintended concurrent writes.  This may happen
// when multiple go-routines may be adding things to the agent config, and
// wanting to persist them to disk. To ensure that the correct data is written
// to disk, the mutex should be locked prior to generating any disk state.
// This way calls that might get interleaved would always write the most
// recent state to disk.  Since we have different agent configs for each
// agent, and there is only one process for each agent, a simple mutex is
// enough for concurrency.  The mutex should also be locked around any access
// to mutable values, either setting or getting.  The only mutable value is
// the values map.  Retrieving and setting values here are protected by the
// mutex.  New mutating methods should also be synchronized using this mutex.
var configMutex sync.Mutex

type connectionDetails struct {
	addresses []string
	password  string
}

type configInternal struct {
	dataDir         string
	tag             string
	nonce           string
	caCert          []byte
	stateDetails    *connectionDetails
	apiDetails      *connectionDetails
	oldPassword     string
	stateServerCert []byte
	stateServerKey  []byte
	apiPort         int
	values          map[string]string
}

type AgentConfigParams struct {
	DataDir        string
	Tag            string
	Password       string
	Nonce          string
	StateAddresses []string
	APIAddresses   []string
	CACert         []byte
	Values         map[string]string
}

// NewAgentConfig returns a new config object suitable for use for a
// machine or unit agent.
func NewAgentConfig(params AgentConfigParams) (Config, error) {
	if params.DataDir == "" {
		return nil, requiredError("data directory")
	}
	if params.Tag == "" {
		return nil, requiredError("entity tag")
	}
	if params.Password == "" {
		return nil, requiredError("password")
	}
	if params.CACert == nil {
		return nil, requiredError("CA certificate")
	}
	// Note that the password parts of the state and api information are
	// blank.  This is by design.
	config := &configInternal{
		dataDir:     params.DataDir,
		tag:         params.Tag,
		nonce:       params.Nonce,
		caCert:      params.CACert,
		oldPassword: params.Password,
		values:      params.Values,
	}
	if len(params.StateAddresses) > 0 {
		config.stateDetails = &connectionDetails{
			addresses: params.StateAddresses,
		}
	}
	if len(params.APIAddresses) > 0 {
		config.apiDetails = &connectionDetails{
			addresses: params.APIAddresses,
		}
	}
	if err := config.check(); err != nil {
		return nil, err
	}
	if config.values == nil {
		config.values = make(map[string]string)
	}
	return config, nil
}

type StateMachineConfigParams struct {
	AgentConfigParams
	StateServerCert []byte
	StateServerKey  []byte
	StatePort       int
	APIPort         int
}

// NewStateMachineConfig returns a configuration suitable for
// a machine running the state server.
func NewStateMachineConfig(params StateMachineConfigParams) (Config, error) {
	if params.StateServerCert == nil {
		return nil, requiredError("state server cert")
	}
	if params.StateServerKey == nil {
		return nil, requiredError("state server key")
	}
	config0, err := NewAgentConfig(params.AgentConfigParams)
	if err != nil {
		return nil, err
	}
	config := config0.(*configInternal)
	config.stateServerCert = params.StateServerCert
	config.stateServerKey = params.StateServerKey
	config.apiPort = params.APIPort
	return config, nil
}

// Dir returns the agent-specific data directory.
func Dir(dataDir, agentName string) string {
	return path.Join(dataDir, "agents", agentName)
}

// ReadConf reads configuration data for the given
// entity from the given data directory.
func ReadConf(dataDir, tag string) (Config, error) {
	// Even though the ReadConf is done at the start of the agent loading, and
	// that this should not be called more than once by an agent, I feel that
	// not locking the mutex that is used to protect writes is wrong.
	configMutex.Lock()
	defer configMutex.Unlock()
	dir := Dir(dataDir, tag)
	format, err := readFormat(dir)
	if err != nil {
		return nil, err
	}
	logger.Debugf("Reading agent config, format: %s", format)
	formatter, err := newFormatter(format)
	if err != nil {
		return nil, err
	}
	config, err := formatter.read(dir)
	if err != nil {
		return nil, err
	}
	config.dataDir = dataDir
	if err := config.check(); err != nil {
		return nil, err
	}

	if format != currentFormat {
		// Migrate the config to the new format.
		currentFormatter.migrate(config)
		// Write the content out in the new format.
		if err := currentFormatter.write(config); err != nil {
			logger.Errorf("cannot write the agent config in format %s: %v", currentFormat, err)
			return nil, err
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

func (c *configInternal) Nonce() string {
	return c.nonce
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

func (c *configInternal) APIAddresses() []string {
	return c.apiDetails.addresses
}

func (c *configInternal) Tag() string {
	return c.tag
}

func (c *configInternal) Dir() string {
	return Dir(c.dataDir, c.tag)
}

func (c *configInternal) check() error {
	if c.stateDetails == nil && c.apiDetails == nil {
		return requiredError("state or API addresses")
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
		return requiredError(what)
	}
	for _, a := range addrs {
		if !validAddr.MatchString(a) {
			return fmt.Errorf("invalid %s %q", what, a)
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
	if err := other.Write(); err != nil {
		return "", err
	}
	*c = other
	return newPassword, nil
}

func (c *configInternal) Write() error {
	// Lock is taken prior to generating any content to write.
	configMutex.Lock()
	defer configMutex.Unlock()
	return currentFormatter.write(c)
}

func (c *configInternal) WriteCommands() ([]string, error) {
	return currentFormatter.writeCommands(c)
}

func (c *configInternal) OpenAPI(dialOpts api.DialOpts) (st *api.State, newPassword string, err error) {
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

func (c *configInternal) OpenState() (*state.State, error) {
	info := state.Info{
		Addrs:    c.stateDetails.addresses,
		Password: c.stateDetails.password,
		CACert:   c.caCert,
		Tag:      c.tag,
	}
	if info.Password != "" {
		st, err := state.Open(&info, state.DefaultDialOpts())
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
	return state.Open(&info, state.DefaultDialOpts())
}

// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	"fmt"
	"path"
	"regexp"

	"launchpad.net/loggo"

	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/utils"
)

var logger = loggo.GetLogger("juju.agent")

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

	// OpenAPI tries to connect to an API end-point.  If a non-empty
	// newPassword is returned, the password used to connect to the state
	// should be changed accordingly - the caller should set the entity's
	// password accordingly.
	OpenAPI(dialOpts api.DialOpts) (st *api.State, newPassword string, err error)

	// OpenState tries to open a direct connection to the state database using
	// the given Conf.
	OpenState() (*state.State, error)

	// Write writes the agent configuration.
	Write() error

	// WriteCommands returns shell commands to write the agent configuration.
	// It returns an error if the configuration does not have all the right
	// elements.
	WriteCommands() ([]string, error)

	// GenerateNewPassword creates a new random password and saves this.  The
	// new password string is returned.
	GenerateNewPassword() (string, error)

	// PasswordHash returns a hash of the password that is stored for state and
	// api connections.
	PasswordHash() string

	// APIServerDetails returns the details needed to run an API server.
	APIServerDetails() (port int, cert, key []byte)
}

// Ensure that the configInternal struct implements the Config interface.
var _ Config = (*configInternal)(nil)

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
}

type AgentConfigParams struct {
	DataDir        string
	Tag            string
	Password       string
	Nonce          string
	StateAddresses []string
	APIAddresses   []string
	CACert         []byte
}

func newConfig(params AgentConfigParams) (*configInternal, error) {
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
	return config, nil
}

// NewAgentConfig returns a new config object suitable for use for a unit
// agent. As the unit agent becomes entirely APIified, we should remove the
// state addresses from here.
func NewAgentConfig(params AgentConfigParams) (Config, error) {
	return newConfig(params)
}

type StateMachineConfigParams struct {
	AgentConfigParams
	StateServerCert []byte
	StateServerKey  []byte
	StatePort       int
	APIPort         int
}

func NewStateMachineConfig(params StateMachineConfigParams) (Config, error) {
	if params.StateServerCert == nil {
		return nil, requiredError("state server cert")
	}
	if params.StateServerKey == nil {
		return nil, requiredError("state server key")
	}
	config, err := newConfig(params.AgentConfigParams)
	if err != nil {
		return nil, err
	}
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
		// Write the content out in the new format.
		if err := currentFormatter.write(config); err != nil {
			logger.Errorf("Problem writing the agent config out in format: %s, %v", currentFormat, err)
			return nil, err
		}
	}

	return config, nil
}

func requiredError(what string) error {
	return fmt.Errorf("%s not found in configuration", what)
}

// File returns the path of the given file in the agent's directory.
func (c *configInternal) File(name string) string {
	return path.Join(c.Dir(), name)
}

func (c *configInternal) DataDir() string {
	return c.dataDir
}

func (c *configInternal) Nonce() string {
	return c.nonce
}

func (c *configInternal) APIServerDetails() (port int, cert, key []byte) {
	return c.apiPort, c.stateServerCert, c.stateServerKey
}

// Tag returns the tag of the entity on whose behalf the state connection will
// be made.
func (c *configInternal) Tag() string {
	return c.tag
}

// Dir returns the agent's directory.
func (c *configInternal) Dir() string {
	return Dir(c.dataDir, c.tag)
}

// Check checks that the configuration has all the required elements.
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

func (c *configInternal) PasswordHash() string {
	var password string
	if c.stateDetails == nil {
		password = c.apiDetails.password
	} else {
		password = c.stateDetails.password
	}
	return utils.PasswordHash(password)
}

func (c *configInternal) GenerateNewPassword() (string, error) {
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

	if err := other.Write(); err != nil {
		return "", err
	}
	*c = other
	return newPassword, nil
}

// Write writes the agent configuration.
func (c *configInternal) Write() error {
	return currentFormatter.write(c)
}

// WriteCommands returns shell commands to write the agent
// configuration.  It returns an error if the configuration does not
// have all the right elements.
func (c *configInternal) WriteCommands() ([]string, error) {
	return currentFormatter.writeCommands(c)
}

// OpenAPI tries to open the state using the given Conf.  If it
// returns a non-empty newPassword, the password used to connect
// to the state should be changed accordingly - the caller should write the
// configuration with StateInfo.Password set to newPassword, then
// set the entity's password accordingly.
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
		if params.ErrCode(err) != params.CodeUnauthorized {
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
	password, err := c.GenerateNewPassword()
	if err != nil {
		st.Close()
		return nil, "", err
	}
	return st, password, nil
}

// OpenState tries to open the state using the given Conf.
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

func InitialStateConfiguration(agentConfig Config, cfg *config.Config, timeout state.DialOpts) (*state.State, error) {
	c := agentConfig.(*configInternal)
	info := state.Info{
		Addrs:  c.stateDetails.addresses,
		CACert: c.caCert,
	}
	logger.Debugf("initializing address %v", info.Addrs)
	st, err := state.Initialize(&info, cfg, timeout)
	if err != nil {
		if errors.IsUnauthorizedError(err) {
			logger.Errorf("unauthorized: %v", err)
		} else {
			logger.Errorf("failed to initialize state: %v", err)
		}
		return nil, err
	}
	logger.Debugf("state initialized")

	if err := bootstrapUsers(st, cfg, c.oldPassword); err != nil {
		st.Close()
		return nil, err
	}
	return st, nil
}

// bootstrapUsers creates the initial admin user for the database, and sets
// the initial password.
func bootstrapUsers(st *state.State, cfg *config.Config, passwordHash string) error {
	logger.Debugf("adding admin user")
	// Set up initial authentication.
	u, err := st.AddUser("admin", "")
	if err != nil {
		return err
	}

	// Note that at bootstrap time, the password is set to
	// the hash of its actual value. The first time a client
	// connects to mongo, it changes the mongo password
	// to the original password.
	logger.Debugf("setting password hash for admin user")
	if err := u.SetPasswordHash(passwordHash); err != nil {
		return err
	}
	if err := st.SetAdminMongoPassword(passwordHash); err != nil {
		return err
	}
	return nil
}

// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"regexp"

	"launchpad.net/goyaml"
	"launchpad.net/loggo"

	"launchpad.net/juju-core/agent/tools"
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

// conf holds information for a given agent.
type conf struct {
	// DataDir specifies the path of the data directory used by all
	// agents
	dataDir string

	// StateServerCert and StateServerKey hold the state server
	// certificate and private key in PEM format.
	StateServerCert []byte `yaml:",omitempty"`
	StateServerKey  []byte `yaml:",omitempty"`

	StatePort int `yaml:",omitempty"`
	APIPort   int `yaml:",omitempty"`

	// OldPassword specifies a password that should be
	// used to connect to the state if StateInfo.Password
	// is blank or invalid.
	OldPassword string

	// MachineNonce is set at provisioning/bootstrap time and used to
	// ensure the agent is running on the correct instance.
	MachineNonce string

	// StateInfo specifies how the agent should connect to the
	// state.  The password may be empty if an old password is
	// specified, or when bootstrapping.
	StateInfo *state.Info `yaml:",omitempty"`

	// OldAPIPassword specifies a password that should
	// be used to connect to the API if APIInfo.Password
	// is blank or invalid.
	OldAPIPassword string

	// APIInfo specifies how the agent should connect to the
	// state through the API.
	APIInfo *api.Info `yaml:",omitempty"`
}

// Ensure that the configInternal struct implements the Config interface.
// var _ Config = (*configInternal)(nil)

type configInternal struct {
	dataDir         string
	tag             string
	password        string
	oldPassword     string
	nonce           string
	stateAddresses  []string
	apiAddresses    []string
	caCert          []byte
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

func newConfig(params AgentConfigParams) (*conf, error) {
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
	var stateInfo *state.Info
	if len(params.StateAddresses) > 0 {
		stateInfo = &state.Info{
			Addrs:  params.StateAddresses,
			Tag:    params.Tag,
			CACert: params.CACert,
		}
	}
	var apiInfo *api.Info
	if len(params.APIAddresses) > 0 {
		apiInfo = &api.Info{
			Addrs:  params.APIAddresses,
			Tag:    params.Tag,
			CACert: params.CACert,
		}
	}
	conf := &conf{
		dataDir:      params.DataDir,
		OldPassword:  params.Password,
		StateInfo:    stateInfo,
		APIInfo:      apiInfo,
		MachineNonce: params.Nonce,
	}
	if err := conf.check(); err != nil {
		return nil, err
	}
	return conf, nil
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
	conf, err := newConfig(params.AgentConfigParams)
	if err != nil {
		return nil, err
	}
	conf.StateServerCert = params.StateServerCert
	conf.StateServerKey = params.StateServerKey
	conf.StatePort = params.StatePort
	conf.APIPort = params.APIPort
	return conf, nil
}

// ReadConf reads configuration data for the given
// entity from the given data directory.
func ReadConf(dataDir, tag string) (Config, error) {
	dir := tools.Dir(dataDir, tag)
	data, err := ioutil.ReadFile(path.Join(dir, "agent.conf"))
	if err != nil {
		return nil, err
	}
	var c conf
	if err := goyaml.Unmarshal(data, &c); err != nil {
		return nil, err
	}
	c.dataDir = dataDir
	if err := c.check(); err != nil {
		return nil, err
	}
	if c.StateInfo != nil {
		c.StateInfo.Tag = tag
	}
	if c.APIInfo != nil {
		c.APIInfo.Tag = tag
	}
	return &c, nil
}

func requiredError(what string) error {
	return fmt.Errorf("%s not found in configuration", what)
}

// File returns the path of the given file in the agent's directory.
func (c *conf) File(name string) string {
	return path.Join(c.Dir(), name)
}

func (c *conf) confFile() string {
	return c.File("agent.conf")
}

func (c *conf) DataDir() string {
	return c.dataDir
}

func (c *conf) Nonce() string {
	return c.MachineNonce
}

func (c *conf) APIServerDetails() (port int, cert, key []byte) {
	return c.APIPort, c.StateServerCert, c.StateServerKey
}

// Tag returns the tag of the entity on whose behalf the state connection will
// be made.
func (c *conf) Tag() string {
	if c.StateInfo != nil {
		return c.StateInfo.Tag
	}
	return c.APIInfo.Tag
}

// Dir returns the agent's directory.
func (c *conf) Dir() string {
	return tools.Dir(c.dataDir, c.Tag())
}

// Check checks that the configuration has all the required elements.
func (c *conf) check() error {
	if c.StateInfo == nil && c.APIInfo == nil {
		return requiredError("state or API addresses")
	}
	if c.StateInfo != nil {
		if err := checkAddrs(c.StateInfo.Addrs, "state server address"); err != nil {
			return err
		}
	}
	// TODO(rog) make APIInfo mandatory
	if c.APIInfo != nil {
		if err := checkAddrs(c.APIInfo.Addrs, "API server address"); err != nil {
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

func (c *conf) PasswordHash() string {
	var password string
	if c.StateInfo == nil {
		password = c.APIInfo.Password
	} else {
		password = c.StateInfo.Password
	}
	return utils.PasswordHash(password)
}

func (c *conf) GenerateNewPassword() (string, error) {
	newPassword, err := utils.RandomPassword()
	if err != nil {
		return "", err
	}
	// Make a copy of the configuration so that if we fail
	// to write the configuration file, the configuration will
	// still be valid.
	other := *c
	if c.StateInfo != nil {
		stateInfo := *c.StateInfo
		stateInfo.Password = newPassword
		other.StateInfo = &stateInfo
	}
	if c.APIInfo != nil {
		apiInfo := *c.APIInfo
		apiInfo.Password = newPassword
		other.APIInfo = &apiInfo
	}

	if err := other.Write(); err != nil {
		return "", err
	}
	*c = other
	return newPassword, nil
}

// Write writes the agent configuration.
func (c *conf) Write() error {
	if err := c.check(); err != nil {
		return err
	}
	data, err := goyaml.Marshal(c)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(c.Dir(), 0755); err != nil {
		return err
	}
	f := c.File("agent.conf-new")
	if err := ioutil.WriteFile(f, data, 0600); err != nil {
		return err
	}
	if err := os.Rename(f, c.confFile()); err != nil {
		return err
	}
	return nil
}

// WriteCommands returns shell commands to write the agent
// configuration.  It returns an error if the configuration does not
// have all the right elements.
func (c *conf) WriteCommands() ([]string, error) {
	if err := c.check(); err != nil {
		return nil, err
	}
	data, err := goyaml.Marshal(c)
	if err != nil {
		return nil, err
	}
	var cmds []string
	addCmd := func(f string, a ...interface{}) {
		cmds = append(cmds, fmt.Sprintf(f, a...))
	}
	f := utils.ShQuote(c.confFile())
	addCmd("mkdir -p %s", utils.ShQuote(c.Dir()))
	addCmd("install -m %o /dev/null %s", 0600, f)
	addCmd(`printf '%%s\n' %s > %s`, utils.ShQuote(string(data)), f)
	return cmds, nil
}

// OpenAPI tries to open the state using the given Conf.  If it
// returns a non-empty newPassword, the password used to connect
// to the state should be changed accordingly - the caller should write the
// configuration with StateInfo.Password set to newPassword, then
// set the entity's password accordingly.
func (c *conf) OpenAPI(dialOpts api.DialOpts) (st *api.State, newPassword string, err error) {
	info := *c.APIInfo
	info.Nonce = c.MachineNonce
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
	info.Password = c.OldPassword
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
func (c *conf) OpenState() (*state.State, error) {
	info := *c.StateInfo
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
	info.Password = c.OldPassword
	return state.Open(&info, state.DefaultDialOpts())
}

func InitialStateConfiguration(agentConfig Config, cfg *config.Config, timeout state.DialOpts) (*state.State, error) {
	c := agentConfig.(*conf)
	info := *c.StateInfo
	info.Tag = ""
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

	if err := bootstrapUsers(st, cfg, c.OldPassword); err != nil {
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

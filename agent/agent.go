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

	"launchpad.net/juju-core/agent/tools"
	"launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/utils"
)

type Config interface {
	// DataDir returns the data directory. Each agent has a subdirectory
	// containing the configuration files.
	// DO we need this???
	// DataDir() string

	// Tag returns the tag of the entity on whose behalf the state connection
	// will be made.
	Tag() string

	// Dir returns the agent's directory.
	Dir() string

	// OpenAPI tries to connect to an API end-point.  If a non-empty
	// newPassword is returned, the password used to connect to the state
	// should be changed accordingly - the caller should write the
	// configuration with StateInfo.Password set to newPassword, then set the
	// entity's password accordingly.
	// TODO(thumper): handle password changes internally.
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
}

// Conf holds information for a given agent.
type Conf struct {
	// DataDir specifies the path of the data directory used by all
	// agents
	DataDir string

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

// ReadConf reads configuration data for the given
// entity from the given data directory.
func ReadConf(dataDir, tag string) (Config, error) {
	dir := tools.Dir(dataDir, tag)
	data, err := ioutil.ReadFile(path.Join(dir, "agent.conf"))
	if err != nil {
		return nil, err
	}
	var c Conf
	if err := goyaml.Unmarshal(data, &c); err != nil {
		return nil, err
	}
	c.DataDir = dataDir
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
func (c *Conf) File(name string) string {
	return path.Join(c.Dir(), name)
}

func (c *Conf) confFile() string {
	return c.File("agent.conf")
}

// Tag returns the tag of the entity on whose behalf the state connection will
// be made.
func (c *Conf) Tag() string {
	if c.StateInfo != nil {
		return c.StateInfo.Tag
	}
	return c.APIInfo.Tag
}

// Dir returns the agent's directory.
func (c *Conf) Dir() string {
	return tools.Dir(c.DataDir, c.Tag())
}

// Check checks that the configuration has all the required elements.
func (c *Conf) check() error {
	if c.DataDir == "" {
		return requiredError("data directory")
	}
	if c.StateInfo == nil && c.APIInfo == nil {
		return requiredError("state info or API info")
	}
	if c.StateInfo != nil {
		if c.StateInfo.Tag == "" {
			return requiredError("state entity tag")
		}
		if err := checkAddrs(c.StateInfo.Addrs, "state server address"); err != nil {
			return err
		}
		if len(c.StateInfo.CACert) == 0 {
			return requiredError("state CA certificate")
		}
	}
	// TODO(rog) make APIInfo mandatory
	if c.APIInfo != nil {
		if c.APIInfo.Tag == "" {
			return requiredError("API entity tag")
		}
		if err := checkAddrs(c.APIInfo.Addrs, "API server address"); err != nil {
			return err
		}
		if len(c.APIInfo.CACert) == 0 {
			return requiredError("API CA certficate")
		}
	}
	if c.StateInfo != nil && c.APIInfo != nil && c.StateInfo.Tag != c.APIInfo.Tag {
		return fmt.Errorf("mismatched entity tags")
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

// Write writes the agent configuration.
func (c *Conf) Write() error {
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
func (c *Conf) WriteCommands() ([]string, error) {
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
	addCmd("echo %s > %s", utils.ShQuote(string(data)), f)
	return cmds, nil
}

// OpenAPI tries to open the state using the given Conf.  If it
// returns a non-empty newPassword, the password used to connect
// to the state should be changed accordingly - the caller should write the
// configuration with StateInfo.Password set to newPassword, then
// set the entity's password accordingly.
func (c *Conf) OpenAPI(dialOpts api.DialOpts) (st *api.State, newPassword string, err error) {
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
	password, err := utils.RandomPassword()
	if err != nil {
		st.Close()
		return nil, "", err
	}
	return st, password, nil
}

// OpenState tries to open the state using the given Conf.
func (c *Conf) OpenState() (*state.State, error) {
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

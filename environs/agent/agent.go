package agent

import (
	"fmt"
	"io/ioutil"
	"launchpad.net/goyaml"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
	"launchpad.net/juju-core/trivial"
	"os"
	"path/filepath"
	"regexp"
)

// Conf holds information for a given agent.
type Conf struct {
	// DataDir specifies the path of the data directory used by all
	// agents
	DataDir string

	// StateServerCert and StateServerKey hold the state server
	// certificate and private key in PEM format.
	StateServerCert []byte `yaml:",omitempty"`
	StateServerKey  []byte `yaml:",omitempty"`

	// OldPassword specifies a password that should be
	// used to connect to the state if StateInfo.Password
	// is blank or invalid.
	OldPassword string

	// StateInfo specifies how the agent should connect to the
	// state.  The password may be empty if an old password is
	// specified, or when bootstrapping.
	StateInfo state.Info

	// OldAPIPassword specifies a password that should
	// be used to connect to the API if APIInfo.Password
	// is blank or invalid.
	OldAPIPassword string

	// APIInfo specifies how the agent should connect to the
	// state through the API.
	APIInfo api.Info
}

var validAddr = regexp.MustCompile("^.+:[0-9]+$")

// ReadConf reads configuration data for the given
// entity from the given data directory.
func ReadConf(dataDir, entityName string) (*Conf, error) {
	dir := environs.AgentDir(dataDir, entityName)
	data, err := ioutil.ReadFile(filepath.Join(dir, "agent.conf"))
	if err != nil {
		return nil, err
	}
	var c Conf
	if err := goyaml.Unmarshal(data, &c); err != nil {
		return nil, err
	}
	c.DataDir = dataDir
	c.StateInfo.EntityName = entityName
	if err := c.Check(); err != nil {
		return nil, err
	}
	return &c, nil
}

func requiredError(what string) error {
	return fmt.Errorf("%s not found in configuration", what)
}

// File returns the path of the given file in the agent's directory.
func (c *Conf) File(name string) string {
	return filepath.Join(c.Dir(), name)
}

func (c *Conf) confFile() string {
	return c.File("agent.conf")
}

// Dir returns the agent's directory.
func (c *Conf) Dir() string {
	return environs.AgentDir(c.DataDir, c.StateInfo.EntityName)
}

// Check checks that the configuration has all the required elements.
func (c *Conf) Check() error {
	if c.DataDir == "" {
		return requiredError("data directory")
	}
	if c.StateInfo.EntityName == "" {
		return requiredError("state entity name")
	}
	if len(c.StateInfo.Addrs) == 0 {
		return requiredError("state server address")
	}
	for _, a := range c.StateInfo.Addrs {
		if !validAddr.MatchString(a) {
			return fmt.Errorf("invalid state server address %q", a)
		}
	}
	if len(c.StateInfo.CACert) == 0 {
		return requiredError("state CA certificate")
	}
	// TODO(rog) make APIInfo mandatory
	if c.APIInfo.Addr != "" {
		if !validAddr.MatchString(c.APIInfo.Addr) {
			return fmt.Errorf("invalid API server address %q", c.APIInfo.Addr)
		}
		if len(c.APIInfo.CACert) == 0 {
			return requiredError("API CA certficate")
		}
	}
	return nil
}

// Write writes the agent configuration.
func (c *Conf) Write() error {
	if err := c.Check(); err != nil {
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
	if err := c.Check(); err != nil {
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
	f := trivial.ShQuote(c.confFile())
	addCmd("mkdir -p %s", trivial.ShQuote(c.Dir()))
	addCmd("echo %s > %s", trivial.ShQuote(string(data)), f)
	addCmd("chmod %o %s", 0600, f)
	return cmds, nil
}

// OpenState tries to open the state using the given Conf.  If
// passwordChanged is returned as true, c.StateInfo.Password has been
// changed, and the caller should write the configuration
// and set the entity's password accordingly (in that order).
func (c *Conf) OpenState() (st *state.State, passwordChanged bool, err error) {
	info := c.StateInfo
	if info.Password != "" {
		st, err := state.Open(&info)
		if err == nil {
			return st, false, nil
		}
		if err != state.ErrUnauthorized {
			return nil, false, err
		}
		// Access isn't authorized even though we have a password
		// This can happen if we crash after saving the
		// password but before changing it, so we'll try again
		// with the old password.
	}
	info.Password = c.OldPassword
	st, err = state.Open(&info)
	if err != nil {
		return nil, false, err
	}
	// We've succeeded in connecting with the old password, so
	// we can now change it to something more private.
	password, err := trivial.RandomPassword()
	if err != nil {
		st.Close()
		return nil, false, err
	}
	c.StateInfo.Password = password
	return st, true, nil
}

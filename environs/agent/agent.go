package agent
import (
	"io/ioutil"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/trivial"
	"launchpad.net/juju-core/state"
	"regexp"
	"os"
	"strings"
	"fmt"
	"path/filepath"
)

// Conf holds information shared by all agents.
type Conf struct {
	// DataDir specifies the path of the data directory used by all
	// agents
	DataDir         string

	// InitialPassword specifies a password that be used to
	// initially connect to the state; this will be changed as soon
	// as possible by the agent.
	InitialPassword string

	// StateInfo specifies how the agent should connect to the
	// state.  The password may be empty if an initial password is
	// specified, or when bootstrapping.
	StateInfo state.Info
}

var validAddr = regexp.MustCompile("^.+:[0-9]+$")

// ReadConf reads configuration data for the given
// entity from the given data directory.
func ReadConf(dataDir, entityName string) (*Conf, error) {
	c := &Conf{
		DataDir: dataDir,
		StateInfo: state.Info{
			EntityName: entityName,
		},
	}
	data, err := ioutil.ReadFile(c.File("host-addrs"))
	if err != nil {
		return nil, err
	}
	c.StateInfo.Addrs = strings.Split(string(data), ",")
	for _, addr := range c.StateInfo.Addrs {
		if !validAddr.MatchString(addr) {
			return nil, fmt.Errorf("%q is not a valid state server address", addr)
		}
	}

	c.StateInfo.CACert, err = ioutil.ReadFile(c.File("ca-cert.pem"))
	if err != nil {
		return nil, err
	}

	data, err = ioutil.ReadFile(c.File("password"))
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	c.StateInfo.Password = string(data)

	data, err = ioutil.ReadFile(c.File("initial-password"))
	if err != nil {
		return nil, err
	}
	c.InitialPassword = string(data)
	if err := c.Check(); err != nil {
		return nil, err
	}
	return c, nil
}

func requiredError(what string) error {
	return fmt.Errorf("%s not found in configuration", what)
}

// File returns the path of the given file in the agent's directory.
func (c *Conf) File(name string) string {
	return filepath.Join(c.Dir(), name)
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
		return requiredError("entity name")
	}
	if len(c.StateInfo.Addrs) == 0 {
		return requiredError("state server address")
	}
	if len(c.StateInfo.CACert) == 0 {
		return requiredError("CA certificate")
	}
	return nil
}

// Write writes the agent configuration.
func (c *Conf) Write() error {
	if err := c.Check(); err != nil {
		return err
	}
	if err := os.MkdirAll(c.Dir(), 0755); err != nil {
		return err
	}
	if err := ioutil.WriteFile(c.File("host-addrs"), []byte(strings.Join(c.StateInfo.Addrs, ",")), 0644); err != nil {
		return err
	}
	if err := ioutil.WriteFile(c.File("ca-cert.pem"), c.StateInfo.CACert, 0644); err != nil {
		return err
	}
	if c.InitialPassword != "" {
		if err := ioutil.WriteFile(c.File("initial-password"), []byte(c.InitialPassword), 0600); err  != nil {
			return err
		}
	}
	if c.StateInfo.Password != "" {
		if err := ioutil.WriteFile(c.File("password"), []byte(c.StateInfo.Password), 0600); err != nil {
			return err
		}
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
	var cmds []string
	addCmd := func(f string, a ...interface{}) {
		cmds = append(cmds, fmt.Sprintf(f, a...))
	}
	addFile := func(name, data string, mode uint) {
		p := trivial.ShQuote(c.File(name))
		addCmd("echo %s > %s", trivial.ShQuote(data), p)
		addCmd("chmod %o %s", mode, p)
	}
	addCmd("mkdir -p %s", trivial.ShQuote(c.Dir()))
	addFile("ca-cert.pem", string(c.StateInfo.CACert), 0644)
	addFile("host-addrs", strings.Join(c.StateInfo.Addrs, ","), 0644)
	if c.InitialPassword != "" {
		addFile("initial-password", c.InitialPassword, 0600)
	}
	if c.StateInfo.Password != "" {
		addFile("password", c.StateInfo.Password, 0600)
	}
	return cmds, nil
}

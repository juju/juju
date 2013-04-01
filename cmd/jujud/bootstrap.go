package main

import (
	"encoding/base64"
	"fmt"
	"launchpad.net/gnuflag"
	"launchpad.net/goyaml"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/version"
)

type BootstrapCommand struct {
	cmd.CommandBase
	Conf        AgentConf
	EnvConfig   map[string]interface{}
	Constraints constraints.Value
}

// Info returns a decription of the command.
func (c *BootstrapCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "bootstrap-state",
		Purpose: "initialize juju state",
	}
}

func (c *BootstrapCommand) SetFlags(f *gnuflag.FlagSet) {
	c.Conf.addFlags(f)
	yamlBase64Var(f, &c.EnvConfig, "env-config", "", "initial environment configuration (yaml, base64 encoded)")
	f.Var(constraints.ConstraintsValue{&c.Constraints}, "constraints", "initial environment constraints (space-separated strings)")
}

// Init initializes the command for running.
func (c *BootstrapCommand) Init(args []string) error {
	if len(c.EnvConfig) == 0 {
		return requiredError("env-config")
	}
	return c.Conf.checkArgs(args)
}

// Run initializes state for an environment.
func (c *BootstrapCommand) Run(_ *cmd.Context) error {
	if err := c.Conf.read("bootstrap"); err != nil {
		return err
	}
	cfg, err := config.New(c.EnvConfig)
	if err != nil {
		return err
	}
	provider, err := environs.Provider(cfg.Type())
	if err != nil {
		return err
	}
	instanceId, err := provider.InstanceId()
	if err != nil {
		return err
	}

	// There is no entity that's created at init time.
	c.Conf.StateInfo.Tag = ""
	st, err := state.Initialize(c.Conf.StateInfo, cfg, state.DefaultDialTimeout)
	if err != nil {
		return err
	}
	defer st.Close()

	if err := st.SetEnvironConstraints(c.Constraints); err != nil {
		return err
	}
	// TODO: we need to be able to customize machine jobs, not just hardcode these.
	m, err := st.InjectMachine(
		version.Current.Series, instanceId,
		state.JobManageEnviron, state.JobServeAPI,
	)
	if err != nil {
		return err
	}

	// Set up initial authentication.
	if _, err := st.AddUser("admin", c.Conf.OldPassword); err != nil {
		return err
	}
	if err := m.SetMongoPassword(c.Conf.OldPassword); err != nil {
		return err
	}
	if err := st.SetAdminMongoPassword(c.Conf.OldPassword); err != nil {
		return err
	}
	return nil
}

// yamlBase64Value implements gnuflag.Value on a map[string]interface{}.
type yamlBase64Value map[string]interface{}

// Set decodes the base64 value into yaml then expands that into a map.
func (v *yamlBase64Value) Set(value string) error {
	decoded, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		return err
	}
	return goyaml.Unmarshal(decoded, v)
}

func (v *yamlBase64Value) String() string {
	return fmt.Sprintf("%v", *v)
}

// yamlBase64Var sets up a gnuflag flag analogous to the FlagSet.*Var methods.
func yamlBase64Var(fs *gnuflag.FlagSet, target *map[string]interface{}, name string, value string, usage string) {
	fs.Var((*yamlBase64Value)(target), name, usage)
}

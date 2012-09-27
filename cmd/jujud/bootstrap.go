package main

import (
	"encoding/base64"
	"fmt"
	"launchpad.net/gnuflag"
	"launchpad.net/goyaml"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/state"
)

type BootstrapCommand struct {
	StateInfo  state.Info
	InstanceId string
	EnvConfig  map[string]interface{}
}

// Info returns a decription of the command.
func (c *BootstrapCommand) Info() *cmd.Info {
	return &cmd.Info{"bootstrap-state", "", "initialize juju state.", ""}
}

// Init initializes the command for running.
func (c *BootstrapCommand) Init(f *gnuflag.FlagSet, args []string) error {
	stateInfoVar(f, &c.StateInfo, "state-servers", []string{"127.0.0.1:37017"}, "address of state server to initialize")
	f.StringVar(&c.InstanceId, "instance-id", "", "instance id of this machine")
	yamlBase64Var(f, &c.EnvConfig, "env-config", "", "initial environment configuration (yaml, base64 encoded)")
	if err := f.Parse(true, args); err != nil {
		return err
	}
	if c.StateInfo.Addrs == nil {
		return requiredError("state-servers")
	}
	if c.InstanceId == "" {
		return requiredError("instance-id")
	}
	if len(c.EnvConfig) == 0 {
		return requiredError("env-config")
	}
	return cmd.CheckEmpty(f.Args())
}

// Run initializes state for an environment.
func (c *BootstrapCommand) Run(_ *cmd.Context) error {
	cfg, err := config.New(c.EnvConfig)
	if err != nil {
		return err
	}
	st, err := state.Initialize(&c.StateInfo, cfg)
	if err != nil {
		return err
	}
	defer st.Close()

	// Manually insert machine/0 into the state
	m, err := st.AddMachine(state.MachineWorker)
	if err != nil {
		return err
	}

	// Set the instance id of machine/0
	if err := m.SetInstanceId(c.InstanceId); err != nil {
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

// yamlBase64Var sets up a gnuflag flag analagously to FlagSet.*Var methods.
func yamlBase64Var(fs *gnuflag.FlagSet, target *map[string]interface{}, name string, value string, usage string) {
	fs.Var((*yamlBase64Value)(target), name, usage)
}

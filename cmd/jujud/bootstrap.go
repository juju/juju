// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"strings"

	"launchpad.net/gnuflag"
	"launchpad.net/goyaml"

	"launchpad.net/juju-core/agent"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/environs/bootstrap"
	"launchpad.net/juju-core/environs/cloudinit"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/state"
)

// Cloud-init write the URL to be used to load the bootstrap state into this file.
// A variable is used here to allow tests to override.
var providerStateURLFile = cloudinit.BootstrapStateURLFile

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
	data, err := ioutil.ReadFile(providerStateURLFile)
	if err != nil {
		return fmt.Errorf("cannot read provider-state-url file: %v", err)
	}
	stateInfoURL := strings.Split(string(data), "\n")[0]
	bsState, err := bootstrap.LoadStateFromURL(stateInfoURL)
	if err != nil {
		return fmt.Errorf("cannot load state from URL %q (read from %q): %v", stateInfoURL, providerStateURLFile, err)
	}
	err = c.Conf.read("machine-0")
	if err != nil {
		return err
	}
	envCfg, err := config.New(config.NoDefaults, c.EnvConfig)
	if err != nil {
		return err
	}
	// TODO(fwereade): we need to be able to customize machine jobs,
	// not just hardcode these values; in particular, JobHostUnits
	// on a machine, like this one, that is running JobManageEnviron
	// (not to mention the actual state server itself...) will allow
	// a malicious or compromised unit to trivially access to the
	// user's environment credentials. However, given that this point
	// is currently moot (see Upgrader in this package), the pseudo-
	// local provider mode (in which everything is deployed with
	// `--to 0`) offers enough value to enough people that
	// JobHostUnits is currently always enabled. This will one day
	// have to change, but it's strictly less important than fixing
	// Upgrader, and it's a capability we'll always want to have
	// available for the aforementioned use case.
	jobs := []state.MachineJob{
		state.JobManageEnviron,
		state.JobHostUnits,
	}
	var characteristics instance.HardwareCharacteristics
	if len(bsState.Characteristics) > 0 {
		characteristics = bsState.Characteristics[0]
	}
	st, _, err := c.Conf.config.InitializeState(envCfg, agent.BootstrapMachineConfig{
		Constraints:     c.Constraints,
		Jobs:            jobs,
		InstanceId:      bsState.StateInstances[0],
		Characteristics: characteristics,
	}, state.DefaultDialOpts())
	if err != nil {
		return err
	}
	st.Close()
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

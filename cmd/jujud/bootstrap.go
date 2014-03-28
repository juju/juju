// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"encoding/base64"
	"fmt"
	"net"
	"sort"

	"launchpad.net/gnuflag"
	"launchpad.net/goyaml"

	"launchpad.net/juju-core/agent"
	"launchpad.net/juju-core/agent/mongo"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
)

type BootstrapCommand struct {
	cmd.CommandBase
	AgentConf
	EnvConfig   map[string]interface{}
	Constraints constraints.Value
	Hardware    instance.HardwareCharacteristics
	InstanceId  string
}

// Info returns a decription of the command.
func (c *BootstrapCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "bootstrap-state",
		Purpose: "initialize juju state",
	}
}

func (c *BootstrapCommand) SetFlags(f *gnuflag.FlagSet) {
	c.AgentConf.AddFlags(f)
	yamlBase64Var(f, &c.EnvConfig, "env-config", "", "initial environment configuration (yaml, base64 encoded)")
	f.Var(constraints.ConstraintsValue{&c.Constraints}, "constraints", "initial environment constraints (space-separated strings)")
	f.Var(&c.Hardware, "hardware", "hardware characteristics (space-separated strings)")
	f.StringVar(&c.InstanceId, "instance-id", "", "unique instance-id for bootstrap machine")
}

// Init initializes the command for running.
func (c *BootstrapCommand) Init(args []string) error {
	if len(c.EnvConfig) == 0 {
		return requiredError("env-config")
	}
	if c.InstanceId == "" {
		return requiredError("instance-id")
	}
	return c.AgentConf.CheckArgs(args)
}

// Run initializes state for an environment.
func (c *BootstrapCommand) Run(_ *cmd.Context) error {
	envCfg, err := config.New(config.NoDefaults, c.EnvConfig)
	if err != nil {
		return err
	}
	err = c.ReadConfig("machine-0")
	if err != nil {
		return err
	}
	agentConfig := c.CurrentConfig()
	// agent.Jobs is an optional field in the agent config, and was
	// introduced after 1.17.2. We default to allowing units on
	// machine-0 if missing.
	jobs := agentConfig.Jobs()
	if len(jobs) == 0 {
		jobs = []params.MachineJob{
			params.JobManageEnviron,
			params.JobHostUnits,
		}
	}

	env, err := environs.New(envCfg)
	if err != nil {
		return err
	}
	insts, err := env.Instances([]instance.Id{instance.Id(c.InstanceId)})
	if err != nil {
		return err
	}

	// We are bootstrapping so we know we want the first
	// and only instance.
	inst := insts[0]
	addresses, err := inst.Addresses()
	if err != nil {
		return err
	}

	var st *state.State
	err = nil
	writeErr := c.ChangeConfig(func(agentConfig agent.ConfigSetter) {
		st, _, err = agent.InitializeState(
			agentConfig,
			envCfg,
			agent.BootstrapMachineConfig{
				Constraints:     c.Constraints,
				Jobs:            jobs,
				InstanceId:      instance.Id(c.InstanceId),
				Characteristics: c.Hardware,
				Addresses:       addresses,
			},
			state.DefaultDialOpts(),
			environs.NewStatePolicy(),
		)
	})
	if writeErr != nil {
		return fmt.Errorf("cannot write initial configuration: %v", err)
	}
	if err != nil {
		return err
	}
	st.Close()

	logger.Infof("%v", addresses)

	preferredAddr, err := selectPreferredStateServerAddress(addresses)
	if err != nil {
		return err
	}
	dialInfo, err := state.DialInfo(c.Conf.config.StateInfo(), state.DefaultDialOpts())
	if err != nil {
		return err
	}
	if err := ensureMongoServer(mongo.EnsureMongoParams{
		HostPort: net.JoinHostPort(preferredAddr.String(), fmt.Sprint(envCfg.StatePort())),
		DataDir:  c.Conf.config.DataDir(),
		DialInfo: dialInfo,
	}); err != nil {
		return err
	}
	return nil
}

func selectPreferredStateServerAddress(addrs []instance.Address) (instance.Address, error) {
	if len(addrs) == 0 {
		return instance.Address{}, fmt.Errorf("no state server addresses")
	}
	newAddrs := append(byAddressPreference{}, addrs...)
	sort.Stable(newAddrs)
	return newAddrs[0], nil
}

// byAddressPreference sorts addresses, preferring numeric cloud local addresses.
type byAddressPreference []instance.Address

func (a byAddressPreference) Len() int {
	return len(a)
}

func (a byAddressPreference) Swap(i, j int) {
	a[i], a[j] = a[j], a[i]
}

func (a byAddressPreference) Less(i, j int) bool {
	a0, a1 := &a[i], &a[j]
	if pref0, pref1 := netScopePref(a0.NetworkScope), netScopePref(a1.NetworkScope); pref0 != pref1 {
		return pref0 < pref1
	}
	if pref0, pref1 := netTypePref(a0.Type), netTypePref(a1.Type); pref0 != pref1 {
		return pref0 < pref1
	}
	return false
}

func netScopePref(scope instance.NetworkScope) int {
	switch scope {
	case instance.NetworkCloudLocal:
		return 0
	case instance.NetworkUnknown:
		return 1
	}
	return 2
}

func netTypePref(atype instance.AddressType) int {
	switch atype {
	case instance.HostName:
		return 0
	}
	return 1
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

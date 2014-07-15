// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"encoding/base64"
	"fmt"
	"net"
	"strconv"
	"time"

	"github.com/juju/cmd"
	"launchpad.net/gnuflag"
	"launchpad.net/goyaml"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/api/params"
	"github.com/juju/juju/worker/peergrouper"
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
	f.Var(constraints.ConstraintsValue{Target: &c.Constraints}, "constraints", "initial environment constraints (space-separated strings)")
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
	network.InitializeFromConfig(agentConfig)

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

	// Get the bootstrap machine's addresses from the provider.
	env, err := environs.New(envCfg)
	if err != nil {
		return err
	}
	instanceId := instance.Id(c.InstanceId)
	instances, err := env.Instances([]instance.Id{instanceId})
	if err != nil {
		return err
	}
	addrs, err := instances[0].Addresses()
	if err != nil {
		return err
	}

	// Create system-identity file
	if err := agent.WriteSystemIdentityFile(agentConfig); err != nil {
		return err
	}

	// Generate a shared secret for the Mongo replica set, and write it out.
	sharedSecret, err := mongo.GenerateSharedSecret()
	if err != nil {
		return err
	}
	info, ok := agentConfig.StateServingInfo()
	if !ok {
		return fmt.Errorf("bootstrap machine config has no state serving info")
	}
	info.SharedSecret = sharedSecret
	err = c.ChangeConfig(func(agentConfig agent.ConfigSetter) {
		agentConfig.SetStateServingInfo(info)
	})
	if err != nil {
		return fmt.Errorf("cannot write agent config: %v", err)
	}
	agentConfig = c.CurrentConfig()

	if err := c.startMongo(addrs, agentConfig); err != nil {
		return err
	}

	logger.Infof("started mongo")
	// Initialise state, and store any agent config (e.g. password) changes.
	var st *state.State
	var m *state.Machine
	err = nil
	writeErr := c.ChangeConfig(func(agentConfig agent.ConfigSetter) {
		st, m, err = agent.InitializeState(
			agentConfig,
			envCfg,
			agent.BootstrapMachineConfig{
				Addresses:       addrs,
				Constraints:     c.Constraints,
				Jobs:            jobs,
				InstanceId:      instanceId,
				Characteristics: c.Hardware,
				SharedSecret:    sharedSecret,
			},
			mongo.DefaultDialOpts(),
			environs.NewStatePolicy(),
		)
	})
	if writeErr != nil {
		return fmt.Errorf("cannot write initial configuration: %v", err)
	}
	if err != nil {
		return err
	}
	defer st.Close()

	// bootstrap machine always gets the vote
	return m.SetHasVote(true)
}

// newEnsureServerParams creates an EnsureServerParams from an agent configuration.
func newEnsureServerParams(agentConfig agent.Config) (mongo.EnsureServerParams, error) {
	// If oplog size is specified in the agent configuration, use that.
	// Otherwise leave the default zero value to indicate to EnsureServer
	// that it should calculate the size.
	var oplogSize int
	if oplogSizeString := agentConfig.Value(agent.MongoOplogSize); oplogSizeString != "" {
		var err error
		if oplogSize, err = strconv.Atoi(oplogSizeString); err != nil {
			return mongo.EnsureServerParams{}, fmt.Errorf("invalid oplog size: %q", oplogSizeString)
		}
	}

	servingInfo, ok := agentConfig.StateServingInfo()
	if !ok {
		return mongo.EnsureServerParams{}, fmt.Errorf("agent config has no state serving info")
	}

	params := mongo.EnsureServerParams{
		StateServingInfo: servingInfo,
		DataDir:          agentConfig.DataDir(),
		Namespace:        agentConfig.Value(agent.Namespace),
		OplogSize:        oplogSize,
	}
	return params, nil
}

func (c *BootstrapCommand) startMongo(addrs []network.Address, agentConfig agent.Config) error {
	logger.Debugf("starting mongo")

	info, ok := agentConfig.MongoInfo()
	if !ok {
		return fmt.Errorf("no state info available")
	}
	// When bootstrapping, we need to allow enough time for mongo
	// to start as there's no retry loop in place.
	// 5 minutes should suffice.
	bootstrapDialOpts := mongo.DialOpts{Timeout: 5 * time.Minute}
	dialInfo, err := mongo.DialInfo(info.Info, bootstrapDialOpts)
	if err != nil {
		return err
	}
	servingInfo, ok := agentConfig.StateServingInfo()
	if !ok {
		return fmt.Errorf("agent config has no state serving info")
	}
	// Use localhost to dial the mongo server, because it's running in
	// auth mode and will refuse to perform any operations unless
	// we dial that address.
	dialInfo.Addrs = []string{
		net.JoinHostPort("127.0.0.1", fmt.Sprint(servingInfo.StatePort)),
	}

	logger.Debugf("calling ensureMongoServer")
	ensureServerParams, err := newEnsureServerParams(agentConfig)
	if err != nil {
		return err
	}
	err = ensureMongoServer(ensureServerParams)
	if err != nil {
		return err
	}

	peerAddr := mongo.SelectPeerAddress(addrs)
	if peerAddr == "" {
		return fmt.Errorf("no appropriate peer address found in %q", addrs)
	}
	peerHostPort := net.JoinHostPort(peerAddr, fmt.Sprint(servingInfo.StatePort))

	return maybeInitiateMongoServer(peergrouper.InitiateMongoParams{
		DialInfo:       dialInfo,
		MemberHostPort: peerHostPort,
	})
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

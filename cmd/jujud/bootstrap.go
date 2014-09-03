// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/utils"
	goyaml "gopkg.in/yaml.v1"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/agent"
	agenttools "github.com/juju/juju/agent/tools"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/filestorage"
	"github.com/juju/juju/environs/storage"
	"github.com/juju/juju/environs/sync"
	envtools "github.com/juju/juju/environs/tools"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
	"github.com/juju/juju/utils/ssh"
	"github.com/juju/juju/version"
	"github.com/juju/juju/worker/peergrouper"
)

var (
	agentInitializeState = agent.InitializeState
	sshGenerateKey       = ssh.GenerateKey
	minSocketTimeout     = 1 * time.Minute
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

	// Generate a private SSH key for the state servers, and add
	// the public key to the environment config. We'll add the
	// private key to StateServingInfo below.
	privateKey, publicKey, err := sshGenerateKey(config.JujuSystemKey)
	if err != nil {
		return errors.Annotate(err, "failed to generate system key")
	}
	authorizedKeys := config.ConcatAuthKeys(envCfg.AuthorizedKeys(), publicKey)
	envCfg, err = env.Config().Apply(map[string]interface{}{
		config.AuthKeysConfig: authorizedKeys,
	})
	if err != nil {
		return errors.Annotate(err, "failed to add public key to environment config")
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
	info.SystemIdentity = privateKey
	err = c.ChangeConfig(func(agentConfig agent.ConfigSetter) error {
		agentConfig.SetStateServingInfo(info)
		return nil
	})
	if err != nil {
		return fmt.Errorf("cannot write agent config: %v", err)
	}
	agentConfig = c.CurrentConfig()

	// Create system-identity file
	if err := agent.WriteSystemIdentityFile(agentConfig); err != nil {
		return err
	}

	if err := c.startMongo(addrs, agentConfig); err != nil {
		return err
	}

	logger.Infof("started mongo")
	// Initialise state, and store any agent config (e.g. password) changes.
	var st *state.State
	var m *state.Machine
	err = c.ChangeConfig(func(agentConfig agent.ConfigSetter) error {
		var stateErr error
		dialOpts := mongo.DefaultDialOpts()

		// Set a longer socket timeout than usual, as the machine
		// will be starting up and disk I/O slower than usual. This
		// has been known to cause timeouts in queries.
		timeouts := envCfg.BootstrapSSHOpts()
		dialOpts.SocketTimeout = timeouts.Timeout
		if dialOpts.SocketTimeout < minSocketTimeout {
			dialOpts.SocketTimeout = minSocketTimeout
		}

		// We shouldn't attempt to dial peers until we have some.
		dialOpts.Direct = true

		st, m, stateErr = agentInitializeState(
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
			dialOpts,
			environs.NewStatePolicy(),
		)
		return stateErr
	})
	if err != nil {
		return err
	}
	defer st.Close()

	// Populate the tools catalogue.
	if err := c.populateTools(env); err != nil {
		return err
	}

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

// populateTools stores uploaded tools in provider storage
// and updates the tools metadata.
//
// TODO(axw) store tools in gridfs, catalogue in state.
func (c *BootstrapCommand) populateTools(env environs.Environ) error {
	agentConfig := c.CurrentConfig()
	dataDir := agentConfig.DataDir()
	tools, err := agenttools.ReadTools(dataDir, version.Current)
	if err != nil {
		return err
	}
	if !strings.HasPrefix(tools.URL, "file://") {
		// Nothing to do since the tools were not uploaded.
		return nil
	}

	// This is a hack: providers using localstorage (local, manual)
	// can't use storage during bootstrap as the localstorage worker
	// isn't running. Use filestorage instead.
	var stor storage.Storage
	storageDir := agentConfig.Value(agent.StorageDir)
	if storageDir != "" {
		stor, err = filestorage.NewFileStorageWriter(storageDir)
		if err != nil {
			return err
		}
	} else {
		stor = env.Storage()
	}

	// Create a temporary directory to contain source and cloned tools.
	tempDir, err := ioutil.TempDir("", "juju-sync-tools")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tempDir)
	destTools := filepath.Join(tempDir, filepath.FromSlash(envtools.StorageName(tools.Version)))
	if err := os.MkdirAll(filepath.Dir(destTools), 0700); err != nil {
		return err
	}
	srcTools := filepath.Join(
		agenttools.SharedToolsDir(dataDir, version.Current),
		"tools.tar.gz",
	)
	if err := utils.CopyFile(destTools, srcTools); err != nil {
		return err
	}

	// Until we catalogue tools in state, we clone the tools
	// for each of the supported series of the same OS.
	otherSeries := version.OSSupportedSeries(version.Current.OS)
	_, err = sync.SyncBuiltTools(stor, &sync.BuiltTools{
		Version:     tools.Version,
		Dir:         tempDir,
		StorageName: envtools.StorageName(tools.Version),
		Sha256Hash:  tools.SHA256,
		Size:        tools.Size,
	}, otherSeries...)
	return err
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

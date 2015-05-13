// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"
	goyaml "gopkg.in/yaml.v1"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/agent"
	agenttools "github.com/juju/juju/agent/tools"
	agentcmd "github.com/juju/juju/cmd/jujud/agent"
	cmdutil "github.com/juju/juju/cmd/jujud/util"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/multiwatcher"
	"github.com/juju/juju/state/storage"
	"github.com/juju/juju/state/toolstorage"
	"github.com/juju/juju/storage/poolmanager"
	"github.com/juju/juju/utils/ssh"
	"github.com/juju/juju/version"
	"github.com/juju/juju/worker/peergrouper"
)

var (
	maybeInitiateMongoServer = peergrouper.MaybeInitiateMongoServer
	agentInitializeState     = agent.InitializeState
	sshGenerateKey           = ssh.GenerateKey
	newStateStorage          = storage.NewStorage
	minSocketTimeout         = 1 * time.Minute
	logger                   = loggo.GetLogger("juju.cmd.jujud")
)

type BootstrapCommand struct {
	cmd.CommandBase
	agentcmd.AgentConf
	EnvConfig        map[string]interface{}
	Constraints      constraints.Value
	Hardware         instance.HardwareCharacteristics
	InstanceId       string
	AdminUsername    string
	ImageMetadataDir string
}

// NewBootstrapCommand returns a new BootstrapCommand that has been initialized.
func NewBootstrapCommand() *BootstrapCommand {
	return &BootstrapCommand{
		AgentConf: agentcmd.NewAgentConf(""),
	}
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
	f.StringVar(&c.AdminUsername, "admin-user", "admin", "set the name for the juju admin user")
	f.StringVar(&c.ImageMetadataDir, "image-metadata", "", "custom image metadata source dir")
}

// Init initializes the command for running.
func (c *BootstrapCommand) Init(args []string) error {
	if len(c.EnvConfig) == 0 {
		return cmdutil.RequiredError("env-config")
	}
	if c.InstanceId == "" {
		return cmdutil.RequiredError("instance-id")
	}
	if !names.IsValidUser(c.AdminUsername) {
		return errors.Errorf("%q is not a valid username", c.AdminUsername)
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
		jobs = []multiwatcher.MachineJob{
			multiwatcher.JobManageEnviron,
			multiwatcher.JobHostUnits,
			multiwatcher.JobManageNetworking,
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

		adminTag := names.NewLocalUserTag(c.AdminUsername)
		st, m, stateErr = agentInitializeState(
			adminTag,
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
	if err := c.populateTools(st, env); err != nil {
		return err
	}

	// Add custom image metadata to environment storage.
	if c.ImageMetadataDir != "" {
		stor := newStateStorage(st.EnvironUUID(), st.MongoSession())
		if err := c.storeCustomImageMetadata(stor); err != nil {
			return err
		}
	}

	// Populate the storage pools.
	if err := c.populateDefaultStoragePools(st); err != nil {
		return err
	}

	// bootstrap machine always gets the vote
	return m.SetHasVote(true)
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
	ensureServerParams, err := cmdutil.NewEnsureServerParams(agentConfig)
	if err != nil {
		return err
	}
	err = cmdutil.EnsureMongoServer(ensureServerParams)
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

// populateDefaultStoragePools creates the default storage pools.
func (c *BootstrapCommand) populateDefaultStoragePools(st *state.State) error {
	settings := state.NewStateSettings(st)
	return poolmanager.AddDefaultStoragePools(settings)
}

// populateTools stores uploaded tools in provider storage
// and updates the tools metadata.
func (c *BootstrapCommand) populateTools(st *state.State, env environs.Environ) error {
	agentConfig := c.CurrentConfig()
	dataDir := agentConfig.DataDir()
	tools, err := agenttools.ReadTools(dataDir, version.Current)
	if err != nil {
		return err
	}

	data, err := ioutil.ReadFile(filepath.Join(
		agenttools.SharedToolsDir(dataDir, version.Current),
		"tools.tar.gz",
	))
	if err != nil {
		return err
	}

	storage, err := st.ToolsStorage()
	if err != nil {
		return err
	}
	defer storage.Close()

	var toolsVersions []version.Binary
	if strings.HasPrefix(tools.URL, "file://") {
		// Tools were uploaded: clone for each series of the same OS.
		osSeries := version.OSSupportedSeries(tools.Version.OS)
		for _, series := range osSeries {
			toolsVersion := tools.Version
			toolsVersion.Series = series
			toolsVersions = append(toolsVersions, toolsVersion)
		}
	} else {
		// Tools were downloaded from an external source: don't clone.
		toolsVersions = []version.Binary{tools.Version}
	}

	for _, toolsVersion := range toolsVersions {
		metadata := toolstorage.Metadata{
			Version: toolsVersion,
			Size:    tools.Size,
			SHA256:  tools.SHA256,
		}
		logger.Debugf("Adding tools: %v", toolsVersion)
		if err := storage.AddTools(bytes.NewReader(data), metadata); err != nil {
			return err
		}
	}
	return nil
}

// storeCustomImageMetadata reads the custom image metadata from disk,
// and stores the files in environment storage with the same relative
// paths.
func (c *BootstrapCommand) storeCustomImageMetadata(stor storage.Storage) error {
	logger.Debugf("storing custom image metadata from %q", c.ImageMetadataDir)
	return filepath.Walk(c.ImageMetadataDir, func(abspath string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		relpath, err := filepath.Rel(c.ImageMetadataDir, abspath)
		if err != nil {
			return err
		}
		f, err := os.Open(abspath)
		if err != nil {
			return err
		}
		defer f.Close()
		relpath = filepath.ToSlash(relpath)
		logger.Debugf("storing %q in environment storage (%d bytes)", relpath, info.Size())
		return stor.Put(relpath, f, info.Size())
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

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
	"github.com/juju/utils"
	"github.com/juju/utils/arch"
	"github.com/juju/utils/series"
	"github.com/juju/utils/ssh"
	goyaml "gopkg.in/yaml.v2"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/agent"
	agenttools "github.com/juju/juju/agent/tools"
	agentcmd "github.com/juju/juju/cmd/jujud/agent"
	cmdutil "github.com/juju/juju/cmd/jujud/util"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/environs/simplestreams"
	envtools "github.com/juju/juju/environs/tools"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/cloudimagemetadata"
	"github.com/juju/juju/state/multiwatcher"
	"github.com/juju/juju/state/storage"
	"github.com/juju/juju/state/toolstorage"
	"github.com/juju/juju/storage/poolmanager"
	"github.com/juju/juju/tools"
	"github.com/juju/juju/version"
	"github.com/juju/juju/worker/peergrouper"
)

var (
	initiateMongoServer  = peergrouper.InitiateMongoServer
	agentInitializeState = agent.InitializeState
	sshGenerateKey       = ssh.GenerateKey
	newStateStorage      = storage.NewStorage
	minSocketTimeout     = 1 * time.Minute
	logger               = loggo.GetLogger("juju.cmd.jujud")
)

// BootstrapCommand represents a jujud bootstrap command.
type BootstrapCommand struct {
	cmd.CommandBase
	agentcmd.AgentConf
	EnvConfig            map[string]interface{}
	BootstrapConstraints constraints.Value
	EnvironConstraints   constraints.Value
	Hardware             instance.HardwareCharacteristics
	InstanceId           string
	AdminUsername        string
	ImageMetadataDir     string
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

// SetFlags adds the flags for this command to the passed gnuflag.FlagSet.
func (c *BootstrapCommand) SetFlags(f *gnuflag.FlagSet) {
	c.AgentConf.AddFlags(f)
	yamlBase64Var(f, &c.EnvConfig, "model-config", "", "initial model configuration (yaml, base64 encoded)")
	f.Var(constraints.ConstraintsValue{Target: &c.BootstrapConstraints}, "bootstrap-constraints", "bootstrap machine constraints (space-separated strings)")
	f.Var(constraints.ConstraintsValue{Target: &c.EnvironConstraints}, "constraints", "initial constraints (space-separated strings)")
	f.Var(&c.Hardware, "hardware", "hardware characteristics (space-separated strings)")
	f.StringVar(&c.InstanceId, "instance-id", "", "unique instance-id for bootstrap machine")
	f.StringVar(&c.AdminUsername, "admin-user", "admin", "set the name for the juju admin user")
	f.StringVar(&c.ImageMetadataDir, "image-metadata", "", "custom image metadata source dir")
}

// Init initializes the command for running.
func (c *BootstrapCommand) Init(args []string) error {
	if len(c.EnvConfig) == 0 {
		return cmdutil.RequiredError("model-config")
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
	network.SetPreferIPv6(agentConfig.PreferIPv6())

	// agent.Jobs is an optional field in the agent config, and was
	// introduced after 1.17.2. We default to allowing units on
	// machine-0 if missing.
	jobs := agentConfig.Jobs()
	if len(jobs) == 0 {
		jobs = []multiwatcher.MachineJob{
			multiwatcher.JobManageModel,
			multiwatcher.JobHostUnits,
			multiwatcher.JobManageNetworking,
		}
	}

	// Get the bootstrap machine's addresses from the provider.
	env, err := environs.New(envCfg)
	if err != nil {
		return err
	}
	newConfigAttrs := make(map[string]interface{})

	// Check to see if a newer agent version has been requested
	// by the bootstrap client.
	desiredVersion, ok := envCfg.AgentVersion()
	if ok && desiredVersion != version.Current {
		// If we have been asked for a newer version, ensure the newer
		// tools can actually be found, or else bootstrap won't complete.
		stream := envtools.PreferredStream(&desiredVersion, envCfg.Development(), envCfg.AgentStream())
		logger.Infof("newer tools requested, looking for %v in stream %v", desiredVersion, stream)
		filter := tools.Filter{
			Number: desiredVersion,
			Arch:   arch.HostArch(),
			Series: series.HostSeries(),
		}
		_, toolsErr := envtools.FindTools(env, -1, -1, stream, filter)
		if toolsErr == nil {
			logger.Infof("tools are available, upgrade will occur after bootstrap")
		}
		if errors.IsNotFound(toolsErr) {
			// Newer tools not available, so revert to using the tools
			// matching the current agent version.
			logger.Warningf("newer tools for %q not available, sticking with version %q", desiredVersion, version.Current)
			newConfigAttrs["agent-version"] = version.Current.String()
		} else if toolsErr != nil {
			logger.Errorf("cannot find newer tools: %v", toolsErr)
			return toolsErr
		}
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

	// When machine addresses are reported from state, they have
	// duplicates removed.  We should do the same here so that
	// there is not unnecessary churn in the mongo replicaset.
	// TODO (cherylj) Add explicit unit tests for this - tracked
	// by bug #1544158.
	addrs = network.MergedAddresses([]network.Address{}, addrs)

	// Generate a private SSH key for the controllers, and add
	// the public key to the environment config. We'll add the
	// private key to StateServingInfo below.
	privateKey, publicKey, err := sshGenerateKey(config.JujuSystemKey)
	if err != nil {
		return errors.Annotate(err, "failed to generate system key")
	}
	authorizedKeys := config.ConcatAuthKeys(envCfg.AuthorizedKeys(), publicKey)
	newConfigAttrs[config.AuthKeysConfig] = authorizedKeys

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

	err = c.ChangeConfig(func(config agent.ConfigSetter) error {
		// We cannot set wired tiger as the storage because mongo
		// shipped with ubuntu lacks js.
		if mongo.BinariesAvailable(mongo.Mongo30wt) {
			config.SetMongoVersion(mongo.Mongo30wt)
		} else {
			config.SetMongoVersion(mongo.Mongo24)
		}
		return nil
	})
	if err != nil {
		return errors.Annotate(err, "cannot set mongo version")
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
	envCfg, err = env.Config().Apply(newConfigAttrs)
	if err != nil {
		return errors.Annotate(err, "failed to update model config")
	}
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
				Addresses:            addrs,
				BootstrapConstraints: c.BootstrapConstraints,
				ModelConstraints:     c.EnvironConstraints,
				Jobs:                 jobs,
				InstanceId:           instanceId,
				Characteristics:      c.Hardware,
				SharedSecret:         sharedSecret,
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
		if err := c.saveCustomImageMetadata(st, env); err != nil {
			return err
		}

		stor := newStateStorage(st.ModelUUID(), st.MongoSession())
		if err := c.storeCustomImageMetadata(stor); err != nil {
			return err
		}
	}

	// Populate the storage pools.
	if err = c.populateDefaultStoragePools(st); err != nil {
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

	return initiateMongoServer(peergrouper.InitiateMongoParams{
		DialInfo:       dialInfo,
		MemberHostPort: peerHostPort,
	}, true)
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
	current := version.Binary{
		Number: version.Current,
		Arch:   arch.HostArch(),
		Series: series.HostSeries(),
	}
	tools, err := agenttools.ReadTools(dataDir, current)
	if err != nil {
		return errors.Trace(err)
	}

	data, err := ioutil.ReadFile(filepath.Join(
		agenttools.SharedToolsDir(dataDir, current),
		"tools.tar.gz",
	))
	if err != nil {
		return errors.Trace(err)
	}

	storage, err := st.ToolsStorage()
	if err != nil {
		return errors.Trace(err)
	}
	defer storage.Close()

	var toolsVersions []version.Binary
	if strings.HasPrefix(tools.URL, "file://") {
		// Tools were uploaded: clone for each series of the same OS.
		os, err := series.GetOSFromSeries(tools.Version.Series)
		if err != nil {
			return errors.Trace(err)
		}
		osSeries := series.OSSupportedSeries(os)
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
			return errors.Trace(err)
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
		logger.Debugf("storing %q in model storage (%d bytes)", relpath, info.Size())
		return stor.Put(relpath, f, info.Size())
	})
}

// Override for testing.
var seriesFromVersion = series.VersionSeries

// saveCustomImageMetadata reads the custom image metadata from disk,
// and saves it in controller.
func (c *BootstrapCommand) saveCustomImageMetadata(st *state.State, env environs.Environ) error {
	logger.Debugf("saving custom image metadata from %q", c.ImageMetadataDir)
	baseURL := fmt.Sprintf("file://%s", filepath.ToSlash(c.ImageMetadataDir))
	publicKey, _ := simplestreams.UserPublicSigningKey()
	datasource := simplestreams.NewURLSignedDataSource("custom", baseURL, publicKey, utils.NoVerifySSLHostnames, simplestreams.CUSTOM_CLOUD_DATA, false)
	return storeImageMetadataFromFiles(st, env, datasource)
}

// storeImageMetadataFromFiles puts image metadata found in sources into state.
// Declared as var to facilitate tests.
var storeImageMetadataFromFiles = func(st *state.State, env environs.Environ, source simplestreams.DataSource) error {
	// Read the image metadata, as we'll want to upload it to the environment.
	imageConstraint := imagemetadata.NewImageConstraint(simplestreams.LookupParams{})
	if inst, ok := env.(simplestreams.HasRegion); ok {
		// If we can determine current region,
		// we want only metadata specific to this region.
		cloud, err := inst.Region()
		if err != nil {
			return err
		}
		imageConstraint.CloudSpec = cloud
	}

	existingMetadata, info, err := imagemetadata.Fetch([]simplestreams.DataSource{source}, imageConstraint)
	if err != nil && !errors.IsNotFound(err) {
		return errors.Annotate(err, "cannot read image metadata")
	}
	return storeImageMetadataInState(st, info.Source, source.Priority(), existingMetadata)
}

// storeImageMetadataInState writes image metadata into state store.
func storeImageMetadataInState(st *state.State, source string, priority int, existingMetadata []*imagemetadata.ImageMetadata) error {
	if len(existingMetadata) == 0 {
		return nil
	}
	metadataState := make([]cloudimagemetadata.Metadata, len(existingMetadata))
	for i, one := range existingMetadata {
		m := cloudimagemetadata.Metadata{
			cloudimagemetadata.MetadataAttributes{
				Stream:          one.Stream,
				Region:          one.RegionName,
				Arch:            one.Arch,
				VirtType:        one.VirtType,
				RootStorageType: one.Storage,
				Source:          source,
			},
			priority,
			one.Id,
		}
		s, err := seriesFromVersion(one.Version)
		if err != nil {
			return errors.Annotatef(err, "cannot determine series for version %v", one.Version)
		}
		m.Series = s
		metadataState[i] = m
	}
	if err := st.CloudImageMetadataStorage.SaveMetadata(metadataState); err != nil {
		return errors.Annotatef(err, "cannot cache image metadata")
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

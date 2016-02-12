// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"fmt"
	"time"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/names"
	"github.com/juju/replicaset"
	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/arch"
	"github.com/juju/utils/series"
	gc "gopkg.in/check.v1"
	"gopkg.in/mgo.v2"

	"github.com/juju/juju/agent"
	agenttools "github.com/juju/juju/agent/tools"
	"github.com/juju/juju/apiserver/params"
	cmdutil "github.com/juju/juju/cmd/jujud/util"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/filestorage"
	envtesting "github.com/juju/juju/environs/testing"
	envtools "github.com/juju/juju/environs/tools"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
	coretools "github.com/juju/juju/tools"
	"github.com/juju/juju/version"
	"github.com/juju/juju/worker/peergrouper"
)

type patchingSuite interface {
	PatchValue(interface{}, interface{})
}

// InstallFakeEnsureMongo creates a new FakeEnsureMongo, patching
// out replicaset.CurrentConfig and cmdutil.EnsureMongoServer.
func InstallFakeEnsureMongo(suite patchingSuite) *FakeEnsureMongo {
	f := &FakeEnsureMongo{
		ServiceInstalled:    true,
		ReplicasetInitiated: true,
	}
	suite.PatchValue(&mongo.IsServiceInstalled, f.IsServiceInstalled)
	suite.PatchValue(&replicaset.CurrentConfig, f.CurrentConfig)
	suite.PatchValue(&cmdutil.EnsureMongoServer, f.EnsureMongo)
	return f
}

// FakeEnsureMongo provides test fakes for the functions used to
// initialise MongoDB.
type FakeEnsureMongo struct {
	EnsureCount         int
	InitiateCount       int
	DataDir             string
	OplogSize           int
	Info                state.StateServingInfo
	InitiateParams      peergrouper.InitiateMongoParams
	Err                 error
	ServiceInstalled    bool
	ReplicasetInitiated bool
}

func (f *FakeEnsureMongo) IsServiceInstalled() (bool, error) {
	return f.ServiceInstalled, nil
}

func (f *FakeEnsureMongo) CurrentConfig(*mgo.Session) (*replicaset.Config, error) {
	if f.ReplicasetInitiated {
		// Return a dummy replicaset config that's good enough to
		// indicate that the replicaset is initiated.
		return &replicaset.Config{
			Members: []replicaset.Member{{}},
		}, nil
	}
	return nil, errors.NotFoundf("replicaset")
}

func (f *FakeEnsureMongo) EnsureMongo(args mongo.EnsureServerParams) error {
	f.EnsureCount++
	f.DataDir, f.OplogSize = args.DataDir, args.OplogSize
	f.Info = state.StateServingInfo{
		APIPort:        args.APIPort,
		StatePort:      args.StatePort,
		Cert:           args.Cert,
		PrivateKey:     args.PrivateKey,
		CAPrivateKey:   args.CAPrivateKey,
		SharedSecret:   args.SharedSecret,
		SystemIdentity: args.SystemIdentity,
	}
	return f.Err
}

func (f *FakeEnsureMongo) MaybeInitiateMongo(p peergrouper.InitiateMongoParams) error {
	return f.InitiateMongo(p, false)
}

func (f *FakeEnsureMongo) InitiateMongo(p peergrouper.InitiateMongoParams, force bool) error {
	f.InitiateCount++
	f.InitiateParams = p
	return nil
}

// agentSuite is a fixture to be used by agent test suites.
type AgentSuite struct {
	oldRestartDelay time.Duration
	testing.JujuConnSuite
}

// PrimeAgent writes the configuration file and tools for an agent
// with the given entity name. It returns the agent's configuration and the
// current tools.
func (s *AgentSuite) PrimeAgent(c *gc.C, tag names.Tag, password string) (agent.ConfigSetterWriter, *coretools.Tools) {
	vers := version.Binary{
		Number: version.Current,
		Arch:   arch.HostArch(),
		Series: series.HostSeries(),
	}
	return s.PrimeAgentVersion(c, tag, password, vers)
}

// PrimeAgentVersion writes the configuration file and tools with version
// vers for an agent with the given entity name. It returns the agent's
// configuration and the current tools.
func (s *AgentSuite) PrimeAgentVersion(c *gc.C, tag names.Tag, password string, vers version.Binary) (agent.ConfigSetterWriter, *coretools.Tools) {
	c.Logf("priming agent %s", tag.String())
	stor, err := filestorage.NewFileStorageWriter(c.MkDir())
	c.Assert(err, jc.ErrorIsNil)
	agentTools := envtesting.PrimeTools(c, stor, s.DataDir(), "released", vers)
	err = envtools.MergeAndWriteMetadata(stor, "released", "released", coretools.List{agentTools}, envtools.DoNotWriteMirrors)
	tools1, err := agenttools.ChangeAgentTools(s.DataDir(), tag.String(), vers)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(tools1, gc.DeepEquals, agentTools)

	stateInfo := s.MongoInfo(c)
	apiInfo := s.APIInfo(c)
	paths := agent.DefaultPaths
	paths.DataDir = s.DataDir()
	conf, err := agent.NewAgentConfig(
		agent.AgentConfigParams{
			Paths:             paths,
			Tag:               tag,
			UpgradedToVersion: vers.Number,
			Password:          password,
			Nonce:             agent.BootstrapNonce,
			StateAddresses:    stateInfo.Addrs,
			APIAddresses:      apiInfo.Addrs,
			CACert:            stateInfo.CACert,
			Model:             apiInfo.ModelTag,
		})
	c.Assert(err, jc.ErrorIsNil)
	conf.SetPassword(password)
	c.Assert(conf.Write(), gc.IsNil)
	s.primeAPIHostPorts(c)
	return conf, agentTools
}

// PrimeStateAgent writes the configuration file and tools for
// a state agent with the given entity name. It returns the agent's
// configuration and the current tools.
func (s *AgentSuite) PrimeStateAgent(c *gc.C, tag names.Tag, password string) (agent.ConfigSetterWriter, *coretools.Tools) {
	vers := version.Binary{
		Number: version.Current,
		Arch:   arch.HostArch(),
		Series: series.HostSeries(),
	}
	return s.PrimeStateAgentVersion(c, tag, password, vers)
}

// PrimeStateAgentVersion writes the configuration file and tools with
// version vers for a state agent with the given entity name. It
// returns the agent's configuration and the current tools.
func (s *AgentSuite) PrimeStateAgentVersion(c *gc.C, tag names.Tag, password string, vers version.Binary) (
	agent.ConfigSetterWriter, *coretools.Tools,
) {
	stor, err := filestorage.NewFileStorageWriter(c.MkDir())
	c.Assert(err, jc.ErrorIsNil)
	agentTools := envtesting.PrimeTools(c, stor, s.DataDir(), "released", vers)
	tools1, err := agenttools.ChangeAgentTools(s.DataDir(), tag.String(), vers)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(tools1, gc.DeepEquals, agentTools)

	conf := s.WriteStateAgentConfig(c, tag, password, vers, s.State.ModelTag())
	s.primeAPIHostPorts(c)
	return conf, agentTools
}

// WriteStateAgentConfig creates and writes a state agent config.
func (s *AgentSuite) WriteStateAgentConfig(
	c *gc.C,
	tag names.Tag,
	password string,
	vers version.Binary,
	modelTag names.ModelTag,
) agent.ConfigSetterWriter {
	stateInfo := s.State.MongoConnectionInfo()
	apiPort := gitjujutesting.FindTCPPort()
	apiAddr := []string{fmt.Sprintf("localhost:%d", apiPort)}
	conf, err := agent.NewStateMachineConfig(
		agent.AgentConfigParams{
			Paths:             agent.NewPathsWithDefaults(agent.Paths{DataDir: s.DataDir()}),
			Tag:               tag,
			UpgradedToVersion: vers.Number,
			Password:          password,
			Nonce:             agent.BootstrapNonce,
			StateAddresses:    stateInfo.Addrs,
			APIAddresses:      apiAddr,
			CACert:            stateInfo.CACert,
			Model:             modelTag,
		},
		params.StateServingInfo{
			Cert:         coretesting.ServerCert,
			PrivateKey:   coretesting.ServerKey,
			CAPrivateKey: coretesting.CAKey,
			StatePort:    gitjujutesting.MgoServer.Port(),
			APIPort:      apiPort,
		})
	c.Assert(err, jc.ErrorIsNil)
	conf.SetPassword(password)
	c.Assert(conf.Write(), gc.IsNil)
	return conf
}

func (s *AgentSuite) primeAPIHostPorts(c *gc.C) {
	apiInfo := s.APIInfo(c)

	c.Assert(apiInfo.Addrs, gc.HasLen, 1)
	hostPorts, err := network.ParseHostPorts(apiInfo.Addrs[0])
	c.Assert(err, jc.ErrorIsNil)

	err = s.State.SetAPIHostPorts([][]network.HostPort{hostPorts})
	c.Assert(err, jc.ErrorIsNil)

	c.Logf("api host ports primed %#v", hostPorts)
}

// InitAgent initialises the given agent command with additional
// arguments as provided.
func (s *AgentSuite) InitAgent(c *gc.C, a cmd.Command, args ...string) {
	args = append([]string{"--data-dir", s.DataDir()}, args...)
	err := coretesting.InitCommand(a, args)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *AgentSuite) AssertCanOpenState(c *gc.C, tag names.Tag, dataDir string) {
	config, err := agent.ReadConfig(agent.ConfigPath(dataDir, tag))
	c.Assert(err, jc.ErrorIsNil)
	info, ok := config.MongoInfo()
	c.Assert(ok, jc.IsTrue)
	st, err := state.Open(config.Model(), info, mongo.DefaultDialOpts(), environs.NewStatePolicy())
	c.Assert(err, jc.ErrorIsNil)
	st.Close()
}

func (s *AgentSuite) AssertCannotOpenState(c *gc.C, tag names.Tag, dataDir string) {
	config, err := agent.ReadConfig(agent.ConfigPath(dataDir, tag))
	c.Assert(err, jc.ErrorIsNil)
	_, ok := config.MongoInfo()
	c.Assert(ok, jc.IsFalse)
}

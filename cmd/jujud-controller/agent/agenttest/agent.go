// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agenttest

import (
	"context"
	"fmt"

	"github.com/juju/clock"
	"github.com/juju/mgo/v3"
	mgotesting "github.com/juju/mgo/v3/testing"
	"github.com/juju/names/v6"
	"github.com/juju/replicaset/v3"
	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"

	"github.com/juju/juju/agent"
	agenttools "github.com/juju/juju/agent/tools"
	cmdutil "github.com/juju/juju/cmd/jujud-controller/util"
	"github.com/juju/juju/controller"
	coredatabase "github.com/juju/juju/core/database"
	modeltesting "github.com/juju/juju/core/model/testing"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/filestorage"
	"github.com/juju/juju/environs/simplestreams"
	sstesting "github.com/juju/juju/environs/simplestreams/testing"
	envtesting "github.com/juju/juju/environs/testing"
	envtools "github.com/juju/juju/environs/tools"
	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	"github.com/juju/juju/internal/database"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/mongo"
	"github.com/juju/juju/internal/mongo/mongotest"
	coretesting "github.com/juju/juju/internal/testing"
	coretools "github.com/juju/juju/internal/tools"
	"github.com/juju/juju/internal/worker/peergrouper"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/stateenvirons"
	statetesting "github.com/juju/juju/state/testing"
)

type patchingSuite interface {
	PatchValue(interface{}, interface{})
}

// InstallFakeEnsureMongo creates a new FakeEnsureMongo, patching
// out replicaset.CurrentConfig and cmdutil.EnsureMongoServerInstalled/Started.
func InstallFakeEnsureMongo(suite patchingSuite, dataDir string) *FakeEnsureMongo {
	f := &FakeEnsureMongo{}
	suite.PatchValue(&mongo.CurrentReplicasetConfig, f.CurrentConfig)
	suite.PatchValue(&cmdutil.EnsureMongoServerInstalled, f.EnsureMongo)
	ensureParams := cmdutil.NewEnsureMongoParams
	suite.PatchValue(&cmdutil.NewEnsureMongoParams, func(agentConfig agent.Config) (mongo.EnsureServerParams, error) {
		params, err := ensureParams(agentConfig)
		if err == nil {
			params.MongoDataDir = dataDir
		}
		return params, err
	})
	return f
}

// FakeEnsureMongo provides test fakes for the functions used to
// initialise MongoDB.
type FakeEnsureMongo struct {
	EnsureCount    int
	InitiateCount  int
	MongoDataDir   string
	OplogSize      int
	Info           controller.StateServingInfo
	InitiateParams peergrouper.InitiateMongoParams
	Err            error
}

func (f *FakeEnsureMongo) CurrentConfig(*mgo.Session) (*replicaset.Config, error) {
	// Return a dummy replicaset config that's good enough to
	// indicate that the replicaset is initiated.
	return &replicaset.Config{
		Members: []replicaset.Member{{}},
	}, nil
}

func (f *FakeEnsureMongo) EnsureMongo(_ context.Context, args mongo.EnsureServerParams) error {
	f.EnsureCount++
	f.MongoDataDir, f.OplogSize = args.MongoDataDir, args.OplogSize
	f.Info = controller.StateServingInfo{
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

func (f *FakeEnsureMongo) InitiateMongo(p peergrouper.InitiateMongoParams) error {
	f.InitiateCount++
	f.InitiateParams = p
	return nil
}

// AgentSuite is a fixture to be used by agent test suites.
type AgentSuite struct {
	testing.ApiServerSuite

	Environ environs.Environ
	DataDir string
	LogDir  string
}

func (s *AgentSuite) SetUpTest(c *tc.C) {
	s.ApiServerSuite.SetUpTest(c)

	domainServices := s.ControllerDomainServices(c)

	var err error
	s.Environ, err = stateenvirons.GetNewEnvironFunc(environs.New)(
		s.ControllerModel(c), domainServices.Cloud(), domainServices.Credential(), domainServices.Config())
	c.Assert(err, jc.ErrorIsNil)

	s.DataDir = c.MkDir()
	s.LogDir = c.MkDir()
}

func mongoInfo() *mongo.MongoInfo {
	info := statetesting.NewMongoInfo()
	info.Password = testing.AdminSecret
	return info
}

// PrimeAgent writes the configuration file and tools for an agent
// with the given entity name. It returns the agent's configuration and the
// current tools.
func (s *AgentSuite) PrimeAgent(c *tc.C, tag names.Tag, password string) (agent.ConfigSetterWriter, *coretools.Tools) {
	vers := coretesting.CurrentVersion()
	return s.PrimeAgentVersion(c, tag, password, vers)
}

// PrimeAgentVersion writes the configuration file and tools with version
// vers for an agent with the given entity name. It returns the agent's
// configuration and the current tools.
func (s *AgentSuite) PrimeAgentVersion(c *tc.C, tag names.Tag, password string, vers semversion.Binary) (agent.ConfigSetterWriter, *coretools.Tools) {
	c.Logf("priming agent %s", tag.String())

	store, err := filestorage.NewFileStorageWriter(c.MkDir())
	c.Assert(err, jc.ErrorIsNil)

	agentTools := envtesting.PrimeTools(c, store, s.DataDir, "released", vers)
	ss := simplestreams.NewSimpleStreams(sstesting.TestDataSourceFactory())
	err = envtools.MergeAndWriteMetadata(context.Background(), ss, store, "released", "released", coretools.List{agentTools}, envtools.DoNotWriteMirrors)
	c.Assert(err, jc.ErrorIsNil)

	tools1, err := agenttools.ChangeAgentTools(s.DataDir, tag.String(), vers)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(tools1, tc.DeepEquals, agentTools)

	stateInfo := mongoInfo()
	apiInfo := s.ControllerModelApiInfo()

	paths := agent.DefaultPaths
	paths.DataDir = s.DataDir
	paths.TransientDataDir = c.MkDir()
	paths.LogDir = s.LogDir
	paths.MetricsSpoolDir = c.MkDir()

	dqlitePort := mgotesting.FindTCPPort()

	conf, err := agent.NewAgentConfig(
		agent.AgentConfigParams{
			Paths:             paths,
			Tag:               tag,
			UpgradedToVersion: vers.Number,
			Password:          password,
			Nonce:             agent.BootstrapNonce,
			APIAddresses:      apiInfo.Addrs,
			CACert:            stateInfo.CACert,
			Controller:        coretesting.ControllerTag,
			Model:             apiInfo.ModelTag,

			QueryTracingEnabled:   controller.DefaultQueryTracingEnabled,
			QueryTracingThreshold: controller.DefaultQueryTracingThreshold,

			OpenTelemetryEnabled:               controller.DefaultOpenTelemetryEnabled,
			OpenTelemetryEndpoint:              "",
			OpenTelemetryInsecure:              controller.DefaultOpenTelemetryInsecure,
			OpenTelemetryStackTraces:           controller.DefaultOpenTelemetryStackTraces,
			OpenTelemetrySampleRatio:           controller.DefaultOpenTelemetrySampleRatio,
			OpenTelemetryTailSamplingThreshold: controller.DefaultOpenTelemetryTailSamplingThreshold,

			ObjectStoreType: controller.DefaultObjectStoreType,

			DqlitePort: dqlitePort,
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	conf.SetPassword(password)
	c.Assert(conf.Write(), tc.IsNil)

	s.primeAPIHostPorts(c)
	return conf, agentTools
}

// PrimeStateAgentVersion writes the configuration file and tools with
// version vers for a state agent with the given entity name. It
// returns the agent's configuration and the current tools.
func (s *AgentSuite) PrimeStateAgentVersion(c *tc.C, tag names.Tag, password string, vers semversion.Binary) (
	agent.ConfigSetterWriter, *coretools.Tools,
) {
	stor, err := filestorage.NewFileStorageWriter(c.MkDir())
	c.Assert(err, jc.ErrorIsNil)

	agentTools := envtesting.PrimeTools(c, stor, s.DataDir, "released", vers)
	tools1, err := agenttools.ChangeAgentTools(s.DataDir, tag.String(), vers)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(tools1, tc.DeepEquals, agentTools)

	domainServices := s.ControllerDomainServices(c)
	cfg, err := domainServices.ControllerConfig().ControllerConfig(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	apiPort, ok := cfg[controller.APIPort].(int)
	if !ok {
		c.Fatalf("no api port in controller config")
	}
	conf := s.WriteStateAgentConfig(c, tag, password, vers, names.NewModelTag(s.ControllerModelUUID()), apiPort)
	s.primeAPIHostPorts(c)

	err = database.BootstrapDqlite(
		context.Background(),
		database.NewNodeManager(conf, true, loggertesting.WrapCheckLog(c), coredatabase.NoopSlowQueryLogger{}),
		modeltesting.GenModelUUID(c),
		loggertesting.WrapCheckLog(c),
	)
	c.Assert(err, jc.ErrorIsNil)

	return conf, agentTools
}

// WriteStateAgentConfig creates and writes a state agent config.
func (s *AgentSuite) WriteStateAgentConfig(
	c *tc.C,
	tag names.Tag,
	password string,
	vers semversion.Binary,
	modelTag names.ModelTag,
	apiPort int,
) agent.ConfigSetterWriter {
	stateInfo := mongoInfo()
	apiAddr := []string{fmt.Sprintf("localhost:%d", apiPort)}
	dqlitePort := mgotesting.FindTCPPort()
	conf, err := agent.NewStateMachineConfig(
		agent.AgentConfigParams{
			Paths: agent.NewPathsWithDefaults(agent.Paths{
				DataDir: s.DataDir,
				LogDir:  s.LogDir,
			}),
			Tag:                   tag,
			UpgradedToVersion:     vers.Number,
			Password:              password,
			Nonce:                 agent.BootstrapNonce,
			APIAddresses:          apiAddr,
			CACert:                stateInfo.CACert,
			Controller:            s.ControllerModel(c).ControllerTag(),
			Model:                 modelTag,
			QueryTracingEnabled:   controller.DefaultQueryTracingEnabled,
			QueryTracingThreshold: controller.DefaultQueryTracingThreshold,

			OpenTelemetryEnabled:               controller.DefaultOpenTelemetryEnabled,
			OpenTelemetryEndpoint:              "",
			OpenTelemetryInsecure:              controller.DefaultOpenTelemetryInsecure,
			OpenTelemetryStackTraces:           controller.DefaultOpenTelemetryStackTraces,
			OpenTelemetrySampleRatio:           controller.DefaultOpenTelemetrySampleRatio,
			OpenTelemetryTailSamplingThreshold: controller.DefaultOpenTelemetryTailSamplingThreshold,

			ObjectStoreType: controller.DefaultObjectStoreType,

			DqlitePort: dqlitePort,
		},
		controller.StateServingInfo{
			Cert:         coretesting.ServerCert,
			PrivateKey:   coretesting.ServerKey,
			CAPrivateKey: coretesting.CAKey,
			StatePort:    mgotesting.MgoServer.Port(),
			APIPort:      apiPort,
		})
	c.Assert(err, jc.ErrorIsNil)

	conf.SetPassword(password)
	c.Assert(conf.Write(), tc.IsNil)

	return conf
}

func (s *AgentSuite) primeAPIHostPorts(c *tc.C) {
	apiInfo := s.ControllerModelApiInfo()

	c.Assert(apiInfo.Addrs, tc.HasLen, 1)
	mHP, err := network.ParseMachineHostPort(apiInfo.Addrs[0])
	c.Assert(err, jc.ErrorIsNil)

	hostPorts := network.SpaceHostPorts{
		{SpaceAddress: network.SpaceAddress{MachineAddress: mHP.MachineAddress}, NetPort: mHP.NetPort}}

	domainServices := s.ControllerDomainServices(c)
	controllerConfig, err := domainServices.ControllerConfig().ControllerConfig(context.Background())
	c.Assert(err, jc.ErrorIsNil)

	st := s.ControllerModel(c).State()

	err = st.SetAPIHostPorts(controllerConfig, []network.SpaceHostPorts{hostPorts}, []network.SpaceHostPorts{hostPorts})
	c.Assert(err, jc.ErrorIsNil)

	c.Logf("api host ports primed %#v", hostPorts)
}

// InitAgent initialises the given agent command with additional
// arguments as provided.
func (s *AgentSuite) InitAgent(c *tc.C, a cmd.Command, args ...string) {
	args = append([]string{"--data-dir", s.DataDir}, args...)
	err := cmdtesting.InitCommand(a, args)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *AgentSuite) AssertCanOpenState(c *tc.C, tag names.Tag, dataDir string) {
	config, err := agent.ReadConfig(agent.ConfigPath(dataDir, tag))
	c.Assert(err, jc.ErrorIsNil)

	info, ok := config.MongoInfo()
	c.Assert(ok, jc.IsTrue)

	session, err := mongo.DialWithInfo(*info, mongotest.DialOpts())
	c.Assert(err, jc.ErrorIsNil)
	defer session.Close()

	pool, err := state.OpenStatePool(state.OpenParams{
		Clock:              clock.WallClock,
		ControllerTag:      config.Controller(),
		ControllerModelTag: config.Model(),
		MongoSession:       session,
		NewPolicy:          stateenvirons.GetNewPolicyFunc(nil, nil, nil, nil),
	})
	c.Assert(err, jc.ErrorIsNil)
	_ = pool.Close()
}

func (s *AgentSuite) AssertCannotOpenState(c *tc.C, tag names.Tag, dataDir string) {
	config, err := agent.ReadConfig(agent.ConfigPath(dataDir, tag))
	c.Assert(err, jc.ErrorIsNil)

	_, ok := config.MongoInfo()
	c.Assert(ok, jc.IsFalse)
}

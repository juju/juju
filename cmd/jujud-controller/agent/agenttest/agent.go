// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agenttest

import (
	"fmt"
	"net"

	"github.com/juju/clock"
	"github.com/juju/names/v6"
	"github.com/juju/tc"

	"github.com/juju/juju/agent"
	agenttools "github.com/juju/juju/agent/tools"
	"github.com/juju/juju/controller"
	coredatabase "github.com/juju/juju/core/database"
	modeltesting "github.com/juju/juju/core/model/testing"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/domain/controllernode"
	"github.com/juju/juju/environs/filestorage"
	"github.com/juju/juju/environs/simplestreams"
	sstesting "github.com/juju/juju/environs/simplestreams/testing"
	envtesting "github.com/juju/juju/environs/testing"
	envtools "github.com/juju/juju/environs/tools"
	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	"github.com/juju/juju/internal/database"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	coretesting "github.com/juju/juju/internal/testing"
	coretools "github.com/juju/juju/internal/tools"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/stateenvirons"
)

// AgentSuite is a fixture to be used by agent test suites.
type AgentSuite struct {
	testing.ApiServerSuite

	DataDir string
	LogDir  string
}

func (s *AgentSuite) SetUpTest(c *tc.C) {
	s.ApiServerSuite.SetUpTest(c)

	s.DataDir = c.MkDir()
	s.LogDir = c.MkDir()
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
	c.Assert(err, tc.ErrorIsNil)

	agentTools := envtesting.PrimeTools(c, store, s.DataDir, "released", vers)
	ss := simplestreams.NewSimpleStreams(sstesting.TestDataSourceFactory())
	err = envtools.MergeAndWriteMetadata(c.Context(), ss, store, "released", "released", coretools.List{agentTools}, envtools.DoNotWriteMirrors)
	c.Assert(err, tc.ErrorIsNil)

	tools1, err := agenttools.ChangeAgentTools(s.DataDir, tag.String(), vers)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(tools1, tc.DeepEquals, agentTools)

	apiInfo := s.ControllerModelApiInfo()

	paths := agent.DefaultPaths
	paths.DataDir = s.DataDir
	paths.TransientDataDir = c.MkDir()
	paths.LogDir = s.LogDir
	paths.MetricsSpoolDir = c.MkDir()

	dqlitePort := findTCPPort()

	conf, err := agent.NewAgentConfig(
		agent.AgentConfigParams{
			Paths:             paths,
			Tag:               tag,
			UpgradedToVersion: vers.Number,
			Password:          password,
			Nonce:             agent.BootstrapNonce,
			APIAddresses:      apiInfo.Addrs,
			CACert:            coretesting.CACert,
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
	c.Assert(err, tc.ErrorIsNil)
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
	c.Assert(err, tc.ErrorIsNil)

	agentTools := envtesting.PrimeTools(c, stor, s.DataDir, "released", vers)
	tools1, err := agenttools.ChangeAgentTools(s.DataDir, tag.String(), vers)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(tools1, tc.DeepEquals, agentTools)

	domainServices := s.ControllerDomainServices(c)
	cfg, err := domainServices.ControllerConfig().ControllerConfig(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	apiPort, ok := cfg[controller.APIPort].(int)
	if !ok {
		c.Fatalf("no api port in controller config")
	}
	conf := s.WriteStateAgentConfig(c, tag, password, vers, names.NewModelTag(s.ControllerModelUUID()), apiPort)
	s.primeAPIHostPorts(c)

	err = database.BootstrapDqlite(
		c.Context(),
		database.NewNodeManager(conf, true, loggertesting.WrapCheckLog(c), coredatabase.NoopSlowQueryLogger{}),
		modeltesting.GenModelUUID(c),
		loggertesting.WrapCheckLog(c),
	)
	c.Assert(err, tc.ErrorIsNil)

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
	apiAddr := []string{fmt.Sprintf("localhost:%d", apiPort)}
	dqlitePort := findTCPPort()
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
			CACert:                coretesting.CACert,
			Controller:            names.NewControllerTag(s.ControllerUUID),
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
		controller.ControllerAgentInfo{
			Cert:         coretesting.ServerCert,
			PrivateKey:   coretesting.ServerKey,
			CAPrivateKey: coretesting.CAKey,
			APIPort:      apiPort,
		})
	c.Assert(err, tc.ErrorIsNil)

	conf.SetPassword(password)
	c.Assert(conf.Write(), tc.IsNil)

	return conf
}

func (s *AgentSuite) primeAPIHostPorts(c *tc.C) {
	apiInfo := s.ControllerModelApiInfo()

	c.Assert(apiInfo.Addrs, tc.HasLen, 1)
	mHP, err := network.ParseMachineHostPort(apiInfo.Addrs[0])
	c.Assert(err, tc.ErrorIsNil)

	hostPorts := network.SpaceHostPorts{
		{SpaceAddress: network.SpaceAddress{MachineAddress: mHP.MachineAddress}, NetPort: mHP.NetPort}}

	apiAddrArgs := controllernode.SetAPIAddressArgs{
		APIAddresses: map[string]network.SpaceHostPorts{
			"0": hostPorts,
		},
	}

	domainServices := s.ControllerDomainServices(c)
	controllerNodeService := domainServices.ControllerNode()

	err = controllerNodeService.SetAPIAddresses(c.Context(), apiAddrArgs)
	c.Assert(err, tc.ErrorIsNil)

	c.Logf("api host ports primed %#v", hostPorts)
}

// InitAgent initialises the given agent command with additional
// arguments as provided.
func (s *AgentSuite) InitAgent(c *tc.C, a cmd.Command, args ...string) {
	args = append([]string{"--data-dir", s.DataDir}, args...)
	err := cmdtesting.InitCommand(a, args)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *AgentSuite) AssertCanOpenState(c *tc.C, tag names.Tag, dataDir string) {
	config, err := agent.ReadConfig(agent.ConfigPath(dataDir, tag))
	c.Assert(err, tc.ErrorIsNil)

	pool, err := state.OpenStatePool(state.OpenParams{
		Clock:              clock.WallClock,
		ControllerTag:      config.Controller(),
		ControllerModelTag: config.Model(),
		NewPolicy:          stateenvirons.GetNewPolicyFunc(nil),
	})
	c.Assert(err, tc.ErrorIsNil)
	_ = pool.Close()
}

func (s *AgentSuite) AssertCannotOpenState(c *tc.C, tag names.Tag, dataDir string) {
	config, err := agent.ReadConfig(agent.ConfigPath(dataDir, tag))
	c.Assert(err, tc.ErrorIsNil)

	_, ok := config.MongoInfo()
	c.Assert(ok, tc.IsFalse)
}

// findTCPPort finds an unused TCP port and returns it.
// Use of this function has an inherent race condition - another
// process may claim the port before we try to use it.
// We hope that the probability is small enough during
// testing to be negligible.
func findTCPPort() int {
	l, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		panic(err)
	}
	l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

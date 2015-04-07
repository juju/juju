// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	"fmt"
	"time"

	"github.com/juju/cmd"
	"github.com/juju/names"
	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent"
	agenttools "github.com/juju/juju/agent/tools"
	"github.com/juju/juju/api"
	apienvironment "github.com/juju/juju/api/environment"
	"github.com/juju/juju/apiserver/params"
	agenttesting "github.com/juju/juju/cmd/jujud/agent/testing"
	cmdutil "github.com/juju/juju/cmd/jujud/util"
	"github.com/juju/juju/environs/filestorage"
	envtesting "github.com/juju/juju/environs/testing"
	"github.com/juju/juju/juju/paths"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/multiwatcher"
	coretesting "github.com/juju/juju/testing"
	coretools "github.com/juju/juju/tools"
	"github.com/juju/juju/version"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/proxyupdater"
)

var (
	_ = gc.Suite(&apiOpenSuite{})
)

type apiOpenSuite struct{ coretesting.BaseSuite }

func (s *apiOpenSuite) SetUpTest(c *gc.C) {
	s.PatchValue(&checkProvisionedStrategy, utils.AttemptStrategy{})
}

func (s *apiOpenSuite) TestOpenAPIStateReplaceErrors(c *gc.C) {
	type replaceErrors struct {
		openErr    error
		replaceErr error
	}
	var apiError error
	s.PatchValue(&apiOpen, func(info *api.Info, opts api.DialOpts) (*api.State, error) {
		return nil, apiError
	})
	errReplacePairs := []replaceErrors{{
		fmt.Errorf("blah"), nil,
	}, {
		openErr:    &params.Error{Code: params.CodeNotProvisioned},
		replaceErr: worker.ErrTerminateAgent,
	}, {
		openErr:    &params.Error{Code: params.CodeUnauthorized},
		replaceErr: worker.ErrTerminateAgent,
	}}
	for i, test := range errReplacePairs {
		c.Logf("test %d", i)
		apiError = test.openErr
		_, _, err := OpenAPIState(fakeAPIOpenConfig{}, nil)
		if test.replaceErr == nil {
			c.Check(err, gc.Equals, test.openErr)
		} else {
			c.Check(err, gc.Equals, test.replaceErr)
		}
	}
}

func (s *apiOpenSuite) TestOpenAPIStateWaitsProvisioned(c *gc.C) {
	s.PatchValue(&checkProvisionedStrategy.Min, 5)
	var called int
	s.PatchValue(&apiOpen, func(info *api.Info, opts api.DialOpts) (*api.State, error) {
		called++
		if called == checkProvisionedStrategy.Min-1 {
			return nil, &params.Error{Code: params.CodeUnauthorized}
		}
		return nil, &params.Error{Code: params.CodeNotProvisioned}
	})
	_, _, err := OpenAPIState(fakeAPIOpenConfig{}, nil)
	c.Assert(err, gc.Equals, worker.ErrTerminateAgent)
	c.Assert(called, gc.Equals, checkProvisionedStrategy.Min-1)
}

func (s *apiOpenSuite) TestOpenAPIStateWaitsProvisionedGivesUp(c *gc.C) {
	s.PatchValue(&checkProvisionedStrategy.Min, 5)
	var called int
	s.PatchValue(&apiOpen, func(info *api.Info, opts api.DialOpts) (*api.State, error) {
		called++
		return nil, &params.Error{Code: params.CodeNotProvisioned}
	})
	_, _, err := OpenAPIState(fakeAPIOpenConfig{}, nil)
	c.Assert(err, gc.Equals, worker.ErrTerminateAgent)
	// +1 because we always attempt at least once outside the attempt strategy
	// (twice if the API server initially returns CodeUnauthorized.)
	c.Assert(called, gc.Equals, checkProvisionedStrategy.Min+1)
}

type acCreator func() (cmd.Command, *AgentConf)

// CheckAgentCommand is a utility function for verifying that common agent
// options are handled by a Command; it returns an instance of that
// command pre-parsed, with any mandatory flags added.
func CheckAgentCommand(c *gc.C, create acCreator, args []string) cmd.Command {
	com, conf := create()
	err := coretesting.InitCommand(com, args)
	dataDir, err := paths.DataDir(version.Current.Series)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(conf.DataDir, gc.Equals, dataDir)
	badArgs := append(args, "--data-dir", "")
	com, _ = create()
	err = coretesting.InitCommand(com, badArgs)
	c.Assert(err, gc.ErrorMatches, "--data-dir option must be set")

	args = append(args, "--data-dir", "jd")
	com, conf = create()
	c.Assert(coretesting.InitCommand(com, args), gc.IsNil)
	c.Assert(conf.DataDir, gc.Equals, "jd")
	return com
}

// ParseAgentCommand is a utility function that inserts the always-required args
// before parsing an agent command and returning the result.
func ParseAgentCommand(ac cmd.Command, args []string) error {
	common := []string{
		"--data-dir", "jd",
	}
	return coretesting.InitCommand(ac, append(common, args...))
}

// AgentSuite is a fixture to be used by agent test suites.
type AgentSuite struct {
	agenttesting.AgentSuite
	oldRestartDelay time.Duration
}

func (s *AgentSuite) SetUpSuite(c *gc.C) {
	s.JujuConnSuite.SetUpSuite(c)

	s.oldRestartDelay = worker.RestartDelay
	// We could use testing.ShortWait, but this thrashes quite
	// a bit when some tests are restarting every 50ms for 10 seconds,
	// so use a slightly more friendly delay.
	worker.RestartDelay = 250 * time.Millisecond
	s.PatchValue(&cmdutil.EnsureMongoServer, func(mongo.EnsureServerParams) error {
		return nil
	})
}

func (s *AgentSuite) TearDownSuite(c *gc.C) {
	s.JujuConnSuite.TearDownSuite(c)
	worker.RestartDelay = s.oldRestartDelay
}

func (s *AgentSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	// Set API host ports so FindTools/Tools API calls succeed.
	hostPorts := [][]network.HostPort{
		network.NewHostPorts(1234, "0.1.2.3"),
	}
	err := s.State.SetAPIHostPorts(hostPorts)
	c.Assert(err, jc.ErrorIsNil)
	s.PatchValue(&proxyupdater.New, func(*apienvironment.Facade, bool) worker.Worker {
		return newDummyWorker()
	})
}

func (s *AgentSuite) primeAPIHostPorts(c *gc.C) {
	apiInfo := s.APIInfo(c)

	c.Assert(apiInfo.Addrs, gc.HasLen, 1)
	hostPorts, err := network.ParseHostPorts(apiInfo.Addrs[0])
	c.Assert(err, jc.ErrorIsNil)

	err = s.State.SetAPIHostPorts([][]network.HostPort{hostPorts})
	c.Assert(err, jc.ErrorIsNil)

	logger.Debugf("api host ports primed %#v", hostPorts)
}

// primeStateAgent writes the configuration file and tools with version vers
// for an agent with the given entity name.  It returns the agent's configuration
// and the current tools.
func (s *AgentSuite) PrimeStateAgent(
	c *gc.C, tag names.Tag, password string, vers version.Binary) (agent.ConfigSetterWriter, *coretools.Tools) {

	stor, err := filestorage.NewFileStorageWriter(c.MkDir())
	c.Assert(err, jc.ErrorIsNil)
	agentTools := envtesting.PrimeTools(c, stor, s.DataDir(), "released", vers)
	tools1, err := agenttools.ChangeAgentTools(s.DataDir(), tag.String(), vers)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(tools1, gc.DeepEquals, agentTools)

	stateInfo := s.MongoInfo(c)
	conf := writeStateAgentConfig(c, stateInfo, s.DataDir(), tag, password, vers, s.State.EnvironTag())
	s.primeAPIHostPorts(c)
	return conf, agentTools
}

func (s *AgentSuite) RunTestOpenAPIState(c *gc.C, ent state.AgentEntity, agentCmd Agent, initialPassword string) {
	conf, err := agent.ReadConfig(agent.ConfigPath(s.DataDir(), ent.Tag()))
	c.Assert(err, jc.ErrorIsNil)

	conf.SetPassword("")
	err = conf.Write()
	c.Assert(err, jc.ErrorIsNil)

	// Check that it starts initially and changes the password
	assertOpen := func(conf agent.Config) {
		st, gotEnt, err := OpenAPIState(conf, agentCmd)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(st, gc.NotNil)
		st.Close()
		c.Assert(gotEnt.Tag(), gc.Equals, ent.Tag().String())
	}
	assertOpen(conf)

	// Check that the initial password is no longer valid.
	err = ent.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ent.PasswordValid(initialPassword), jc.IsFalse)

	// Read the configuration and check that we can connect with it.
	conf, err = agent.ReadConfig(agent.ConfigPath(conf.DataDir(), conf.Tag()))
	c.Assert(err, gc.IsNil)
	// Check we can open the API with the new configuration.
	assertOpen(conf)
}

// writeStateAgentConfig creates and writes a state agent config.
func writeStateAgentConfig(
	c *gc.C, stateInfo *mongo.MongoInfo, dataDir string, tag names.Tag,
	password string, vers version.Binary, envTag names.EnvironTag) agent.ConfigSetterWriter {
	port := gitjujutesting.FindTCPPort()
	apiAddr := []string{fmt.Sprintf("localhost:%d", port)}
	conf, err := agent.NewStateMachineConfig(
		agent.AgentConfigParams{
			DataDir:           dataDir,
			Tag:               tag,
			UpgradedToVersion: vers.Number,
			Password:          password,
			Nonce:             agent.BootstrapNonce,
			StateAddresses:    stateInfo.Addrs,
			APIAddresses:      apiAddr,
			CACert:            stateInfo.CACert,
			Environment:       envTag,
		},
		params.StateServingInfo{
			Cert:         coretesting.ServerCert,
			PrivateKey:   coretesting.ServerKey,
			CAPrivateKey: coretesting.CAKey,
			StatePort:    gitjujutesting.MgoServer.Port(),
			APIPort:      port,
		})
	c.Assert(err, jc.ErrorIsNil)
	conf.SetPassword(password)
	c.Assert(conf.Write(), gc.IsNil)
	return conf
}

type fakeAPIOpenConfig struct{ agent.Config }

func (fakeAPIOpenConfig) APIInfo() *api.Info              { return &api.Info{} }
func (fakeAPIOpenConfig) OldPassword() string             { return "old" }
func (fakeAPIOpenConfig) Jobs() []multiwatcher.MachineJob { return []multiwatcher.MachineJob{} }

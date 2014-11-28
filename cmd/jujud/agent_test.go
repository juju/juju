// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	stderrors "errors"
	"fmt"
	"net"
	"strconv"
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
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/filestorage"
	envtesting "github.com/juju/juju/environs/testing"
	envtools "github.com/juju/juju/environs/tools"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/multiwatcher"
	coretesting "github.com/juju/juju/testing"
	coretools "github.com/juju/juju/tools"
	"github.com/juju/juju/version"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/upgrader"
)

var _ = gc.Suite(&toolSuite{})

type toolSuite struct {
	coretesting.BaseSuite
}

var errorImportanceTests = []error{
	nil,
	stderrors.New("foo"),
	&upgrader.UpgradeReadyError{},
	worker.ErrTerminateAgent,
}

func (*toolSuite) TestErrorImportance(c *gc.C) {
	for i, err0 := range errorImportanceTests {
		for j, err1 := range errorImportanceTests {
			c.Assert(moreImportant(err0, err1), gc.Equals, i > j)
		}
	}
}

var isFatalTests = []struct {
	err     error
	isFatal bool
}{{
	err:     worker.ErrTerminateAgent,
	isFatal: true,
}, {
	err:     &upgrader.UpgradeReadyError{},
	isFatal: true,
}, {
	err: &params.Error{
		Message: "blah",
		Code:    params.CodeNotProvisioned,
	},
	isFatal: false,
}, {
	err:     &fatalError{"some fatal error"},
	isFatal: true,
}, {
	err:     stderrors.New("foo"),
	isFatal: false,
}, {
	err: &params.Error{
		Message: "blah",
		Code:    params.CodeNotFound,
	},
	isFatal: false,
}}

func (*toolSuite) TestIsFatal(c *gc.C) {
	for i, test := range isFatalTests {
		c.Logf("test %d: %s", i, test.err)
		c.Assert(isFatal(test.err), gc.Equals, test.isFatal)
	}
}

type apiOpenSuite struct {
	coretesting.BaseSuite
}

type fakeAPIOpenConfig struct {
	agent.Config
}

func (fakeAPIOpenConfig) APIInfo() *api.Info {
	return &api.Info{}
}

func (fakeAPIOpenConfig) OldPassword() string {
	return "old"
}

func (fakeAPIOpenConfig) Jobs() []multiwatcher.MachineJob {
	return []multiwatcher.MachineJob{}
}

var _ = gc.Suite(&apiOpenSuite{})

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
		_, _, err := openAPIState(fakeAPIOpenConfig{}, nil)
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
	_, _, err := openAPIState(fakeAPIOpenConfig{}, nil)
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
	_, _, err := openAPIState(fakeAPIOpenConfig{}, nil)
	c.Assert(err, gc.Equals, worker.ErrTerminateAgent)
	// +1 because we always attempt at least once outside the attempt strategy
	// (twice if the API server initially returns CodeUnauthorized.)
	c.Assert(called, gc.Equals, checkProvisionedStrategy.Min+1)
}

type testPinger func() error

func (f testPinger) Ping() error {
	return f()
}

func (s *toolSuite) TestConnectionIsFatal(c *gc.C) {
	var (
		errPinger testPinger = func() error {
			return stderrors.New("ping error")
		}
		okPinger testPinger = func() error {
			return nil
		}
	)
	for i, pinger := range []testPinger{errPinger, okPinger} {
		for j, test := range isFatalTests {
			c.Logf("test %d.%d: %s", i, j, test.err)
			fatal := connectionIsFatal(pinger)(test.err)
			if test.isFatal {
				c.Check(fatal, jc.IsTrue)
			} else {
				c.Check(fatal, gc.Equals, i == 0)
			}
		}
	}
}

func mkTools(s string) *coretools.Tools {
	return &coretools.Tools{
		Version: version.MustParseBinary(s + "-foo-bar"),
	}
}

type acCreator func() (cmd.Command, *AgentConf)

// CheckAgentCommand is a utility function for verifying that common agent
// options are handled by a Command; it returns an instance of that
// command pre-parsed, with any mandatory flags added.
func CheckAgentCommand(c *gc.C, create acCreator, args []string) cmd.Command {
	com, conf := create()
	err := coretesting.InitCommand(com, args)
	c.Assert(conf.dataDir, gc.Equals, "/var/lib/juju")
	badArgs := append(args, "--data-dir", "")
	com, conf = create()
	err = coretesting.InitCommand(com, badArgs)
	c.Assert(err, gc.ErrorMatches, "--data-dir option must be set")

	args = append(args, "--data-dir", "jd")
	com, conf = create()
	c.Assert(coretesting.InitCommand(com, args), gc.IsNil)
	c.Assert(conf.dataDir, gc.Equals, "jd")
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

type runner interface {
	Run(*cmd.Context) error
	Stop() error
}

// runWithTimeout runs an agent and waits
// for it to complete within a reasonable time.
func runWithTimeout(r runner) error {
	done := make(chan error)
	go func() {
		done <- r.Run(nil)
	}()
	select {
	case err := <-done:
		return err
	case <-time.After(coretesting.LongWait):
	}
	err := r.Stop()
	return fmt.Errorf("timed out waiting for agent to finish; stop error: %v", err)
}

// agentSuite is a fixture to be used by agent test suites.
type agentSuite struct {
	oldRestartDelay time.Duration
	testing.JujuConnSuite
}

func (s *agentSuite) SetUpSuite(c *gc.C) {
	s.JujuConnSuite.SetUpSuite(c)

	s.oldRestartDelay = worker.RestartDelay
	// We could use testing.ShortWait, but this thrashes quite
	// a bit when some tests are restarting every 50ms for 10 seconds,
	// so use a slightly more friendly delay.
	worker.RestartDelay = 250 * time.Millisecond
	s.PatchValue(&ensureMongoServer, func(mongo.EnsureServerParams) error {
		return nil
	})
}

func (s *agentSuite) TearDownSuite(c *gc.C) {
	s.JujuConnSuite.TearDownSuite(c)
	worker.RestartDelay = s.oldRestartDelay
}

func (s *agentSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	// The standard jujud setupLogging replaces the default logger with one
	// that uses a lumberjack rolling log file.  We want to make sure that all
	// logging information from the agents when run through the tests continue
	// to have the logging information available in the gocheck test logs, so
	// we mock it out here.
	s.PatchValue(&setupLogging, func(conf agent.Config) error { return nil })
	// Set API host ports so FindTools/Tools API calls succeed.
	hostPorts := [][]network.HostPort{{{
		Address: network.NewAddress("0.1.2.3", network.ScopeUnknown),
		Port:    1234,
	}}}
	err := s.State.SetAPIHostPorts(hostPorts)
	c.Assert(err, jc.ErrorIsNil)
}

// primeAgent writes the configuration file and tools with version vers
// for an agent with the given entity name.  It returns the agent's
// configuration and the current tools.
func (s *agentSuite) primeAgent(c *gc.C, tag names.Tag, password string, vers version.Binary) (agent.ConfigSetterWriter, *coretools.Tools) {
	logger.Debugf("priming agent %s", tag.String())
	stor, err := filestorage.NewFileStorageWriter(c.MkDir())
	c.Assert(err, jc.ErrorIsNil)
	agentTools := envtesting.PrimeTools(c, stor, s.DataDir(), "released", vers)
	err = envtools.MergeAndWriteMetadata(stor, "released", "released", coretools.List{agentTools}, envtools.DoNotWriteMirrors)
	tools1, err := agenttools.ChangeAgentTools(s.DataDir(), tag.String(), vers)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(tools1, gc.DeepEquals, agentTools)

	stateInfo := s.MongoInfo(c)
	apiInfo := s.APIInfo(c)
	conf, err := agent.NewAgentConfig(
		agent.AgentConfigParams{
			DataDir:           s.DataDir(),
			Tag:               tag,
			UpgradedToVersion: vers.Number,
			Password:          password,
			Nonce:             agent.BootstrapNonce,
			StateAddresses:    stateInfo.Addrs,
			APIAddresses:      apiInfo.Addrs,
			CACert:            stateInfo.CACert,
		})
	conf.SetPassword(password)
	c.Assert(conf.Write(), gc.IsNil)
	s.primeAPIHostPorts(c)
	return conf, agentTools
}

func (s *agentSuite) primeAPIHostPorts(c *gc.C) {
	apiInfo := s.APIInfo(c)

	c.Assert(apiInfo.Addrs, gc.HasLen, 1)
	hostPort, err := parseHostPort(apiInfo.Addrs[0])
	c.Assert(err, jc.ErrorIsNil)

	err = s.State.SetAPIHostPorts([][]network.HostPort{{hostPort}})
	c.Assert(err, jc.ErrorIsNil)

	logger.Debugf("api host ports primed %#v", hostPort)
}

func parseHostPort(s string) (network.HostPort, error) {
	addr, port, err := net.SplitHostPort(s)
	if err != nil {
		return network.HostPort{}, err
	}
	portNum, err := strconv.Atoi(port)
	if err != nil {
		return network.HostPort{}, fmt.Errorf("bad port number %q", port)
	}
	addrs := network.NewAddresses(addr)
	hostPorts := network.AddressesWithPort(addrs, portNum)
	return hostPorts[0], nil
}

// writeStateAgentConfig creates and writes a state agent config.
func writeStateAgentConfig(c *gc.C, stateInfo *mongo.MongoInfo, dataDir string, tag names.Tag, password string, vers version.Binary) agent.ConfigSetterWriter {
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
		},
		params.StateServingInfo{
			Cert:       coretesting.ServerCert,
			PrivateKey: coretesting.ServerKey,
			StatePort:  gitjujutesting.MgoServer.Port(),
			APIPort:    port,
		})
	c.Assert(err, jc.ErrorIsNil)
	conf.SetPassword(password)
	c.Assert(conf.Write(), gc.IsNil)
	return conf
}

// primeStateAgent writes the configuration file and tools with version vers
// for an agent with the given entity name.  It returns the agent's configuration
// and the current tools.
func (s *agentSuite) primeStateAgent(
	c *gc.C, tag names.Tag, password string, vers version.Binary) (agent.ConfigSetterWriter, *coretools.Tools) {

	stor, err := filestorage.NewFileStorageWriter(c.MkDir())
	c.Assert(err, jc.ErrorIsNil)
	agentTools := envtesting.PrimeTools(c, stor, s.DataDir(), "released", vers)
	tools1, err := agenttools.ChangeAgentTools(s.DataDir(), tag.String(), vers)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(tools1, gc.DeepEquals, agentTools)

	stateInfo := s.MongoInfo(c)
	conf := writeStateAgentConfig(c, stateInfo, s.DataDir(), tag, password, vers)
	s.primeAPIHostPorts(c)
	return conf, agentTools
}

// initAgent initialises the given agent command with additional
// arguments as provided.
func (s *agentSuite) initAgent(c *gc.C, a cmd.Command, args ...string) {
	args = append([]string{"--data-dir", s.DataDir()}, args...)
	err := coretesting.InitCommand(a, args)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *agentSuite) testOpenAPIState(c *gc.C, ent state.AgentEntity, agentCmd Agent, initialPassword string) {
	conf, err := agent.ReadConfig(agent.ConfigPath(s.DataDir(), ent.Tag()))
	c.Assert(err, jc.ErrorIsNil)

	conf.SetPassword("")
	err = conf.Write()
	c.Assert(err, jc.ErrorIsNil)

	// Check that it starts initially and changes the password
	assertOpen := func(conf agent.Config) {
		st, gotEnt, err := openAPIState(conf, agentCmd)
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
	conf = refreshConfig(c, conf)
	// Check we can open the API with the new configuration.
	assertOpen(conf)
}

type errorAPIOpener struct {
	err error
}

func (e *errorAPIOpener) OpenAPI(_ api.DialOpts) (*api.State, string, error) {
	return nil, "", e.err
}

func (s *agentSuite) assertCanOpenState(c *gc.C, tag names.Tag, dataDir string) {
	config, err := agent.ReadConfig(agent.ConfigPath(dataDir, tag))
	c.Assert(err, jc.ErrorIsNil)
	info, ok := config.MongoInfo()
	c.Assert(ok, jc.IsTrue)
	st, err := state.Open(info, mongo.DialOpts{}, environs.NewStatePolicy())
	c.Assert(err, jc.ErrorIsNil)
	st.Close()
}

func (s *agentSuite) assertCannotOpenState(c *gc.C, tag names.Tag, dataDir string) {
	config, err := agent.ReadConfig(agent.ConfigPath(dataDir, tag))
	c.Assert(err, jc.ErrorIsNil)
	_, ok := config.MongoInfo()
	c.Assert(ok, jc.IsFalse)
}

func refreshConfig(c *gc.C, config agent.Config) agent.ConfigSetterWriter {
	config1, err := agent.ReadConfig(agent.ConfigPath(config.DataDir(), config.Tag()))
	c.Assert(err, jc.ErrorIsNil)
	return config1
}

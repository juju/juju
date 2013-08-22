// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"bytes"
	stderrors "errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/agent"
	"launchpad.net/juju-core/agent/tools"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
	coretesting "launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/version"
	"launchpad.net/juju-core/worker"
	"launchpad.net/juju-core/worker/upgrader"
)

var _ = gc.Suite(&toolSuite{})

type toolSuite struct {
	coretesting.LoggingSuite
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

func mkTools(s string) *tools.Tools {
	return &tools.Tools{
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
	case <-time.After(5 * time.Second):
	}
	err := r.Stop()
	return fmt.Errorf("timed out waiting for agent to finish; stop error: %v", err)
}

// agentSuite is a fixture to be used by agent test suites.
type agentSuite struct {
	testing.JujuConnSuite
}

// primeAgent writes the configuration file and tools for an agent with the
// given entity name.  It returns the agent's configuration and the current
// tools.
func (s *agentSuite) primeAgent(c *gc.C, tag, password string) (agent.Config, *tools.Tools) {
	agentTools := s.primeTools(c, version.Current)
	tools1, err := tools.ChangeAgentTools(s.DataDir(), tag, version.Current)
	c.Assert(err, gc.IsNil)
	c.Assert(tools1, gc.DeepEquals, agentTools)

	stateInfo := s.StateInfo(c)
	apiInfo := s.APIInfo(c)
	conf, err := agent.NewAgentConfig(
		agent.AgentConfigParams{
			DataDir:        s.DataDir(),
			Tag:            tag,
			Password:       password,
			Nonce:          state.BootstrapNonce,
			StateAddresses: stateInfo.Addrs,
			APIAddresses:   apiInfo.Addrs,
			CACert:         stateInfo.CACert,
		})
	c.Assert(conf.Write(), gc.IsNil)
	return conf, agentTools
}

// primeStateAgent writes the configuration file and tools for an agent with the
// given entity name.  It returns the agent's configuration and the current
// tools.
func (s *agentSuite) primeStateAgent(c *gc.C, tag, password string) (agent.Config, *tools.Tools) {
	agentTools := s.primeTools(c, version.Current)
	tools1, err := tools.ChangeAgentTools(s.DataDir(), tag, version.Current)
	c.Assert(err, gc.IsNil)
	c.Assert(tools1, gc.DeepEquals, agentTools)

	stateInfo := s.StateInfo(c)
	port := coretesting.FindTCPPort()
	apiAddr := []string{fmt.Sprintf("localhost:%d", port)}
	conf, err := agent.NewStateMachineConfig(
		agent.StateMachineConfigParams{
			AgentConfigParams: agent.AgentConfigParams{
				DataDir:        s.DataDir(),
				Tag:            tag,
				Password:       password,
				Nonce:          state.BootstrapNonce,
				StateAddresses: stateInfo.Addrs,
				APIAddresses:   apiAddr,
				CACert:         stateInfo.CACert,
			},
			StateServerCert: []byte(coretesting.ServerCert),
			StateServerKey:  []byte(coretesting.ServerKey),
			StatePort:       coretesting.MgoPort,
			APIPort:         port,
		})
	c.Assert(conf.Write(), gc.IsNil)
	return conf, agentTools
}

// initAgent initialises the given agent command with additional
// arguments as provided.
func (s *agentSuite) initAgent(c *gc.C, a cmd.Command, args ...string) {
	args = append([]string{"--data-dir", s.DataDir()}, args...)
	err := coretesting.InitCommand(a, args)
	c.Assert(err, gc.IsNil)
}

func (s *agentSuite) proposeVersion(c *gc.C, vers version.Number) {
	cfg, err := s.State.EnvironConfig()
	c.Assert(err, gc.IsNil)
	cfg, err = cfg.Apply(map[string]interface{}{
		"agent-version": vers.String(),
	})
	c.Assert(err, gc.IsNil)
	err = s.State.SetEnvironConfig(cfg)
	c.Assert(err, gc.IsNil)
}

func (s *agentSuite) uploadTools(c *gc.C, vers version.Binary) *tools.Tools {
	tgz := coretesting.TarGz(
		coretesting.NewTarFile("jujud", 0777, "jujud contents "+vers.String()),
	)
	storage := s.Conn.Environ.Storage()
	err := storage.Put(tools.StorageName(vers), bytes.NewReader(tgz), int64(len(tgz)))
	c.Assert(err, gc.IsNil)
	url, err := s.Conn.Environ.Storage().URL(tools.StorageName(vers))
	c.Assert(err, gc.IsNil)
	return &tools.Tools{URL: url, Version: vers}
}

// primeTools sets up the current version of the tools to vers and
// makes sure that they're available JujuConnSuite's DataDir.
func (s *agentSuite) primeTools(c *gc.C, vers version.Binary) *tools.Tools {
	err := os.RemoveAll(filepath.Join(s.DataDir(), "tools"))
	c.Assert(err, gc.IsNil)
	version.Current = vers
	agentTools := s.uploadTools(c, vers)
	resp, err := http.Get(agentTools.URL)
	c.Assert(err, gc.IsNil)
	defer resp.Body.Close()
	err = tools.UnpackTools(s.DataDir(), agentTools, resp.Body)
	c.Assert(err, gc.IsNil)
	return agentTools
}

func (s *agentSuite) testOpenAPIState(c *gc.C, ent state.AgentEntity, agentCmd Agent, initialPassword string) {
	conf, err := agent.ReadConf(s.DataDir(), ent.Tag())
	c.Assert(err, gc.IsNil)

	// Check that it starts initially and changes the password
	assertOpen := func(conf agent.Config) {
		st, gotEnt, err := openAPIState(conf, agentCmd)
		c.Assert(err, gc.IsNil)
		c.Assert(st, gc.NotNil)
		st.Close()
		c.Assert(gotEnt.Tag(), gc.Equals, ent.Tag())
	}
	assertOpen(conf)

	// Check that the initial password is no longer valid.
	err = ent.Refresh()
	c.Assert(err, gc.IsNil)
	c.Assert(ent.PasswordValid(initialPassword), gc.Equals, false)

	// Read the configuration and check that we can connect with it.
	conf = refreshConfig(c, conf)
	// Check we can open the API with the new configuration.
	assertOpen(conf)
}

func (s *agentSuite) testUpgrade(c *gc.C, agent runner, currentTools *tools.Tools) {
	newVers := version.Current
	newVers.Patch++
	newTools := s.uploadTools(c, newVers)
	s.proposeVersion(c, newVers.Number)
	err := runWithTimeout(agent)
	c.Assert(err, gc.FitsTypeOf, &upgrader.UpgradeReadyError{})
	ug := err.(*upgrader.UpgradeReadyError)
	c.Assert(ug.NewTools, gc.DeepEquals, newTools)
	c.Assert(ug.OldTools, gc.DeepEquals, currentTools)
}

func refreshConfig(c *gc.C, config agent.Config) agent.Config {
	config, err := agent.ReadConf(config.DataDir(), config.Tag())
	c.Assert(err, gc.IsNil)
	return config
}

// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"encoding/base64"
	"io/ioutil"
	"path/filepath"

	gc "launchpad.net/gocheck"
	"launchpad.net/goyaml"

	"launchpad.net/juju-core/agent"
	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/environs/bootstrap"
	"launchpad.net/juju-core/environs/jujutest"
	"launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/provider/dummy"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/testing"
	jc "launchpad.net/juju-core/testing/checkers"
	"launchpad.net/juju-core/testing/testbase"
	"launchpad.net/juju-core/utils"
)

// We don't want to use JujuConnSuite because it gives us
// an already-bootstrapped environment.
type BootstrapSuite struct {
	testbase.LoggingSuite
	testing.MgoSuite
	dataDir              string
	providerStateURLFile string
}

var _ = gc.Suite(&BootstrapSuite{})

var testRoundTripper = &jujutest.ProxyRoundTripper{}

func init() {
	// Prepare mock http transport for provider-state output in tests.
	testRoundTripper.RegisterForScheme("test")
}

func (s *BootstrapSuite) SetUpSuite(c *gc.C) {
	s.LoggingSuite.SetUpSuite(c)
	s.MgoSuite.SetUpSuite(c)
	stateInfo := bootstrap.BootstrapState{
		StateInstances: []instance.Id{instance.Id("dummy.instance.id")},
	}
	stateData, err := goyaml.Marshal(stateInfo)
	c.Assert(err, gc.IsNil)
	content := map[string]string{"/" + bootstrap.StateFile: string(stateData)}
	testRoundTripper.Sub = jujutest.NewCannedRoundTripper(content, nil)
	s.providerStateURLFile = filepath.Join(c.MkDir(), "provider-state-url")
	providerStateURLFile = s.providerStateURLFile
}

func (s *BootstrapSuite) TearDownSuite(c *gc.C) {
	s.MgoSuite.TearDownSuite(c)
	s.LoggingSuite.TearDownSuite(c)
}

func (s *BootstrapSuite) SetUpTest(c *gc.C) {
	s.LoggingSuite.SetUpTest(c)
	s.MgoSuite.SetUpTest(c)
	s.dataDir = c.MkDir()
}

func (s *BootstrapSuite) TearDownTest(c *gc.C) {
	s.MgoSuite.TearDownTest(c)
	s.LoggingSuite.TearDownTest(c)
}

var testPassword = "my-admin-secret"

func testPasswordHash() string {
	return utils.UserPasswordHash(testPassword, utils.CompatSalt)
}

func (s *BootstrapSuite) initBootstrapCommand(c *gc.C, args ...string) (machineConf agent.Config, cmd *BootstrapCommand, err error) {
	ioutil.WriteFile(s.providerStateURLFile, []byte("test://localhost/provider-state\n"), 0600)
	// NOTE: the old test used an equivalent of the NewAgentConfig, but it
	// really should be using NewStateMachineConfig.
	params := agent.AgentConfigParams{
		DataDir:        s.dataDir,
		Tag:            "bootstrap",
		Password:       testPasswordHash(),
		Nonce:          state.BootstrapNonce,
		StateAddresses: []string{testing.MgoServer.Addr()},
		APIAddresses:   []string{"0.1.2.3:1234"},
		CACert:         []byte(testing.CACert),
	}
	bootConf, err := agent.NewAgentConfig(params)
	c.Assert(err, gc.IsNil)
	err = bootConf.Write()
	c.Assert(err, gc.IsNil)

	params.Tag = "machine-0"
	machineConf, err = agent.NewAgentConfig(params)
	c.Assert(err, gc.IsNil)
	err = machineConf.Write()
	c.Assert(err, gc.IsNil)

	cmd = &BootstrapCommand{}
	err = testing.InitCommand(cmd, append([]string{"--data-dir", s.dataDir}, args...))
	return machineConf, cmd, err
}

func (s *BootstrapSuite) TestInitializeEnvironment(c *gc.C) {
	_, cmd, err := s.initBootstrapCommand(c, "--env-config", testConfig)
	c.Assert(err, gc.IsNil)
	err = cmd.Run(nil)
	c.Assert(err, gc.IsNil)

	st, err := state.Open(&state.Info{
		Addrs:    []string{testing.MgoServer.Addr()},
		CACert:   []byte(testing.CACert),
		Password: testPasswordHash(),
	}, state.DefaultDialOpts())
	c.Assert(err, gc.IsNil)
	defer st.Close()
	machines, err := st.AllMachines()
	c.Assert(err, gc.IsNil)
	c.Assert(machines, gc.HasLen, 1)

	instid, err := machines[0].InstanceId()
	c.Assert(err, gc.IsNil)
	c.Assert(instid, gc.Equals, instance.Id("dummy.instance.id"))

	cons, err := st.EnvironConstraints()
	c.Assert(err, gc.IsNil)
	c.Assert(&cons, jc.Satisfies, constraints.IsEmpty)
}

func (s *BootstrapSuite) TestSetConstraints(c *gc.C) {
	tcons := constraints.Value{Mem: uint64p(2048), CpuCores: uint64p(2)}
	_, cmd, err := s.initBootstrapCommand(c, "--env-config", testConfig, "--constraints", tcons.String())
	c.Assert(err, gc.IsNil)
	err = cmd.Run(nil)
	c.Assert(err, gc.IsNil)

	st, err := state.Open(&state.Info{
		Addrs:    []string{testing.MgoServer.Addr()},
		CACert:   []byte(testing.CACert),
		Password: testPasswordHash(),
	}, state.DefaultDialOpts())
	c.Assert(err, gc.IsNil)
	defer st.Close()
	cons, err := st.EnvironConstraints()
	c.Assert(err, gc.IsNil)
	c.Assert(cons, gc.DeepEquals, tcons)

	machines, err := st.AllMachines()
	c.Assert(err, gc.IsNil)
	c.Assert(machines, gc.HasLen, 1)
	cons, err = machines[0].Constraints()
	c.Assert(err, gc.IsNil)
	c.Assert(cons, gc.DeepEquals, tcons)
}

func uint64p(v uint64) *uint64 {
	return &v
}

func (s *BootstrapSuite) TestMachinerWorkers(c *gc.C) {
	_, cmd, err := s.initBootstrapCommand(c, "--env-config", testConfig)
	c.Assert(err, gc.IsNil)
	err = cmd.Run(nil)
	c.Assert(err, gc.IsNil)

	st, err := state.Open(&state.Info{
		Addrs:    []string{testing.MgoServer.Addr()},
		CACert:   []byte(testing.CACert),
		Password: testPasswordHash(),
	}, state.DefaultDialOpts())
	c.Assert(err, gc.IsNil)
	defer st.Close()
	m, err := st.Machine("0")
	c.Assert(err, gc.IsNil)
	c.Assert(m.Jobs(), gc.DeepEquals, []state.MachineJob{
		state.JobManageEnviron, state.JobHostUnits,
	})
}

func testOpenState(c *gc.C, info *state.Info, expectErrType error) {
	st, err := state.Open(info, state.DefaultDialOpts())
	if st != nil {
		st.Close()
	}
	if expectErrType != nil {
		c.Assert(err, gc.FitsTypeOf, expectErrType)
	} else {
		c.Assert(err, gc.IsNil)
	}
}

func (s *BootstrapSuite) TestInitialPassword(c *gc.C) {
	machineConf, cmd, err := s.initBootstrapCommand(c, "--env-config", testConfig)
	c.Assert(err, gc.IsNil)

	err = cmd.Run(nil)
	c.Assert(err, gc.IsNil)

	// Check that we cannot now connect to the state without a
	// password.
	info := &state.Info{
		Addrs:  []string{testing.MgoServer.Addr()},
		CACert: []byte(testing.CACert),
	}
	testOpenState(c, info, errors.Unauthorizedf(""))

	// Check we can log in to mongo as admin.
	info.Tag, info.Password = "", testPasswordHash()
	st, err := state.Open(info, state.DefaultDialOpts())
	c.Assert(err, gc.IsNil)
	// Reset password so the tests can continue to use the same server.
	defer st.Close()
	defer st.SetAdminMongoPassword("")

	// Check that the admin user has been given an appropriate
	// password
	u, err := st.User("admin")
	c.Assert(err, gc.IsNil)
	c.Assert(u.PasswordValid(testPassword), gc.Equals, true)

	// Check that the machine configuration has been given a new
	// password and that we can connect to mongo as that machine
	// and that the in-mongo password also verifies correctly.
	machineConf1, err := agent.ReadConf(machineConf.DataDir(), "machine-0")
	c.Assert(err, gc.IsNil)

	st, err = machineConf1.OpenState()
	c.Assert(err, gc.IsNil)
	defer st.Close()
}

var base64ConfigTests = []struct {
	input    []string
	err      string
	expected map[string]interface{}
}{
	{
		// no value supplied
		nil,
		"--env-config option must be set",
		nil,
	}, {
		// empty
		[]string{"--env-config", ""},
		"--env-config option must be set",
		nil,
	}, {
		// wrong, should be base64
		[]string{"--env-config", "name: banana\n"},
		".*illegal base64 data at input byte.*",
		nil,
	}, {
		[]string{"--env-config", base64.StdEncoding.EncodeToString([]byte("name: banana\n"))},
		"",
		map[string]interface{}{"name": "banana"},
	},
}

func (s *BootstrapSuite) TestBase64Config(c *gc.C) {
	for i, t := range base64ConfigTests {
		c.Logf("test %d", i)
		var args []string
		args = append(args, t.input...)
		_, cmd, err := s.initBootstrapCommand(c, args...)
		if t.err == "" {
			c.Assert(cmd, gc.NotNil)
			c.Assert(err, gc.IsNil)
			c.Assert(cmd.EnvConfig, gc.DeepEquals, t.expected)
		} else {
			c.Assert(err, gc.ErrorMatches, t.err)
		}
	}
}

type b64yaml map[string]interface{}

func (m b64yaml) encode() string {
	data, err := goyaml.Marshal(m)
	if err != nil {
		panic(err)
	}
	return base64.StdEncoding.EncodeToString(data)
}

var testConfig = b64yaml(
	dummy.SampleConfig().Merge(
		testing.Attrs{
			"state-server":  false,
			"agent-version": "3.4.5",
		},
	).Delete("admin-secret", "ca-private-key")).encode()

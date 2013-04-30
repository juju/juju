package main

import (
	"encoding/base64"
	. "launchpad.net/gocheck"
	"launchpad.net/goyaml"
	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/environs/agent"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/testing"
)

// We don't want to use JujuConnSuite because it gives us
// an already-bootstrapped environment.
type BootstrapSuite struct {
	testing.LoggingSuite
	testing.MgoSuite
	dataDir string
}

var _ = Suite(&BootstrapSuite{})

func (s *BootstrapSuite) SetUpSuite(c *C) {
	s.LoggingSuite.SetUpSuite(c)
	s.MgoSuite.SetUpSuite(c)
}

func (s *BootstrapSuite) TearDownSuite(c *C) {
	s.MgoSuite.TearDownSuite(c)
	s.LoggingSuite.TearDownSuite(c)
}

func (s *BootstrapSuite) SetUpTest(c *C) {
	s.LoggingSuite.SetUpTest(c)
	s.MgoSuite.SetUpTest(c)
	s.dataDir = c.MkDir()
}

func (s *BootstrapSuite) TearDownTest(c *C) {
	s.MgoSuite.TearDownTest(c)
	s.LoggingSuite.TearDownTest(c)
}

func (s *BootstrapSuite) initBootstrapCommand(c *C, args ...string) (*agent.Conf, *BootstrapCommand, error) {
	conf := &agent.Conf{
		DataDir: s.dataDir,
		StateInfo: &state.Info{
			Tag:    "bootstrap",
			Addrs:  []string{testing.MgoAddr},
			CACert: []byte(testing.CACert),
		},
	}
	err := conf.Write()
	c.Assert(err, IsNil)
	cmd := &BootstrapCommand{}
	err = testing.InitCommand(cmd, append([]string{"--data-dir", s.dataDir}, args...))
	return conf, cmd, err
}

func (s *BootstrapSuite) TestInitializeEnvironment(c *C) {
	_, cmd, err := s.initBootstrapCommand(c, "--env-config", testConfig)
	c.Assert(err, IsNil)
	err = cmd.Run(nil)
	c.Assert(err, IsNil)

	st, err := state.Open(&state.Info{
		Addrs:  []string{testing.MgoAddr},
		CACert: []byte(testing.CACert),
	}, state.DefaultDialOpts())
	c.Assert(err, IsNil)
	defer st.Close()
	machines, err := st.AllMachines()
	c.Assert(err, IsNil)
	c.Assert(machines, HasLen, 1)

	instid, ok := machines[0].InstanceId()
	c.Assert(ok, Equals, true)
	c.Assert(instid, Equals, state.InstanceId("dummy.instance.id"))

	cons, err := st.EnvironConstraints()
	c.Assert(err, IsNil)
	c.Assert(cons, DeepEquals, constraints.Value{})
}

func (s *BootstrapSuite) TestSetConstraints(c *C) {
	tcons := constraints.Value{Mem: uint64p(2048), CpuCores: uint64p(2)}
	_, cmd, err := s.initBootstrapCommand(c, "--env-config", testConfig, "--constraints", tcons.String())
	c.Assert(err, IsNil)
	err = cmd.Run(nil)
	c.Assert(err, IsNil)

	st, err := state.Open(&state.Info{
		Addrs:  []string{testing.MgoAddr},
		CACert: []byte(testing.CACert),
	}, state.DefaultDialOpts())
	c.Assert(err, IsNil)
	defer st.Close()
	cons, err := st.EnvironConstraints()
	c.Assert(err, IsNil)
	c.Assert(cons, DeepEquals, tcons)

	machines, err := st.AllMachines()
	c.Assert(err, IsNil)
	c.Assert(machines, HasLen, 1)
	cons, err = machines[0].Constraints()
	c.Assert(err, IsNil)
	c.Assert(cons, DeepEquals, tcons)
}

func uint64p(v uint64) *uint64 {
	return &v
}

func (s *BootstrapSuite) TestMachinerWorkers(c *C) {
	_, cmd, err := s.initBootstrapCommand(c, "--env-config", testConfig)
	c.Assert(err, IsNil)
	err = cmd.Run(nil)
	c.Assert(err, IsNil)

	st, err := state.Open(&state.Info{
		Addrs:  []string{testing.MgoAddr},
		CACert: []byte(testing.CACert),
	}, state.DefaultDialOpts())
	c.Assert(err, IsNil)
	defer st.Close()
	m, err := st.Machine("0")
	c.Assert(err, IsNil)
	c.Assert(m.Jobs(), DeepEquals, []state.MachineJob{
		state.JobManageEnviron, state.JobServeAPI, state.JobHostUnits,
	})
}

func testOpenState(c *C, info *state.Info, expectErr error) {
	st, err := state.Open(info, state.DefaultDialOpts())
	if st != nil {
		st.Close()
	}
	if expectErr != nil {
		c.Assert(state.IsUnauthorizedError(err), Equals, true)
	} else {
		c.Assert(err, IsNil)
	}
}

func (s *BootstrapSuite) TestInitialPassword(c *C) {
	conf, cmd, err := s.initBootstrapCommand(c, "--env-config", testConfig)
	c.Assert(err, IsNil)
	conf.OldPassword = "foo"
	err = conf.Write()
	c.Assert(err, IsNil)

	err = cmd.Run(nil)
	c.Assert(err, IsNil)

	// Check that we cannot now connect to the state
	// without a password.
	info := &state.Info{
		Addrs:  []string{testing.MgoAddr},
		CACert: []byte(testing.CACert),
	}
	testOpenState(c, info, state.Unauthorizedf("some auth problem"))

	info.Tag, info.Password = "machine-0", "foo"
	testOpenState(c, info, nil)

	info.Tag = ""
	st, err := state.Open(info, state.DefaultDialOpts())
	c.Assert(err, IsNil)
	defer st.Close()

	// Reset password so the tests can continue to use the same server.
	err = st.SetAdminMongoPassword("")
	c.Assert(err, IsNil)
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

func (s *BootstrapSuite) TestBase64Config(c *C) {
	for i, t := range base64ConfigTests {
		c.Logf("test %d", i)
		var args []string
		args = append(args, t.input...)
		_, cmd, err := s.initBootstrapCommand(c, args...)
		if t.err == "" {
			c.Assert(cmd, NotNil)
			c.Assert(err, IsNil)
			c.Assert(cmd.EnvConfig, DeepEquals, t.expected)
		} else {
			c.Assert(err, ErrorMatches, t.err)
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

var testConfig = b64yaml{
	"name":            "dummyenv",
	"type":            "dummy",
	"state-server":    false,
	"agent-version":   "1.2.3",
	"authorized-keys": "i-am-a-key",
	"ca-cert":         testing.CACert,
	"ca-private-key":  "",
}.encode()

package main

import (
	"encoding/base64"
	. "launchpad.net/gocheck"
	"launchpad.net/goyaml"
	"launchpad.net/juju-core/cmd"
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
			EntityName: "bootstrap",
			Addrs:      []string{testing.MgoAddr},
			CACert:     []byte(testing.CACert),
		},
	}
	err := conf.Write()
	c.Assert(err, IsNil)
	cmd := &BootstrapCommand{}
	err = initCmd(cmd, append([]string{"--data-dir", s.dataDir}, args...))
	return conf, cmd, err
}

func (s *BootstrapSuite) TestParse(c *C) {
	create := func() (cmd.Command, *AgentConf) {
		a := &BootstrapCommand{}
		return a, &a.Conf
	}
	a := CheckAgentCommand(c, create, []string{
		"--env-config", b64yaml{"foo": 123}.encode(),
		"--instance-id", "iWhatever",
	})
	cmd := a.(*BootstrapCommand)
	c.Check(cmd.InstanceId, Equals, "iWhatever")
}

func (s *BootstrapSuite) TestParseNoInstanceId(c *C) {
	ecfg := b64yaml{"foo": 123}.encode()
	_, _, err := s.initBootstrapCommand(c, "--env-config", ecfg)
	c.Assert(err, ErrorMatches, "--instance-id option must be set")
}

func (s *BootstrapSuite) TestParseNoEnvConfig(c *C) {
	_, _, err := s.initBootstrapCommand(c, "--instance-id", "x")
	c.Assert(err, ErrorMatches, "--env-config option must be set")
}

func (s *BootstrapSuite) TestSetMachineId(c *C) {
	args := []string{
		"--instance-id", "over9000",
		"--env-config", b64yaml{
			"name":            "dummyenv",
			"type":            "dummy",
			"state-server":    false,
			"authorized-keys": "i-am-a-key",
			"ca-cert":         testing.CACert,
		}.encode(),
	}
	_, cmd, err := s.initBootstrapCommand(c, args...)
	c.Assert(err, IsNil)
	err = cmd.Run(nil)
	c.Assert(err, IsNil)

	st, err := state.Open(&state.Info{
		Addrs:  []string{testing.MgoAddr},
		CACert: []byte(testing.CACert),
	})
	c.Assert(err, IsNil)
	defer st.Close()
	machines, err := st.AllMachines()
	c.Assert(err, IsNil)
	c.Assert(len(machines), Equals, 1)

	instid, err := machines[0].InstanceId()
	c.Assert(err, IsNil)
	c.Assert(instid, Equals, state.InstanceId("over9000"))
}

func (s *BootstrapSuite) TestMachinerWorkers(c *C) {
	args := []string{
		"--instance-id", "over9000",
		"--env-config", b64yaml{
			"name":            "dummyenv",
			"type":            "dummy",
			"state-server":    false,
			"authorized-keys": "i-am-a-key",
			"ca-cert":         testing.CACert,
		}.encode(),
	}
	_, cmd, err := s.initBootstrapCommand(c, args...)
	c.Assert(err, IsNil)
	err = cmd.Run(nil)
	c.Assert(err, IsNil)

	st, err := state.Open(&state.Info{
		Addrs:  []string{testing.MgoAddr},
		CACert: []byte(testing.CACert),
	})
	c.Assert(err, IsNil)
	defer st.Close()
	m, err := st.Machine("0")
	c.Assert(err, IsNil)
	c.Assert(m.Jobs(), DeepEquals, []state.MachineJob{state.JobManageEnviron})
}

func testOpenState(c *C, info *state.Info, expectErr error) {
	st, err := state.Open(info)
	if st != nil {
		st.Close()
	}
	if expectErr != nil {
		c.Assert(err, Equals, expectErr)
	} else {
		c.Assert(err, IsNil)
	}
}

func (s *BootstrapSuite) TestInitialPassword(c *C) {
	args := []string{
		"--instance-id", "over9000",
		"--env-config", b64yaml{
			"name":            "dummyenv",
			"type":            "dummy",
			"state-server":    false,
			"authorized-keys": "i-am-a-key",
			"ca-cert":         testing.CACert,
		}.encode(),
	}
	conf, cmd, err := s.initBootstrapCommand(c, args...)
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
	testOpenState(c, info, state.ErrUnauthorized)

	info.EntityName, info.Password = "machine-0", "foo"
	testOpenState(c, info, nil)

	info.EntityName = ""
	st, err := state.Open(info)
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
		args = append(args, "--instance-id", "over9000")
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

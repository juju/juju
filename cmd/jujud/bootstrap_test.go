package main

import (
	"encoding/base64"
	. "launchpad.net/gocheck"
	"launchpad.net/goyaml"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
)

type BootstrapSuite struct {
	testing.JujuConnSuite
}

var _ = Suite(&BootstrapSuite{})

func initBootstrapCommand(args []string) (*BootstrapCommand, error) {
	c := &BootstrapCommand{}
	return c, initCmd(c, args)
}

func (s *BootstrapSuite) TestParse(c *C) {
	create := func() (cmd.Command, *AgentConf) {
		a := &BootstrapCommand{}
		return a, &a.Conf
	}
	a := CheckAgentCommand(c, create, []string{
		"--env-config", b64yaml{"foo": 123}.encode(),
		"--instance-id", "iWhatever",
	}, flagInitialPassword|flagStateInfo)
	cmd := a.(*BootstrapCommand)
	c.Check(cmd.InstanceId, Equals, "iWhatever")
}

func (s *BootstrapSuite) TestParseNoInstanceId(c *C) {
	ecfg := b64yaml{"foo": 123}.encode()
	_, err := initBootstrapCommand([]string{"--env-config", ecfg})
	c.Assert(err, ErrorMatches, "--instance-id option must be set")
}

func (s *BootstrapSuite) TestParseNoEnvConfig(c *C) {
	_, err := initBootstrapCommand([]string{"--instance-id", "x"})
	c.Assert(err, ErrorMatches, "--env-config option must be set")

}

func (s *BootstrapSuite) TestSetMachineId(c *C) {
	args := []string{
		"--state-servers", s.StateInfo(c).Addrs[0],
		"--instance-id", "over9000",
		"--env-config", b64yaml{
			"name":            "dummyenv",
			"type":            "dummy",
			"state-server":    false,
			"authorized-keys": "i-am-a-key",
		}.encode(),
	}
	cmd, err := initBootstrapCommand(args)
	c.Assert(err, IsNil)
	err = cmd.Run(nil)
	c.Assert(err, IsNil)

	machines, err := s.State.AllMachines()
	c.Assert(err, IsNil)
	c.Assert(len(machines), Equals, 1)

	instid, err := machines[0].InstanceId()
	c.Assert(err, IsNil)
	c.Assert(instid, Equals, "over9000")
}

func (s *BootstrapSuite) TestMachinerWorkers(c *C) {
	args := []string{
		"--state-servers", s.StateInfo(c).Addrs[0],
		"--instance-id", "over9000",
		"--env-config", b64yaml{
			"name":            "dummyenv",
			"type":            "dummy",
			"state-server":    false,
			"authorized-keys": "i-am-a-key",
		}.encode(),
	}
	cmd, err := initBootstrapCommand(args)
	c.Assert(err, IsNil)
	err = cmd.Run(nil)
	c.Assert(err, IsNil)

	m, err := s.State.Machine(0)
	c.Assert(err, IsNil)
	c.Assert(m.Workers(), DeepEquals, []state.WorkerKind{state.MachinerWorker, state.ProvisionerWorker, state.FirewallerWorker})
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
	// First get a state that's properly logged in, so
	// that we can reset the password if something goes wrong.
	err := s.State.SetAdminPassword("arble")
	c.Assert(err, IsNil)
	defer func() {
		if err := s.State.SetAdminPassword(""); err != nil {
			c.Errorf("cannot reset admin password: %v", err)
		}
	}()
	info := s.StateInfo(c)
	info.Password = "arble"
	st, err := state.Open(info)
	c.Assert(err, IsNil)
	defer st.Close()
	// Revert to previous passwordless state.
	err = st.SetAdminPassword("")
	c.Assert(err, IsNil)

	args := []string{
		"--state-servers", s.StateInfo(c).Addrs[0],
		"--instance-id", "over9000",
		"--env-config", b64yaml{
			"name":            "dummyenv",
			"type":            "dummy",
			"state-server":    false,
			"authorized-keys": "i-am-a-key",
		}.encode(),
		"--initial-password", "foo",
	}
	cmd, err := initBootstrapCommand(args)
	c.Assert(err, IsNil)
	err = cmd.Run(nil)
	c.Assert(err, IsNil)

	// Check that we cannot now connect to the state
	// without a password.
	info = s.StateInfo(c)
	testOpenState(c, info, state.ErrUnauthorized)

	info.Password = "foo"
	testOpenState(c, info, nil)

	info.EntityName = "machine-0"
	testOpenState(c, info, nil)
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
		args := []string{"--state-servers"}
		args = append(args, s.StateInfo(c).Addrs...)
		args = append(args, "--instance-id", "over9000")
		args = append(args, t.input...)
		cmd, err := initBootstrapCommand(args)
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

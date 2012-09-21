package main

import (
	"encoding/base64"
	. "launchpad.net/gocheck"
	"launchpad.net/goyaml"
	"launchpad.net/juju-core/juju/testing"
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
	args := []string{}
	_, err := initBootstrapCommand(args)
	c.Assert(err, ErrorMatches, "--instance-id option must be set")

	args = append(args, "--instance-id", "iWhatever")
	_, err = initBootstrapCommand(args)
	c.Assert(err, ErrorMatches, "--env-config option must be set")

	args = append(args, "--env-config", b64yaml{"foo": 123}.encode())
	cmd, err := initBootstrapCommand(args)
	c.Assert(err, IsNil)
	c.Assert(cmd.StateInfo.Addrs, DeepEquals, []string{"127.0.0.1:2181"})
	c.Assert(cmd.InstanceId, Equals, "iWhatever")
	c.Assert(cmd.EnvConfig, DeepEquals, map[string]interface{}{"foo": 123})

	args = append(args, "--state-servers", "zk1:2181,zk2:2181")
	cmd, err = initBootstrapCommand(args)
	c.Assert(err, IsNil)
	c.Assert(cmd.StateInfo.Addrs, DeepEquals, []string{"zk1:2181", "zk2:2181"})

	args = append(args, "haha disregard that")
	_, err = initBootstrapCommand(args)
	c.Assert(err, ErrorMatches, `unrecognized args: \["haha disregard that"\]`)
}

func (s *BootstrapSuite) TestSetMachineId(c *C) {
	args := []string{"--state-servers"}
	args = append(args, s.StateInfo(c).Addrs...)
	args = append(args, "--instance-id", "over9000")
	args = append(args, "--env-config", b64yaml{"blah": "blah"}.encode())
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

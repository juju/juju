package main

import (
	"io/ioutil"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/version"
	"os"
	"path/filepath"
	"time"
)

type UnitSuite struct {
	testing.JujuConnSuite
}

var _ = Suite(&UnitSuite{})

func (s *UnitSuite) TestParseSuccess(c *C) {
	create := func() (cmd.Command, *AgentConf) {
		a := &UnitAgent{}
		return a, &a.Conf
	}
	uc := CheckAgentCommand(c, create, []string{"--unit-name", "w0rd-pre55/1"})
	c.Assert(uc.(*UnitAgent).UnitName, Equals, "w0rd-pre55/1")
}

func (s *UnitSuite) TestParseMissing(c *C) {
	uc := &UnitAgent{}
	err := ParseAgentCommand(uc, []string{})
	c.Assert(err, ErrorMatches, "--unit-name option must be set")
}

func (s *UnitSuite) TestParseNonsense(c *C) {
	for _, args := range [][]string{
		[]string{"--unit-name", "wordpress"},
		[]string{"--unit-name", "wordpress/seventeen"},
		[]string{"--unit-name", "wordpress/-32"},
		[]string{"--unit-name", "wordpress/wild/9"},
		[]string{"--unit-name", "20/20"},
	} {
		err := ParseAgentCommand(&UnitAgent{}, args)
		c.Assert(err, ErrorMatches, `--unit-name option expects "<service>/<n>" argument`)
	}
}

func (s *UnitSuite) TestParseUnknown(c *C) {
	uc := &UnitAgent{}
	err := ParseAgentCommand(uc, []string{"--unit-name", "wordpress/1", "thundering typhoons"})
	c.Assert(err, ErrorMatches, `unrecognized args: \["thundering typhoons"\]`)
}

func (s *UnitSuite) TestRunStop(c *C) {
	// Set up state.
	ch := s.AddTestingCharm(c, "dummy")
	svc, err := s.Conn.AddService("dummy", ch)
	c.Assert(err, IsNil)
	unit, err := svc.AddUnit()
	c.Assert(err, IsNil)

	// Set up local environment.
	jujuDir := environs.VarDir
	vers := version.Current.String()
	toolsDir := filepath.Join(jujuDir, "tools", vers)
	err = os.MkdirAll(toolsDir, 0755)
	c.Assert(err, IsNil)
	err = ioutil.WriteFile(filepath.Join(toolsDir, "jujuc"), nil, 0644)
	c.Assert(err, IsNil)
	toolsLink := filepath.Join(jujuDir, "tools", "unit-dummy-0")
	err = os.Symlink(vers, toolsLink)
	c.Assert(err, IsNil)

	// Run a unit agent.
	a := &UnitAgent{
		Conf:     AgentConf{jujuDir, *s.StateInfo(c)},
		UnitName: unit.Name(),
	}
	done := make(chan error)
	go func() {
		done <- a.Run(nil)
	}()
	defer a.Stop()
	timeout := time.After(3 * time.Second)
	for {
		select {
		case <-timeout:
			c.Fatalf("no activity detected")
		case <-time.After(50 * time.Millisecond):
			st, info, err := unit.Status()
			c.Assert(err, IsNil)
			switch st {
			case state.UnitPending, state.UnitInstalled:
				c.Logf("waiting...")
				continue
			case state.UnitStarted:
				c.Logf("started!")
			default:
				c.Fatalf("unexpected status %s %s", st, info)
			}
		}
		break
	}
	err = a.Stop()
	c.Assert(err, IsNil)
	c.Assert(<-done, IsNil)
}

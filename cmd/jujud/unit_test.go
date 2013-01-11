package main

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/version"
	"os"
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
	uc := CheckAgentCommand(c, create, []string{"--unit-name", "w0rd-pre55/1"}, flagAll)
	c.Assert(uc.(*UnitAgent).UnitName, Equals, "w0rd-pre55/1")
}

func (s *UnitSuite) TestParseMissing(c *C) {
	uc := &UnitAgent{}
	err := ParseAgentCommand(uc, []string{})
	c.Assert(err, ErrorMatches, "--unit-name option must be set")
}

func (s *UnitSuite) TestParseNonsense(c *C) {
	for _, args := range [][]string{
		{"--unit-name", "wordpress"},
		{"--unit-name", "wordpress/seventeen"},
		{"--unit-name", "wordpress/-32"},
		{"--unit-name", "wordpress/wild/9"},
		{"--unit-name", "20/20"},
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
	a, unit, _ := s.newAgent(c)
	mgr, reset := patchDeployManager(c, &a.Conf.StateInfo, a.Conf.DataDir)
	defer reset()
	go func() { c.Check(a.Run(nil), IsNil) }()
	defer func() { c.Check(a.Stop(), IsNil) }()
	timeout := time.After(5 * time.Second)

waitStarted:
	for {
		select {
		case <-timeout:
			c.Fatalf("no activity detected")
		case <-time.After(50 * time.Millisecond):
			err := unit.Refresh()
			c.Assert(err, IsNil)
			st, info, err := unit.Status()
			c.Assert(err, IsNil)
			switch st {
			case state.UnitPending, state.UnitInstalled:
				c.Logf("waiting...")
				continue
			case state.UnitStarted:
				c.Logf("started!")
				break waitStarted
			case state.UnitDown:
				s.State.StartSync()
				c.Logf("unit is still down")
			default:
				c.Fatalf("unexpected status %s %s", st, info)
			}
		}
	}

	// Check no subordinates have been deployed.
	mgr.waitDeployed(c)

	// Add a relation with a subordinate service and wait for the subordinate
	// to be deployed...
	_, err := s.State.AddService("logging", s.AddTestingCharm(c, "logging"))
	c.Assert(err, IsNil)
	eps, err := s.State.InferEndpoints([]string{"wordpress", "logging"})
	c.Assert(err, IsNil)
	_, err = s.State.AddRelation(eps...)
	c.Assert(err, IsNil)
	mgr.waitDeployed(c, "logging/0")

	// ...then kill the subordinate and wait for it to be recalled and removed.
	logging0, err := s.State.Unit("logging/0")
	c.Assert(err, IsNil)
	err = logging0.EnsureDead()
	c.Assert(err, IsNil)
	mgr.waitDeployed(c)
	err = logging0.Refresh()
	c.Assert(state.IsNotFound(err), Equals, true)
}

// newAgent starts a new unit agent.
func (s *UnitSuite) newAgent(c *C) (*UnitAgent, *state.Unit, *state.Tools) {
	svc, err := s.Conn.AddService("wordpress", s.AddTestingCharm(c, "wordpress"))
	c.Assert(err, IsNil)
	unit, err := svc.AddUnit()
	c.Assert(err, IsNil)
	err = unit.SetPassword("unit-password")
	c.Assert(err, IsNil)

	dataDir, tools := primeTools(c, s.Conn, version.Current)
	entityName := unit.EntityName()
	tools1, err := environs.ChangeAgentTools(dataDir, entityName, version.Current)
	c.Assert(err, IsNil)
	c.Assert(tools1, DeepEquals, tools)

	err = os.MkdirAll(environs.AgentDir(dataDir, entityName), 0777)
	c.Assert(err, IsNil)

	a := &UnitAgent{
		Conf: AgentConf{
			DataDir:         dataDir,
			StateInfo:       *s.StateInfo(c),
			InitialPassword: "unit-password",
		},
		UnitName: unit.Name(),
	}
	a.Conf.StateInfo.EntityName = entityName
	return a, unit, tools
}

func (s *UnitSuite) TestUpgrade(c *C) {
	newVers := version.Current
	newVers.Patch++
	newTools := uploadTools(c, s.Conn, newVers)
	proposeVersion(c, s.State, newVers.Number, true)
	a, _, currentTools := s.newAgent(c)
	defer a.Stop()
	err := runWithTimeout(a)
	c.Assert(err, FitsTypeOf, &UpgradeReadyError{})
	ug := err.(*UpgradeReadyError)
	c.Assert(ug.NewTools, DeepEquals, newTools)
	c.Assert(ug.OldTools, DeepEquals, currentTools)
}

func (s *UnitSuite) TestWithDeadUnit(c *C) {
	a, unit, _ := s.newAgent(c)
	err := unit.EnsureDead()
	c.Assert(err, IsNil)

	dataDir := a.Conf.DataDir
	a = &UnitAgent{
		Conf: AgentConf{
			DataDir:         dataDir,
			StateInfo:       *s.StateInfo(c),
			InitialPassword: "unit-password",
		},
		UnitName: unit.Name(),
	}
	err = runWithTimeout(a)
	c.Assert(err, IsNil)

	svc, err := s.State.Service(unit.ServiceName())
	c.Assert(err, IsNil)

	// try again when the unit has been removed.
	err = svc.RemoveUnit(unit)
	c.Assert(err, IsNil)
	a = &UnitAgent{
		Conf: AgentConf{
			DataDir:         dataDir,
			StateInfo:       *s.StateInfo(c),
			InitialPassword: "unit-password",
		},
		UnitName: unit.Name(),
	}
	err = runWithTimeout(a)
	c.Assert(err, IsNil)
}

func (s *UnitSuite) TestChangePasswordChanging(c *C) {
	a, unit, _ := s.newAgent(c)
	dataDir := a.Conf.DataDir
	newAgent := func(initialPassword string) runner {
		return &UnitAgent{
			Conf: AgentConf{
				DataDir:         dataDir,
				StateInfo:       *s.StateInfo(c),
				InitialPassword: initialPassword,
			},
			UnitName: unit.Name(),
		}
	}
	testAgentPasswordChanging(&s.JujuConnSuite, c, unit, dataDir, newAgent)
}

package machiner_test

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/container"
	"launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
	coretesting "launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/version"
	"launchpad.net/juju-core/worker/machiner"
	stdtesting "testing"
	"time"
)

func TestPackage(t *stdtesting.T) {
	coretesting.ZkTestPackage(t)
}

type MachinerSuite struct {
	testing.JujuConnSuite
}

var _ = Suite(&MachinerSuite{})

func (s *MachinerSuite) TestMachinerStartStop(c *C) {
	m, err := s.State.AddMachine()
	c.Assert(err, IsNil)

	p := machiner.NewMachiner(m, &state.Info{}, c.MkDir())
	c.Assert(p.Stop(), IsNil)
}

func (s *MachinerSuite) TestMachinerDeployDestroy(c *C) {
	dummyCharm := s.AddTestingCharm(c, "dummy")
	loggingCharm := s.AddTestingCharm(c, "logging")

	d0, err := s.State.AddService("d0", dummyCharm)
	c.Assert(err, IsNil)
	d1, err := s.State.AddService("d1", dummyCharm)
	c.Assert(err, IsNil)
	sub0, err := s.State.AddService("sub0", loggingCharm)
	c.Assert(err, IsNil)

	// Add one unit to start with.
	ud0, err := d0.AddUnit()
	c.Assert(err, IsNil)
	_, err = sub0.AddUnitSubordinateTo(ud0)
	c.Assert(err, IsNil)

	ud1, err := d1.AddUnit()
	c.Assert(err, IsNil)

	m0, err := s.State.AddMachine()
	c.Assert(err, IsNil)

	m1, err := s.State.AddMachine()
	c.Assert(err, IsNil)

	err = ud0.AssignToMachine(m0)
	c.Assert(err, IsNil)

	stateInfo := &state.Info{}

	expectedTools := &state.Tools{Binary: version.Current}

	dataDir := c.MkDir()

	action := make(chan string, 5)
	*machiner.Deploy = func(cfg container.Config, u *state.Unit, info *state.Info, tools *state.Tools) error {
		c.Check(info, Equals, stateInfo)
		c.Check(tools, DeepEquals, expectedTools)
		c.Check(cfg.DataDir, Equals, dataDir)
		action <- "+" + u.Name()
		return nil
	}
	*machiner.Destroy = func(cfg container.Config, u *state.Unit) error {
		c.Check(cfg.DataDir, Equals, dataDir)
		action <- "-" + u.Name()
		return nil
	}
	machiner := machiner.NewMachiner(m0, stateInfo, dataDir)

	tests := []struct {
		change  func()
		actions []string
	}{
		{
			func() {},
			[]string{"+d0/0"},
		}, {
			func() {
				err := ud1.AssignToMachine(m0)
				c.Assert(err, IsNil)
			},
			[]string{"+d1/0"},
		}, {
			func() {
				err := ud0.UnassignFromMachine()
				c.Assert(err, IsNil)
			},
			[]string{"-d0/0"},
		}, {
			func() {
				err := ud1.UnassignFromMachine()
				c.Assert(err, IsNil)
			},
			[]string{"-d1/0"},
		}, {
			func() {
				err := ud0.AssignToMachine(m1)
				c.Assert(err, IsNil)
			},
			nil,
		},
	}
	for i, t := range tests {
		c.Logf("test %d", i)
		t.change()
		for _, a := range t.actions {
			checkAction(c, action, a)
		}
		checkAction(c, action, "")
	}

	err = machiner.Stop()
	c.Assert(err, IsNil)
}

func checkAction(c *C, action <-chan string, expect string) {
	timeout := 500 * time.Millisecond
	if expect == "" {
		timeout = 200 * time.Millisecond
	}
	select {
	case a := <-action:
		c.Assert(a, Equals, expect)
	case <-time.After(timeout):
		if expect != "" {
			c.Fatalf("expected action %v got nothing", expect)
		}
	}
}

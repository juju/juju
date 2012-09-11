package machiner_test

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/container"
	"launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/version"
	coretesting "launchpad.net/juju-core/testing"
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


	oldNewSimpleContainer := *machiner.NewSimpleContainer
	defer func() {
		*machiner.NewSimpleContainer = oldNewSimpleContainer
	}()
	stateInfo := &state.Info{}
	dcontainer := &dummyContainer{
		c: c,
		expectedTools: &state.Tools{Binary: version.Current},
		expectedStateInfo:  stateInfo,
		action: make(chan string, 5),
	}
	*machiner.NewSimpleContainer = func(string) container.Container {
		return dcontainer
	}

	machiner := machiner.NewMachiner(m0, stateInfo, c.MkDir())

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
			dcontainer.checkAction(c, a)
		}
		dcontainer.checkAction(c, "")
	}

	err = machiner.Stop()
	c.Assert(err, IsNil)
}

type dummyContainer struct {
	c *C
	expectedStateInfo *state.Info
	expectedTools *state.Tools
	action chan string
}

var _ container.Container = (*dummyContainer)(nil)


func (d *dummyContainer) Deploy(u *state.Unit, info *state.Info, tools *state.Tools) error {
	d.c.Check(info, Equals, d.expectedStateInfo)
	d.c.Check(tools, DeepEquals, d.expectedTools)
	d.action <- "+" + u.Name()
	return nil
}

func (d *dummyContainer) Destroy(u *state.Unit) error {
	d.action <- "-" + u.Name()
	return nil
}

func (d *dummyContainer) checkAction(c *C, action string) {
	timeout := 500 * time.Millisecond
	if action == "" {
		timeout = 200 * time.Millisecond
	}
	select {
	case a := <-d.action:
		c.Assert(a, Equals, action)
	case <-time.After(timeout):
		if action != "" {
			c.Fatalf("expected action %v got nothing", action)
		}
	}
}

package machiner_test

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/container"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/testing"
	coretesting "launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/worker/machiner"
	stdtesting "testing"
	"time"
)

func TestPackage(t *stdtesting.T) {
	coretesting.ZkTestPackage(t)
}

type MachinerSuite struct {
	coretesting.LoggingSuite
	testing.StateSuite
}

var _ = Suite(&MachinerSuite{})

func (s *MachinerSuite) SetUpTest(c *C) {
	s.LoggingSuite.SetUpTest(c)
	s.StateSuite.SetUpTest(c)
}

func (s *MachinerSuite) TearDownTest(c *C) {
	s.StateSuite.TearDownTest(c)
	s.LoggingSuite.TearDownTest(c)
}

func (s *MachinerSuite) TestMachinerStartStop(c *C) {
	m, err := s.State.AddMachine()
	c.Assert(err, IsNil)

	p, err := machiner.NewMachiner(s.StateInfo(c), m.Id())
	c.Assert(err, IsNil)
	c.Assert(p.Stop(), IsNil)
}

func (s *MachinerSuite) TestMachinerDeployDestroy(c *C) {
	dcontainer := newDummyContainer()
	backup := container.Simple
	container.Simple = dcontainer
	defer func() {
		container.Simple = backup
	}()

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

	machiner, err := machiner.NewMachiner(s.StateInfo(c), m0.Id())
	c.Assert(err, IsNil)

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
	action chan string
}

func newDummyContainer() *dummyContainer {
	return &dummyContainer{
		make(chan string, 5),
	}
}

func (d *dummyContainer) Deploy(u *state.Unit) error {
	d.action <- "+" + u.Name()
	return nil
}

func (d *dummyContainer) Destroy(u *state.Unit) error {
	d.action <- "-" + u.Name()
	return nil
}

func (d *dummyContainer) ToolsDir(u *state.Unit) string {
	return "/dummy/tools"
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

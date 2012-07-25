package firewaller_test

import (
	"fmt"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/dummy"
	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/testing"
	coretesting "launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/worker/firewaller"
	"sort"
	"strings"
	stdtesting "testing"
	"time"
)

func TestPackage(t *stdtesting.T) {
	coretesting.ZkTestPackage(t)
}

// hooLogger allows the grabbing of debug log statements
// to compare them inside the tests.
type hookLogger struct {
	event     chan string
	oldTarget log.Logger
}

var logHook *hookLogger

const prefix = "JUJU:DEBUG firewaller: "

func (h *hookLogger) Output(calldepth int, s string) error {
	err := h.oldTarget.Output(calldepth, s)
	if strings.HasPrefix(s, prefix) {
		h.event <- s[len(prefix):]
	}
	return err
}

func setUpLogHook() {
	logHook = &hookLogger{
		event:     make(chan string, 30),
		oldTarget: log.Target,
	}
	log.Target = logHook
}

func tearDownLogHook() {
	log.Target = logHook.oldTarget
}

// assertEvents asserts that the expected events are received from
// the firewaller, in no particular order.
func assertEvents(c *C, expect []string) {
	got := []string{}
	for _ = range expect {
		select {
		case e := <-logHook.event:
			got = append(got, e)
		case <-time.After(500 * time.Millisecond):
			c.Fatalf("expected %q; timed out, got %q", expect, got)
		}
	}
	select {
	case e := <-logHook.event:
		got = append(got, e)
		c.Fatalf("expected %q; too many events %q ", expect, got)
	case <-time.After(100 * time.Millisecond):
	}
	sort.Strings(expect)
	sort.Strings(got)
	c.Assert(got, DeepEquals, expect)
}

type FirewallerSuite struct {
	coretesting.LoggingSuite
	testing.StateSuite
	environ environs.Environ
	op      <-chan dummy.Operation
	charm   *state.Charm
}

var _ = Suite(&FirewallerSuite{})

func (s *FirewallerSuite) SetUpTest(c *C) {
	s.LoggingSuite.SetUpTest(c)

	op := make(chan dummy.Operation, 500)
	dummy.Listen(op)
	s.op = op

	var err error
	config := map[string]interface{}{
		"name":            "testing",
		"type":            "dummy",
		"zookeeper":       true,
		"authorized-keys": "i-am-a-key",
	}
	s.environ, err = environs.NewFromAttrs(config)
	c.Assert(err, IsNil)
	err = s.environ.Bootstrap(false)
	c.Assert(err, IsNil)

	// Sanity check
	info, err := s.environ.StateInfo()
	c.Assert(err, IsNil)
	c.Assert(info, DeepEquals, s.StateInfo(c))

	s.StateSuite.SetUpTest(c)

	s.charm = s.AddTestingCharm(c, "dummy")
}

func (s *FirewallerSuite) TearDownTest(c *C) {
	dummy.Reset()
	s.LoggingSuite.TearDownTest(c)
}

func (s *FirewallerSuite) TestStartStop(c *C) {
	fw, err := firewaller.NewFirewaller(s.environ, s.State)
	c.Assert(err, IsNil)
	c.Assert(fw.Stop(), IsNil)
}

func (s *FirewallerSuite) TestAddRemoveMachine(c *C) {
	fw, err := firewaller.NewFirewaller(s.environ, s.State)
	c.Assert(err, IsNil)
	defer func() { c.Assert(fw.Stop(), IsNil) }()

	setUpLogHook()
	defer tearDownLogHook()

	m1, err := s.State.AddMachine()
	c.Assert(err, IsNil)
	m2, err := s.State.AddMachine()
	c.Assert(err, IsNil)
	m3, err := s.State.AddMachine()
	c.Assert(err, IsNil)

	assertEvents(c, []string{
		fmt.Sprint("started watching machine ", m1.Id()),
		fmt.Sprint("started watching machine ", m2.Id()),
		fmt.Sprint("started watching machine ", m3.Id()),
	})

	err = s.State.RemoveMachine(m2.Id())
	c.Assert(err, IsNil)

	assertEvents(c, []string{
		fmt.Sprint("stopped watching machine ", m2.Id()),
	})
}

func (s *FirewallerSuite) TestAssignUnassignUnit(c *C) {
	fw, err := firewaller.NewFirewaller(s.environ, s.State)
	c.Assert(err, IsNil)
	defer func() { c.Assert(fw.Stop(), IsNil) }()

	setUpLogHook()
	defer tearDownLogHook()

	m1, err := s.State.AddMachine()
	c.Assert(err, IsNil)
	err = m1.SetInstanceId("testing-0")
	c.Assert(err, IsNil)
	_, err = s.environ.StartInstance(m1.Id(), s.StateInfo(c))
	c.Assert(err, IsNil)
	m2, err := s.State.AddMachine()
	c.Assert(err, IsNil)
	_, err = s.environ.StartInstance(m2.Id(), s.StateInfo(c))
	c.Assert(err, IsNil)
	err = m2.SetInstanceId("testing-1")
	c.Assert(err, IsNil)
	s1, err := s.State.AddService("wordpress", s.charm)
	c.Assert(err, IsNil)
	u1, err := s1.AddUnit()
	c.Assert(err, IsNil)
	err = u1.AssignToMachine(m1)
	c.Assert(err, IsNil)
	u2, err := s1.AddUnit()
	c.Assert(err, IsNil)
	err = u2.AssignToMachine(m2)
	c.Assert(err, IsNil)

	assertEvents(c, []string{
		fmt.Sprint("started watching machine ", m1.Id()),
		fmt.Sprint("started watching machine ", m2.Id()),
		fmt.Sprint("started watching unit ", u1.Name()),
		fmt.Sprint("started watching unit ", u2.Name()),
	})

	err = u1.UnassignFromMachine()
	c.Assert(err, IsNil)

	assertEvents(c, []string{
		fmt.Sprint("stopped watching unit ", u1.Name()),
	})
}

func (s *FirewallerSuite) TestOpenClosePorts(c *C) {
	fw, err := firewaller.NewFirewaller(s.environ, s.State)
	c.Assert(err, IsNil)
	defer func() { c.Assert(fw.Stop(), IsNil) }()

	setUpLogHook()
	defer tearDownLogHook()

	// Scenario 1: Service has *not* been exposed.
	m1, err := s.State.AddMachine()
	c.Assert(err, IsNil)
	err = m1.SetInstanceId("testing-0")
	c.Assert(err, IsNil)
	_, err = s.environ.StartInstance(m1.Id(), s.StateInfo(c))
	c.Assert(err, IsNil)
	s1, err := s.State.AddService("wordpress", s.charm)
	c.Assert(err, IsNil)
	u1, err := s1.AddUnit()
	c.Assert(err, IsNil)
	err = u1.AssignToMachine(m1)
	c.Assert(err, IsNil)
	err = u1.OpenPort("tcp", 80)
	c.Assert(err, IsNil)

	assertEvents(c, []string{
		fmt.Sprint("started watching machine ", m1.Id()),
		fmt.Sprint("started watching unit ", u1.Name()),
	})

	err = u1.ClosePort("tcp", 80)
	c.Assert(err, IsNil)

	assertEvents(c, []string{})

	// Scenario 2: Service has been exposed.
	m2, err := s.State.AddMachine()
	c.Assert(err, IsNil)
	err = m2.SetInstanceId("testing-1")
	c.Assert(err, IsNil)
	_, err = s.environ.StartInstance(m2.Id(), s.StateInfo(c))
	c.Assert(err, IsNil)
	s2, err := s.State.AddService("mysql", s.charm)
	c.Assert(err, IsNil)
	err = s2.SetExposed()
	c.Assert(err, IsNil)
	u2, err := s2.AddUnit()
	c.Assert(err, IsNil)
	err = u2.AssignToMachine(m2)
	c.Assert(err, IsNil)
	err = u2.OpenPort("tcp", 3306)
	c.Assert(err, IsNil)

	assertEvents(c, []string{
		fmt.Sprint("started watching machine ", m2.Id()),
		fmt.Sprint("started watching unit ", u2.Name()),
		fmt.Sprintf("opened ports [{tcp 3306}] on machine %d", m2.Id()),
	})

	err = u2.ClosePort("tcp", 3306)
	c.Assert(err, IsNil)

	assertEvents(c, []string{
		fmt.Sprintf("closed ports [{tcp 3306}] on machine %d", m2.Id()),
	})
}

func (s *FirewallerSuite) TestFirewallerStopOnStateClose(c *C) {
	fw, err := firewaller.NewFirewaller(s.environ, s.State)
	c.Assert(err, IsNil)
	fw.CloseState()
	c.Check(fw.Wait(), ErrorMatches, ".* zookeeper is closing")
	c.Assert(fw.Stop(), ErrorMatches, ".* zookeeper is closing")
}

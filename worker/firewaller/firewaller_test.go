package firewaller_test

import (
	"fmt"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/environs/dummy"
	"launchpad.net/juju-core/log"
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
	var got []string
	for _ = range expect {
		select {
		case e := <-logHook.event:
			got = append(got, e)
		case <-time.After(500 * time.Millisecond):
			c.Fatalf("expected %q; timed out after %q", expect, got)
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
	op <-chan dummy.Operation
}

var _ = Suite(&FirewallerSuite{})

func (s *FirewallerSuite) SetUpTest(c *C) {
	s.LoggingSuite.SetUpTest(c)

	op := make(chan dummy.Operation, 500)
	dummy.Listen(op)
	s.op = op

	s.StateSuite.SetUpTest(c)
}

func (s *FirewallerSuite) TearDownTest(c *C) {
	dummy.Reset()
	s.LoggingSuite.TearDownTest(c)
}

func (s *FirewallerSuite) TestStartStop(c *C) {
	fw, err := firewaller.NewFirewaller(s.State)
	c.Assert(err, IsNil)
	c.Assert(fw.Stop(), IsNil)
}

func (s *FirewallerSuite) TestAddRemoveMachine(c *C) {
	fw, err := firewaller.NewFirewaller(s.State)
	c.Assert(err, IsNil)

	setUpLogHook()
	defer tearDownLogHook()

	m1, err := s.State.AddMachine()
	c.Assert(err, IsNil)
	m2, err := s.State.AddMachine()
	c.Assert(err, IsNil)
	m3, err := s.State.AddMachine()
	c.Assert(err, IsNil)

	assertEvents(c, []string{
		fmt.Sprint("add-machine ", m1.Id()),
		fmt.Sprint("add-machine ", m2.Id()),
		fmt.Sprint("add-machine ", m3.Id()),
	})

	err = s.State.RemoveMachine(m2.Id())
	c.Assert(err, IsNil)

	assertEvents(c, []string{
		fmt.Sprint("remove-machine ", m2.Id()),
	})

	c.Assert(fw.Stop(), IsNil)
}

func (s *FirewallerSuite) TestFirewallerStopOnStateClose(c *C) {
	fw, err := firewaller.NewFirewaller(s.State)
	c.Assert(err, IsNil)
	fw.CloseState()
	c.Check(fw.Wait(), ErrorMatches, ".* zookeeper is closing")
	c.Assert(fw.Stop(), ErrorMatches, ".* zookeeper is closing")
}

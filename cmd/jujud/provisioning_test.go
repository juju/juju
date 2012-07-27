package main

import (
	"fmt"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/dummy"
	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/state/testing"
	coretesting "launchpad.net/juju-core/testing"
	"sort"
	"strings"
	"time"
)

// hookLogger allows the grabbing of log statements
// to compare them inside the tests.
type hookLogger struct {
	event     chan string
	oldTarget log.Logger
}

var logHook *hookLogger

const prefix = "JUJU:DEBUG provisioning: "

func (h *hookLogger) Output(calldepth int, s string) error {
	err := h.oldTarget.Output(calldepth, s)
	if strings.HasPrefix(s, prefix) {
		h.event <- s[len(prefix):]
	}
	return err
}

func setUpLogHook() {
	logHook = &hookLogger{
		event:     make(chan string, 100),
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

type ProvisioningSuite struct {
	coretesting.LoggingSuite
	testing.StateSuite
	op <-chan dummy.Operation
}

var _ = Suite(&ProvisioningSuite{})

func (s *ProvisioningSuite) SetUpTest(c *C) {
	s.LoggingSuite.SetUpTest(c)

	// Create the operations channel with more than enough space
	// for those tests that don't listen on it.
	op := make(chan dummy.Operation, 500)
	dummy.Listen(op)
	s.op = op

	env, err := environs.NewFromAttrs(map[string]interface{}{
		"name":            "testing",
		"type":            "dummy",
		"zookeeper":       true,
		"authorized-keys": "i-am-a-key",
	})
	c.Assert(err, IsNil)
	err = env.Bootstrap(false)
	c.Assert(err, IsNil)

	s.StateSuite.SetUpTest(c)
}

func (s *ProvisioningSuite) TearDownTest(c *C) {
	dummy.Reset()
	s.LoggingSuite.TearDownTest(c)
}

func (s *ProvisioningSuite) TestParseSuccess(c *C) {
	create := func() (cmd.Command, *AgentConf) {
		a := &ProvisioningAgent{}
		return a, &a.Conf
	}
	CheckAgentCommand(c, create, []string{})
}

func (s *ProvisioningSuite) TestParseUnknown(c *C) {
	a := &ProvisioningAgent{}
	err := ParseAgentCommand(a, []string{"nincompoops"})
	c.Assert(err, ErrorMatches, `unrecognized args: \["nincompoops"\]`)
}

func (s *ProvisioningSuite) TestRunStop(c *C) {
	a := &ProvisioningAgent{
		Conf: AgentConf{
			JujuDir:   "/var/lib/juju",
			StateInfo: *s.StateInfo(c),
		},
	}

	setUpLogHook()
	defer tearDownLogHook()

	go func() {
		err := a.Run(nil)
		c.Assert(err, IsNil)
	}()

	assertEvents(c, []string{
		fmt.Sprint("opened state"),
		fmt.Sprint("started provisioner"),
		fmt.Sprint("started firewaller"),
	})

	err := a.Stop()
	c.Assert(err, IsNil)

	assertEvents(c, []string{
		fmt.Sprint("stopped firewaller"),
		fmt.Sprint("stopped provisioner"),
		fmt.Sprint("closed state"),
	})
}

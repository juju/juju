package state_test

import (
	"fmt"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/version"
	"time"
)

type tooler interface {
	AgentTools() (*state.Tools, error)
	SetAgentTools(t *state.Tools) error
	ProposedAgentTools() (*state.Tools, error)
	ProposeAgentTools(t *state.Tools) error
	WatchAgentTools() *state.AgentToolsWatcher
	WatchProposedAgentTools() *state.AgentToolsWatcher
}

var _ = Suite(&ToolsSuite{})

type ToolsSuite struct {
	ConnSuite
}

func newTools(vers, url string) *state.Tools {
	return &state.Tools{
		Binary: version.MustParseBinary(vers),
		URL:    url,
	}
}

func testAgentTools(c *C, obj tooler, agent string) {
	// object starts with zero'd tools.
	t, err := obj.AgentTools()
	c.Assert(err, IsNil)
	c.Assert(t, DeepEquals, &state.Tools{})

	t, err = obj.ProposedAgentTools()
	c.Assert(err, IsNil)
	c.Assert(t, DeepEquals, &state.Tools{})

	err = obj.ProposeAgentTools(&state.Tools{})
	c.Assert(err, ErrorMatches, fmt.Sprintf("cannot set proposed tools for %s agent: empty series or arch", agent))
	// check that we can set the version
	t0 := newTools("5.6.7-precise-amd64", "http://foo/bar.tgz")
	err = obj.ProposeAgentTools(t0)
	c.Assert(err, IsNil)
	t1, err := obj.ProposedAgentTools()
	c.Assert(err, IsNil)
	c.Assert(t1, DeepEquals, t0)

	err = obj.SetAgentTools(&state.Tools{})
	c.Assert(err, ErrorMatches, fmt.Sprintf("cannot set current tools for %s agent: empty series or arch", agent))
	t2 := newTools("7.8.9-foo-bar", "http://arble.tgz")
	err = obj.SetAgentTools(t2)
	c.Assert(err, IsNil)
	t3, err := obj.AgentTools()
	c.Assert(err, IsNil)
	c.Assert(t3, DeepEquals, t2)

	// check there's no cross-talk
	t4, err := obj.ProposedAgentTools()
	c.Assert(err, IsNil)
	c.Assert(t4, DeepEquals, t0)
}

// toolSetter provides an interface that lets us test a tools watcher
// independently of whether we are watching current or proposed tools.
type toolSetter interface {
	setTools(t *state.Tools) error
	setOtherTools(t *state.Tools) error
	watch() *state.AgentToolsWatcher
}

type agentToolsSetter struct {
	t tooler
}

func (s agentToolsSetter) setTools(t *state.Tools) error {
	return s.t.SetAgentTools(t)
}
func (s agentToolsSetter) setOtherTools(t *state.Tools) error {
	return s.t.ProposeAgentTools(t)
}
func (s agentToolsSetter) watch() *state.AgentToolsWatcher {
	return s.t.WatchAgentTools()
}

type proposedAgentToolsSetter struct {
	t tooler
}

func (s proposedAgentToolsSetter) setTools(t *state.Tools) error {
	return s.t.ProposeAgentTools(t)
}
func (s proposedAgentToolsSetter) setOtherTools(t *state.Tools) error {
	return s.t.SetAgentTools(t)
}
func (s proposedAgentToolsSetter) watch() *state.AgentToolsWatcher {
	return s.t.WatchProposedAgentTools()
}

var agentToolsWatcherTests = []struct {
	test func(t toolSetter) error
	want *state.Tools
}{
	{
		func(t toolSetter) error { return nil },
		&state.Tools{},
	},
	{
		func(t toolSetter) error {
			return t.setTools(newTools("7.8.9-foo-bar", "http://arble.tgz"))
		},
		newTools("7.8.9-foo-bar", "http://arble.tgz"),
	},
	{
		func(t toolSetter) error {
			return t.setTools(newTools("7.8.9-foo-bar", "http://foo.com"))
		},
		newTools("7.8.9-foo-bar", "http://foo.com"),
	},
	{
		func(t toolSetter) error {
			return t.setTools(newTools("1.1.1-x-y", "http://foo.com"))
		},
		newTools("1.1.1-x-y", "http://foo.com"),
	},
	{
		func(t toolSetter) error {
			return t.setTools(newTools("1.1.1-x-y", "http://foo.com"))
		},
		nil,
	},
	{
		func(t toolSetter) error {
			return t.setOtherTools(newTools("1.1.1-x-y", "http://nowhere"))
		},
		nil,
	},
}

func testAgentToolsWatcher(c *C, setter toolSetter, kind string) {
	c.Logf("watcher tests for %v tools", kind)
	w := setter.watch()
	for i, t := range agentToolsWatcherTests {
		c.Logf("test %d", i)
		err := t.test(setter)
		c.Assert(err, IsNil)
		timeout := 200 * time.Millisecond
		if t.want == nil {
			timeout /= 2
		}
		select {
		case got, ok := <-w.Changes():
			c.Assert(ok, Equals, true)
			if t.want == nil {
				c.Fatalf("got change %v; expected nothing", got)
			} else {
				c.Assert(got, DeepEquals, t.want)
			}
		case <-time.After(timeout):
			if t.want != nil {
				c.Fatalf("got no change, expected %v", t.want)
			}
		}
	}
}

func (s *ToolsSuite) TestMachineAgentTools(c *C) {
	m, err := s.State.AddMachine()
	c.Assert(err, IsNil)
	testAgentTools(c, m, "machine")

	m, err = s.State.AddMachine()
	c.Assert(err, IsNil)
	testAgentToolsWatcher(c, agentToolsSetter{m}, "machine current")

	m, err = s.State.AddMachine()
	c.Assert(err, IsNil)
	testAgentToolsWatcher(c, proposedAgentToolsSetter{m}, "machine proposed")
}

func (s *ToolsSuite) TestUnitAgentTools(c *C) {
	charm := s.AddTestingCharm(c, "dummy")
	svc, err := s.State.AddService("wordpress", charm)
	c.Assert(err, IsNil)
	unit, err := svc.AddUnit()
	c.Assert(err, IsNil)
	testAgentTools(c, unit, "unit")

	unit, err = svc.AddUnit()
	c.Assert(err, IsNil)
	testAgentToolsWatcher(c, agentToolsSetter{unit}, "unit current")

	unit, err = svc.AddUnit()
	c.Assert(err, IsNil)
	testAgentToolsWatcher(c, proposedAgentToolsSetter{unit}, "unit proposed")
}

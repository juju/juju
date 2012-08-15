package state_test

import (
	"fmt"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/version"
	"strings"
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

var agentToolsWatcherTests = []struct {
	version string
	url string
	other bool		// whether to set the "other" tools (current <-> proposed)
	want *state.Tools
}{
	{
		want: &state.Tools{},
	},
	{
		version:  "7.8.9-foo-bar",
		url: "http://arble.tgz",
		want: newTools("7.8.9-foo-bar", "http://arble.tgz"),
	},
	{
		version: "7.8.9-foo-bar",
		url: "http://foo.com",
		want: newTools("7.8.9-foo-bar", "http://foo.com"),
	},
	{
		version: "1.1.1-x-y",
		url: "http://foo.com",
		want: newTools("1.1.1-x-y", "http://foo.com"),
	},
	{
		version: "1.1.1-x-y",
		url: "http://foo.com",
		want: nil,
	},
	{
		other: true,
		version: "1.1.1-x-y",
		url: "http://nowhere",
		want: nil,
	},
}

func testAgentToolsWatcher(c *C, t tooler, w *state.AgentToolsWatcher, kind string) {
	c.Logf("watcher tests for %v tools", kind)
	current := strings.HasSuffix(kind, "current")
	for i, test := range agentToolsWatcherTests {
		c.Logf("test %d", i)
		if test.version != "" {
			if current != test.other {
				err := t.SetAgentTools(newTools(test.version, test.url))
				c.Assert(err, IsNil)
			} else {
				err := t.ProposeAgentTools(newTools(test.version, test.url))
				c.Assert(err, IsNil)
			}
		}
		timeout := 200 * time.Millisecond
		if test.want == nil {
			timeout /= 2
		}
		select {
		case got, ok := <-w.Changes():
			c.Assert(ok, Equals, true)
			if test.want == nil {
				c.Fatalf("got change %v; expected nothing", got)
			} else {
				c.Assert(got, DeepEquals, test.want)
			}
		case <-time.After(timeout):
			if test.want != nil {
				c.Fatalf("got no change, expected %v", test.want)
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
	testAgentToolsWatcher(c, m, m.WatchAgentTools(), "machine current")

	m, err = s.State.AddMachine()
	c.Assert(err, IsNil)
	testAgentToolsWatcher(c, m, m.WatchProposedAgentTools(), "machine proposed")
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
	testAgentToolsWatcher(c, unit, unit.WatchAgentTools(), "unit current")

	unit, err = svc.AddUnit()
	c.Assert(err, IsNil)
	testAgentToolsWatcher(c, unit, unit.WatchProposedAgentTools(), "unit proposed")
}

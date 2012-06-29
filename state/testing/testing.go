package testing

import (
	"fmt"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/testing"
	"net/url"
)

// StateSuite provides a pre-initialized state for each test method.
type StateSuite struct {
	testing.ZkSuite
	St *state.State
}

func (s *StateSuite) SetUpTest(c *C) {
	var err error
	s.St, err = state.Initialize(s.StateInfo(c))
	c.Assert(err, IsNil)
}

func (s *StateSuite) TearDownTest(c *C) {
	s.ZkSuite.TearDownTest(c)
	c.Assert(s.St.Close(), IsNil)
}

func (s *StateSuite) AssertMachineCount(c *C, expect int) {
	ms, err := s.St.AllMachines()
	c.Assert(err, IsNil)
	c.Assert(len(ms), Equals, expect)
}

func (s *StateSuite) StateInfo(c *C) *state.Info {
	return &state.Info{Addrs: []string{testing.ZkAddr}}
}

func (s *StateSuite) AddTestingCharm(c *C, name string) *state.Charm {
	ch := testing.Charms.Dir(name)
	ident := fmt.Sprintf("%s-%d", name, ch.Revision())
	curl := charm.MustParseURL("local:series/" + ident)
	bundleURL, err := url.Parse("http://bundles.example.com/" + ident)
	c.Assert(err, IsNil)
	sch, err := s.St.AddCharm(ch, curl, bundleURL, ident+"-sha256")
	c.Assert(err, IsNil)
	return sch
}

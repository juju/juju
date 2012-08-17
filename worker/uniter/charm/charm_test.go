package charm_test

import (
	"io/ioutil"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/worker/uniter/charm"
	"path/filepath"
	"testing"
)

func Test(t *testing.T) {
	TestingT(t)
}

type StateFileSuite struct{}

var _ = Suite(&StateFileSuite{})

func (s *StateFileSuite) TestStateFile(c *C) {
	path := filepath.Join(c.MkDir(), "charm")
	f := charm.StateFile(path)
	st, err := f.Read()
	c.Assert(err, IsNil)
	c.Assert(st, DeepEquals, charm.State{charm.Missing})

	err = ioutil.WriteFile(path, []byte("roflcopter"), 0644)
	c.Assert(err, IsNil)
	_, err = f.Read()
	c.Assert(err, ErrorMatches, "invalid charm state at "+path)

	bad := func() { f.Write(charm.Status("claptrap")) }
	c.Assert(bad, PanicMatches, `unknown charm status "claptrap"`)

	bad = func() { f.Write(charm.Missing) }
	c.Assert(bad, PanicMatches, `insane operation`)

	err = f.Write(charm.Installed)
	c.Assert(err, IsNil)
	st, err = f.Read()
	c.Assert(err, IsNil)
	c.Assert(st, DeepEquals, charm.State{charm.Installed})
}

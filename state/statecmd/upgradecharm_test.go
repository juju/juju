package statecmd_test

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/statecmd"
	coretest "launchpad.net/juju-core/testing"
)

type UpgradeCharmSuite struct {
	testing.JujuConnSuite
}

var _ = Suite(&UpgradeCharmSuite{})

var serviceUpgradeCharmTests = []struct {
	about   string
	charm   string
	service string
	force   bool
}{
	{
		about:   "upgrade a charm",
		charm:   "upgrade1",
		service: "upgrade",
		force:   false,
	},
	{
		about:   "force upgrade a charm",
		charm:   "upgrade1",
		service: "upgrade",
		force:   true,
	},
}

func (s *UpgradeCharmSuite) TestServiceUpgradeCharm(c *C) {
	for i, t := range serviceUpgradeCharmTests {
		c.Logf("test %d. %s", i, t.about)
		charm := s.AddTestingCharm(c, t.charm)
		svc, err := s.State.AddService(t.service, charm)
		c.Assert(err, IsNil)
		c.Assert(svc.Life(), Equals, state.Alive)
		c.Logf("Svc: %+v", svc)
		c.Logf("Charm: %+v", charm)
		err = statecmd.ServiceUpgradeCharm(s.State, params.ServiceUpgradeCharm{
			ServiceName: t.service,
			Force:       t.force,
			RepoPath:    coretest.Charms.Path,
		})
		c.Assert(err, IsNil)
		err = svc.Destroy()
		c.Assert(err, IsNil)
	}
}

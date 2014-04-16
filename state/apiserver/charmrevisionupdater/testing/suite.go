// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"fmt"
	"net/url"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/charm"
	charmtesting "launchpad.net/juju-core/charm/testing"
	jujutesting "launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
	coretesting "launchpad.net/juju-core/testing"
)

// CharmSuite provides infrastructure to set up and perform tests associated
// with charm versioning. A mock charm store is created using some known charms
// used for testing.
type CharmSuite struct {
	jcSuite *jujutesting.JujuConnSuite

	Server *charmtesting.MockStore
	charms map[string]*state.Charm
}

func (s *CharmSuite) SetUpSuite(c *gc.C, jcSuite *jujutesting.JujuConnSuite) {
	s.jcSuite = jcSuite
	s.Server = charmtesting.NewMockStore(c, map[string]int{
		"cs:quantal/mysql":     23,
		"cs:quantal/dummy":     24,
		"cs:quantal/riak":      25,
		"cs:quantal/wordpress": 26,
		"cs:quantal/logging":   27,
		"cs:quantal/borken":    28,
	})
}

func (s *CharmSuite) TearDownSuite(c *gc.C) {
	s.Server.Close()
}

func (s *CharmSuite) SetUpTest(c *gc.C) {
	s.jcSuite.PatchValue(&charm.CacheDir, c.MkDir())
	s.jcSuite.PatchValue(&charm.Store, &charm.CharmStore{BaseURL: s.Server.Address()})
	s.Server.Downloads = nil
	s.Server.Authorizations = nil
	s.Server.Metadata = nil
	s.charms = make(map[string]*state.Charm)
}

func (s *CharmSuite) TearDownTest(c *gc.C) {
}

// UpdateStoreRevision sets the revision of the specified charm to rev.
func (s *CharmSuite) UpdateStoreRevision(ch string, rev int) {
	s.Server.UpdateStoreRevision(ch, rev)
}

// AddMachine adds a new machine to state.
func (s *CharmSuite) AddMachine(c *gc.C, machineId string, job state.MachineJob) {
	m, err := s.jcSuite.State.AddOneMachine(state.MachineTemplate{
		Series: "quantal",
		Jobs:   []state.MachineJob{job},
	})
	c.Assert(err, gc.IsNil)
	c.Assert(m.Id(), gc.Equals, machineId)
	cons, err := m.Constraints()
	c.Assert(err, gc.IsNil)
	inst, hc := jujutesting.AssertStartInstanceWithConstraints(c, s.jcSuite.Conn.Environ, m.Id(), cons)
	err = m.SetProvisioned(inst.Id(), "fake_nonce", hc)
	c.Assert(err, gc.IsNil)

}

// AddCharmWithRevision adds a charm with the specified revision to state.
func (s *CharmSuite) AddCharmWithRevision(c *gc.C, charmName string, rev int) *state.Charm {
	ch := coretesting.Charms.Dir(charmName)
	name := ch.Meta().Name
	curl := charm.MustParseURL(fmt.Sprintf("cs:quantal/%s-%d", name, rev))
	bundleURL, err := url.Parse(fmt.Sprintf("http://bundles.testing.invalid/%s-%d", name, rev))
	c.Assert(err, gc.IsNil)
	dummy, err := s.jcSuite.State.AddCharm(ch, curl, bundleURL, fmt.Sprintf("%s-%d-sha256", name, rev))
	c.Assert(err, gc.IsNil)
	s.charms[name] = dummy
	return dummy
}

// AddService adds a service for the specified charm to state.
func (s *CharmSuite) AddService(c *gc.C, charmName, serviceName string, includeNetworks, excludeNetworks []string) {
	ch, ok := s.charms[charmName]
	c.Assert(ok, gc.Equals, true)
	_, err := s.jcSuite.State.AddService(serviceName, "user-admin", ch, includeNetworks, excludeNetworks)
	c.Assert(err, gc.IsNil)
}

// AddUnit adds a new unit for service to the specified machine.
func (s *CharmSuite) AddUnit(c *gc.C, serviceName, machineId string) {
	svc, err := s.jcSuite.State.Service(serviceName)
	c.Assert(err, gc.IsNil)
	u, err := svc.AddUnit()
	c.Assert(err, gc.IsNil)
	m, err := s.jcSuite.State.Machine(machineId)
	c.Assert(err, gc.IsNil)
	err = u.AssignToMachine(m)
	c.Assert(err, gc.IsNil)
}

// SetUnitRevision sets the unit's charm to the specified revision.
func (s *CharmSuite) SetUnitRevision(c *gc.C, unitName string, rev int) {
	u, err := s.jcSuite.State.Unit(unitName)
	c.Assert(err, gc.IsNil)
	svc, err := u.Service()
	c.Assert(err, gc.IsNil)
	curl := charm.MustParseURL(fmt.Sprintf("cs:quantal/%s-%d", svc.Name(), rev))
	err = u.SetCharmURL(curl)
	c.Assert(err, gc.IsNil)
}

// SetupScenario adds some machines and services to state.
// It assumes a state server machine has already been created.
func (s *CharmSuite) SetupScenario(c *gc.C) {
	s.AddMachine(c, "1", state.JobHostUnits)
	s.AddMachine(c, "2", state.JobHostUnits)
	s.AddMachine(c, "3", state.JobHostUnits)

	// mysql is out of date
	s.AddCharmWithRevision(c, "mysql", 22)
	s.AddService(c, "mysql", "mysql", nil, nil)
	s.AddUnit(c, "mysql", "1")

	// wordpress is up to date
	s.AddCharmWithRevision(c, "wordpress", 26)
	s.AddService(c, "wordpress", "wordpress", nil, nil)
	s.AddUnit(c, "wordpress", "2")
	s.AddUnit(c, "wordpress", "2")
	// wordpress/0 has a version, wordpress/1 is unknown
	s.SetUnitRevision(c, "wordpress/0", 26)

	// varnish is a charm that does not have a version in the mock store.
	s.AddCharmWithRevision(c, "varnish", 5)
	s.AddService(c, "varnish", "varnish", nil, nil)
	s.AddUnit(c, "varnish", "3")
}

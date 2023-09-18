// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// This used to live at
// apiserver/facades/controller/charmrevisionupdater/testing/suite.go
// but we moved it here as it's a JujuConnSuite test only used by this
// package's tests.

package testing

import (
	"fmt"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testcharms"
)

// CharmSuite provides infrastructure to set up and perform tests associated
// with charm versioning. A testing charm store server is created and populated
// with some known charms used for testing.
type CharmSuite struct {
	jcSuite *jujutesting.JujuConnSuite

	charms map[string]*state.Charm
}

func (s *CharmSuite) SetUpSuite(c *gc.C, jcSuite *jujutesting.JujuConnSuite) {
	s.jcSuite = jcSuite
}

func (s *CharmSuite) SetUpTest(c *gc.C) {
	s.charms = make(map[string]*state.Charm)
}

// AddMachine adds a new machine to state.
func (s *CharmSuite) AddMachine(c *gc.C, machineId string, job state.MachineJob) {
	m, err := s.jcSuite.State.AddOneMachine(state.MachineTemplate{
		Base: state.UbuntuBase("12.10"),
		Jobs: []state.MachineJob{job},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m.Id(), gc.Equals, machineId)
	cons, err := m.Constraints()
	c.Assert(err, jc.ErrorIsNil)
	controllerCfg, err := s.jcSuite.State.ControllerConfig()
	c.Assert(err, jc.ErrorIsNil)
	inst, hc := jujutesting.AssertStartInstanceWithConstraints(c, s.jcSuite.Environ, s.jcSuite.ProviderCallContext, controllerCfg.ControllerUUID(), m.Id(), cons)
	err = m.SetProvisioned(inst.Id(), "", "fake_nonce", hc)
	c.Assert(err, jc.ErrorIsNil)
}

// AddCharmhubCharmWithRevision adds a charmhub charm with the specified revision to state.
func (s *CharmSuite) AddCharmhubCharmWithRevision(c *gc.C, charmName string, rev int) *state.Charm {
	ch := testcharms.Hub.CharmDir(charmName)
	name := ch.Meta().Name
	curl := fmt.Sprintf("ch:amd64/jammy/%s-%d", name, rev)
	info := state.CharmInfo{
		Charm:       ch,
		ID:          curl,
		StoragePath: "dummy-path",
		SHA256:      fmt.Sprintf("%s-%d-sha256", name, rev),
	}
	dummy, err := s.jcSuite.State.AddCharm(info)
	c.Assert(err, jc.ErrorIsNil)
	s.charms[name] = dummy
	return dummy
}

// AddApplication adds an application for the specified charm to state.
func (s *CharmSuite) AddApplication(c *gc.C, charmName, applicationName string) {
	ch, ok := s.charms[charmName]
	c.Assert(ok, jc.IsTrue)
	revision := ch.Revision()
	_, err := s.jcSuite.State.AddApplication(state.AddApplicationArgs{
		Name:  applicationName,
		Charm: ch,
		CharmOrigin: &state.CharmOrigin{
			ID:     "mycharmhubid",
			Hash:   "mycharmhash",
			Source: "charm-hub",
			Platform: &state.Platform{
				Architecture: "amd64",
				OS:           "ubuntu",
				Channel:      "12.10/stable",
			},
			Revision: &revision,
			Channel: &state.Channel{
				Track: "latest",
				Risk:  "stable",
			},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
}

// AddUnit adds a new unit for application to the specified machine.
func (s *CharmSuite) AddUnit(c *gc.C, appName, machineId string) {
	svc, err := s.jcSuite.State.Application(appName)
	c.Assert(err, jc.ErrorIsNil)
	u, err := svc.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	m, err := s.jcSuite.State.Machine(machineId)
	c.Assert(err, jc.ErrorIsNil)
	err = u.AssignToMachine(m)
	c.Assert(err, jc.ErrorIsNil)
}

// SetUnitRevision sets the unit's charm to the specified revision.
func (s *CharmSuite) SetUnitRevision(c *gc.C, unitName string, rev int) {
	u, err := s.jcSuite.State.Unit(unitName)
	c.Assert(err, jc.ErrorIsNil)
	svc, err := u.Application()
	c.Assert(err, jc.ErrorIsNil)
	curl := fmt.Sprintf("ch:amd64/jammy/%s-%d", svc.Name(), rev)
	err = u.SetCharmURL(curl)
	c.Assert(err, jc.ErrorIsNil)
}

// SetupScenario adds some machines and applications to state.
// It assumes a controller machine has already been created.
func (s *CharmSuite) SetupScenario(c *gc.C) {
	s.AddMachine(c, "1", state.JobHostUnits)
	s.AddMachine(c, "2", state.JobHostUnits)
	s.AddMachine(c, "3", state.JobHostUnits)

	// mysql is out of date
	s.AddCharmhubCharmWithRevision(c, "mysql", 22)
	s.AddApplication(c, "mysql", "mysql")
	s.AddUnit(c, "mysql", "1")

	// wordpress is up to date
	s.AddCharmhubCharmWithRevision(c, "wordpress", 26)
	s.AddApplication(c, "wordpress", "wordpress")
	s.AddUnit(c, "wordpress", "2")
	s.AddUnit(c, "wordpress", "2")
	// wordpress/0 has a version, wordpress/1 is unknown
	s.SetUnitRevision(c, "wordpress/0", 26)

	// varnish is a charm that does not have a version in the mock store.
	s.AddCharmhubCharmWithRevision(c, "varnish", 5)
	s.AddApplication(c, "varnish", "varnish")
	s.AddUnit(c, "varnish", "3")
}

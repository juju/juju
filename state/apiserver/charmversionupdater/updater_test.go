// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmversionupdater_test

import (
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/apiserver/charmversionupdater"
	"launchpad.net/juju-core/state/apiserver/charmversionupdater/testing"
	"launchpad.net/juju-core/state/apiserver/common"
	apiservertesting "launchpad.net/juju-core/state/apiserver/testing"
)

type charmVersionSuite struct {
	testing.CharmSuite

	charmversionupdater *charmversionupdater.CharmVersionUpdaterAPI
	resources           *common.Resources
	authoriser          apiservertesting.FakeAuthorizer
}

var _ = gc.Suite(&charmVersionSuite{})

func (s *charmVersionSuite) SetUpSuite(c *gc.C) {
	s.CharmSuite.SetUpSuite(c)
}

func (s *charmVersionSuite) SetUpTest(c *gc.C) {
	s.CharmSuite.SetUpTest(c)
	s.resources = common.NewResources()
	s.AddCleanup(func(_ *gc.C) { s.resources.StopAll() })
	s.authoriser = apiservertesting.FakeAuthorizer{
		LoggedIn:     true,
		StateManager: true,
	}
	var err error
	s.charmversionupdater, err = charmversionupdater.NewCharmVersionUpdaterAPI(s.State, s.resources, s.authoriser)
	c.Assert(err, gc.IsNil)
}

func (s *charmVersionSuite) TestNewCharmVersionUpdaterAPIAcceptsStateManager(c *gc.C) {
	endPoint, err := charmversionupdater.NewCharmVersionUpdaterAPI(s.State, s.resources, s.authoriser)
	c.Assert(err, gc.IsNil)
	c.Assert(endPoint, gc.NotNil)
}

func (s *charmVersionSuite) TestNewCharmVersionUpdaterAPIRefusesNonStateManager(c *gc.C) {
	anAuthoriser := s.authoriser
	anAuthoriser.StateManager = false
	endPoint, err := charmversionupdater.NewCharmVersionUpdaterAPI(s.State, s.resources, anAuthoriser)
	c.Assert(endPoint, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *charmVersionSuite) TestUpdateVersions(c *gc.C) {
	s.AddMachine(c, "0", state.JobManageEnviron)
	s.SetupScenario(c)
	result, err := s.charmversionupdater.UpdateVersions()
	c.Assert(err, gc.IsNil)
	c.Assert(result.Error, gc.IsNil)

	svc, err := s.State.Service("mysql")
	c.Assert(err, gc.IsNil)
	c.Assert(svc.RevisionStatus(), gc.Equals, "out of date (available: 23)")
	u, err := s.State.Unit("mysql/0")
	c.Assert(err, gc.IsNil)
	c.Assert(u.RevisionStatus(), gc.Equals, "unknown")

	svc, err = s.State.Service("wordpress")
	c.Assert(err, gc.IsNil)
	c.Assert(svc.RevisionStatus(), gc.Equals, "")
	u, err = s.State.Unit("wordpress/0")
	c.Assert(err, gc.IsNil)
	c.Assert(u.RevisionStatus(), gc.Equals, "")
	u, err = s.State.Unit("wordpress/1")
	c.Assert(err, gc.IsNil)
	c.Assert(u.RevisionStatus(), gc.Equals, "unknown")

	svc, err = s.State.Service("varnish")
	c.Assert(err, gc.IsNil)
	c.Assert(svc.RevisionStatus(), gc.Equals, "unknown: charm not found: cs:quantal/varnish")
	u, err = s.State.Unit("varnish/0")
	c.Assert(err, gc.IsNil)
	c.Assert(u.RevisionStatus(), gc.Equals, "")

	// Update mysql version and run update again.
	svc, err = s.State.Service("mysql")
	c.Assert(err, gc.IsNil)
	ch := s.AddCharmWithRevision(c, "mysql", 23)
	err = svc.SetCharm(ch, true)
	c.Assert(err, gc.IsNil)

	result, err = s.charmversionupdater.UpdateVersions()
	c.Assert(err, gc.IsNil)
	c.Assert(result.Error, gc.IsNil)

	// mysql is now up to date, wordpress, varnish have not changed.
	svc, err = s.State.Service("mysql")
	c.Assert(err, gc.IsNil)
	c.Assert(svc.RevisionStatus(), gc.Equals, "")
	svc, err = s.State.Service("wordpress")
	c.Assert(err, gc.IsNil)
	c.Assert(svc.RevisionStatus(), gc.Equals, "")
	svc, err = s.State.Service("varnish")
	c.Assert(err, gc.IsNil)
	c.Assert(svc.RevisionStatus(), gc.Equals, "unknown: charm not found: cs:quantal/varnish")
}

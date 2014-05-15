// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmrevisionupdater_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/charm"
	jujutesting "launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/apiserver/charmrevisionupdater"
	"launchpad.net/juju-core/state/apiserver/charmrevisionupdater/testing"
	"launchpad.net/juju-core/state/apiserver/common"
	apiservertesting "launchpad.net/juju-core/state/apiserver/testing"
)

type charmVersionSuite struct {
	testing.CharmSuite
	jujutesting.JujuConnSuite

	charmrevisionupdater *charmrevisionupdater.CharmRevisionUpdaterAPI
	resources            *common.Resources
	authoriser           apiservertesting.FakeAuthorizer
}

var _ = gc.Suite(&charmVersionSuite{})

func (s *charmVersionSuite) SetUpSuite(c *gc.C) {
	s.JujuConnSuite.SetUpSuite(c)
	s.CharmSuite.SetUpSuite(c, &s.JujuConnSuite)
}

func (s *charmVersionSuite) TearDownSuite(c *gc.C) {
	s.CharmSuite.TearDownSuite(c)
	s.JujuConnSuite.TearDownSuite(c)
}

func (s *charmVersionSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.CharmSuite.SetUpTest(c)
	s.resources = common.NewResources()
	s.AddCleanup(func(_ *gc.C) { s.resources.StopAll() })
	s.authoriser = apiservertesting.FakeAuthorizer{
		LoggedIn:       true,
		EnvironManager: true,
	}
	var err error
	s.charmrevisionupdater, err = charmrevisionupdater.NewCharmRevisionUpdaterAPI(s.State, s.resources, s.authoriser)
	c.Assert(err, gc.IsNil)
}

func (s *charmVersionSuite) TearDownTest(c *gc.C) {
	s.CharmSuite.TearDownTest(c)
	s.JujuConnSuite.TearDownTest(c)
}

func (s *charmVersionSuite) TestNewCharmRevisionUpdaterAPIAcceptsStateManager(c *gc.C) {
	endPoint, err := charmrevisionupdater.NewCharmRevisionUpdaterAPI(s.State, s.resources, s.authoriser)
	c.Assert(err, gc.IsNil)
	c.Assert(endPoint, gc.NotNil)
}

func (s *charmVersionSuite) TestNewCharmRevisionUpdaterAPIRefusesNonStateManager(c *gc.C) {
	anAuthoriser := s.authoriser
	anAuthoriser.EnvironManager = false
	endPoint, err := charmrevisionupdater.NewCharmRevisionUpdaterAPI(s.State, s.resources, anAuthoriser)
	c.Assert(endPoint, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *charmVersionSuite) TestUpdateRevisions(c *gc.C) {
	s.AddMachine(c, "0", state.JobManageEnviron)
	s.SetupScenario(c)

	curl := charm.MustParseURL("cs:quantal/mysql")
	_, err := s.State.LatestPlaceholderCharm(curl)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)

	curl = charm.MustParseURL("cs:quantal/wordpress")
	_, err = s.State.LatestPlaceholderCharm(curl)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)

	result, err := s.charmrevisionupdater.UpdateLatestRevisions()
	c.Assert(err, gc.IsNil)
	c.Assert(result.Error, gc.IsNil)

	curl = charm.MustParseURL("cs:quantal/mysql")
	pending, err := s.State.LatestPlaceholderCharm(curl)
	c.Assert(err, gc.IsNil)
	c.Assert(pending.String(), gc.Equals, "cs:quantal/mysql-23")

	// Latest wordpress is already deployed, so no pending charm.
	curl = charm.MustParseURL("cs:quantal/wordpress")
	_, err = s.State.LatestPlaceholderCharm(curl)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)

	// Varnish has an error when updating, so no pending charm.
	curl = charm.MustParseURL("cs:quantal/varnish")
	_, err = s.State.LatestPlaceholderCharm(curl)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)

	// Update mysql version and run update again.
	svc, err := s.State.Service("mysql")
	c.Assert(err, gc.IsNil)
	ch := s.AddCharmWithRevision(c, "mysql", 23)
	err = svc.SetCharm(ch, true)
	c.Assert(err, gc.IsNil)

	result, err = s.charmrevisionupdater.UpdateLatestRevisions()
	c.Assert(err, gc.IsNil)
	c.Assert(result.Error, gc.IsNil)

	// Latest mysql is now deployed, so no pending charm.
	curl = charm.MustParseURL("cs:quantal/mysql")
	_, err = s.State.LatestPlaceholderCharm(curl)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *charmVersionSuite) TestEnvironmentUUIDUsed(c *gc.C) {
	s.AddMachine(c, "0", state.JobManageEnviron)
	s.SetupScenario(c)
	result, err := s.charmrevisionupdater.UpdateLatestRevisions()
	c.Assert(err, gc.IsNil)
	c.Assert(result.Error, gc.IsNil)

	env, err := s.State.Environment()
	c.Assert(err, gc.IsNil)
	c.Assert(s.Server.Metadata, gc.DeepEquals, []string{"environment_uuid=" + env.UUID()})
}

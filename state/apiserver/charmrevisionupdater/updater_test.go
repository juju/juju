// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmrevisionupdater_test

import (
	"fmt"
	"strings"

	gc "launchpad.net/gocheck"
	"launchpad.net/loggo"

	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/apiserver/charmrevisionupdater"
	"launchpad.net/juju-core/state/apiserver/charmrevisionupdater/testing"
	"launchpad.net/juju-core/state/apiserver/common"
	apiservertesting "launchpad.net/juju-core/state/apiserver/testing"
	jc "launchpad.net/juju-core/testing/checkers"
)

type charmVersionSuite struct {
	testing.CharmSuite

	charmrevisionupdater *charmrevisionupdater.CharmRevisionUpdaterAPI
	resources            *common.Resources
	authoriser           apiservertesting.FakeAuthorizer
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
	s.charmrevisionupdater, err = charmrevisionupdater.NewCharmRevisionUpdaterAPI(s.State, s.resources, s.authoriser)
	c.Assert(err, gc.IsNil)
}

func (s *charmVersionSuite) TestNewCharmRevisionUpdaterAPIAcceptsStateManager(c *gc.C) {
	endPoint, err := charmrevisionupdater.NewCharmRevisionUpdaterAPI(s.State, s.resources, s.authoriser)
	c.Assert(err, gc.IsNil)
	c.Assert(endPoint, gc.NotNil)
}

func (s *charmVersionSuite) TestNewCharmRevisionUpdaterAPIRefusesNonStateManager(c *gc.C) {
	anAuthoriser := s.authoriser
	anAuthoriser.StateManager = false
	endPoint, err := charmrevisionupdater.NewCharmRevisionUpdaterAPI(s.State, s.resources, anAuthoriser)
	c.Assert(endPoint, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *charmVersionSuite) TestUpdateRevisions(c *gc.C) {
	s.AddMachine(c, "0", state.JobManageEnviron)
	s.SetupScenario(c)

	curl := charm.MustParseURL("cs:quantal/mysql")
	_, err := s.State.LatestPendingCharm(curl)
	c.Assert(err, jc.Satisfies, errors.IsNotFoundError)

	curl = charm.MustParseURL("cs:quantal/wordpress")
	_, err = s.State.LatestPendingCharm(curl)
	c.Assert(err, jc.Satisfies, errors.IsNotFoundError)

	result, err := s.charmrevisionupdater.UpdateLatestRevisions()
	c.Assert(err, gc.IsNil)
	c.Assert(result.Error, gc.IsNil)

	curl = charm.MustParseURL("cs:quantal/mysql")
	pending, err := s.State.LatestPendingCharm(curl)
	c.Assert(err, gc.IsNil)
	c.Assert(pending.String(), gc.Equals, "cs:quantal/mysql-23")

	// Latest wordpress is already deployed, so no pending charm.
	curl = charm.MustParseURL("cs:quantal/wordpress")
	_, err = s.State.LatestPendingCharm(curl)
	c.Assert(err, jc.Satisfies, errors.IsNotFoundError)

	// Varnish has an error when updating, so no pending charm.
	curl = charm.MustParseURL("cs:quantal/varnish")
	_, err = s.State.LatestPendingCharm(curl)
	c.Assert(err, jc.Satisfies, errors.IsNotFoundError)

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
	_, err = s.State.LatestPendingCharm(curl)
	c.Assert(err, jc.Satisfies, errors.IsNotFoundError)
}

func (s *charmVersionSuite) TestEnvironmentUUIDUsed(c *gc.C) {
	// There's no easy way to check that the environment uuid is used, apart from
	// inspecting the log messages produced by the mock store. But at least it is
	// tested - the auth headers which are implemented similarly are not tested AFAICS'
	tw := &loggo.TestWriter{}
	c.Assert(loggo.RegisterWriter("version-update-tester", tw, loggo.DEBUG), gc.IsNil)
	defer func() {
		loggo.RemoveWriter("version-update-tester")
	}()

	s.AddMachine(c, "0", state.JobManageEnviron)
	s.SetupScenario(c)
	result, err := s.charmversionupdater.UpdateVersions()
	c.Assert(err, gc.IsNil)
	c.Assert(result.Error, gc.IsNil)

	messageFound := false
	env, err := s.State.Environment()
	c.Assert(err, gc.IsNil)
	expectedMessageSnippet := fmt.Sprintf("Juju metadata: environment_uuid=%s", env.UUID())
	for _, log := range tw.Log {
		if messageFound = strings.Contains(log.Message, expectedMessageSnippet); messageFound {
			break
		}
	}
	c.Assert(messageFound, jc.IsTrue)
}

// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmrevisionupdater_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	//"gopkg.in/juju/charm.v6"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facades/controller/charmrevisionupdater"
	"github.com/juju/juju/apiserver/facades/controller/charmrevisionupdater/testing"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
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
		Controller: true,
		Tag:        names.NewMachineTag("99"),
	}
	var err error
	s.charmrevisionupdater, err = charmrevisionupdater.NewCharmRevisionUpdaterAPI(s.State, s.resources, s.authoriser)
	c.Assert(err, jc.ErrorIsNil)

	s.AddMachine(c, "0", state.JobManageModel)
}

func (s *charmVersionSuite) TearDownTest(c *gc.C) {
	s.CharmSuite.TearDownTest(c)
	s.JujuConnSuite.TearDownTest(c)
}

func (s *charmVersionSuite) TestNewCharmRevisionUpdaterAPIAcceptsStateManager(c *gc.C) {
	// remove? duplicates what happens during setup
	endPoint, err := charmrevisionupdater.NewCharmRevisionUpdaterAPI(s.State, s.resources, s.authoriser)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(endPoint, gc.NotNil)
}

func (s *charmVersionSuite) TestNewCharmRevisionUpdaterAPIRefusesNonStateManager(c *gc.C) {
	anAuthoriser := s.authoriser
	anAuthoriser.Controller = false
	endPoint, err := charmrevisionupdater.NewCharmRevisionUpdaterAPI(s.State, s.resources, anAuthoriser)
	c.Assert(endPoint, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

// func (s *charmVersionSuite) TestStateSupportsUpdatingRevisions(c *gc.C) {
// 	s.AddMachine(c, "0", state.JobManageModel)
// 	s.SetupScenario(c)

// 	charmUrl := "cs:quantal/mysql"
// 	charmId := charm.MustParseURL(charmUrl)

// 	_, err := s.State.LatestPlaceholderCharm(charmId)
// 	c.Assert(err, jc.Satisfies, errors.IsNotFound)

// 	err = s.State.AddStoreCharmPlaceholder(charmId)
// 	c.Assert(err, jc.ErrorIsNil)

// 	result, err := s.State.LatestPlaceholderCharm(charmId)
// 	c.Assert(err, jc.ErrorIsNil)
// 	c.Assert(result.String(), gc.Equals, charmUrl)
// }

func (s *charmVersionSuite) TestUpdateRevisionsWithNothingToUpdateDoesNotCauseError(c *gc.C) {
	result, err := s.charmrevisionupdater.UpdateLatestRevisions()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Error, gc.IsNil)
}

func (s *charmVersionSuite) TestUpdateRevisionsWithDeployedApplicationsDoesNotCauseError(c *gc.C) {
	s.SetupScenario(c)
	result, err := s.charmrevisionupdater.UpdateLatestRevisions()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Error, gc.IsNil)
}

// type brokenHandler struct{}

// func (h brokenHandler) HandleLatest(name names.ApplicationTag, charmInfo charmstore.CharmInfo) error {
// 	charmInfo.L
// 	return errors.New("not updated")
// }

// func (s *charmVersionSuite) TestUpdateRevisionsWithBrokenHandler(c *gc.C) {
// 	charmrevisionupdater.RegisterLatestCharmHandler("mysql", func(*state.State) (charmrevisionupdater.LatestCharmHandler, error) {
// 		return brokenHandler{}, nil
// 	})

// 	result, err := s.charmrevisionupdater.UpdateLatestRevisions()
// 	c.Assert(err, jc.ErrorIsNil)
// 	c.Assert(result.Error, gc.IsNil)
// }

// func (s *charmVersionSuite) TestUpdateRevisions(c *gc.C) {
// 	charmUrl := "cs:quantal/mysql"
// 	charmId := charm.MustParseURL(charmUrl)
// 	pending, err := s.State.LatestPlaceholderCharm(charmId)
// 	c.Assert(err, jc.ErrorIsNil)
// 	c.Assert(pending.String(), gc.Equals, charmUrl)

// 	result, err := s.charmrevisionupdater.UpdateLatestRevisions()
// 	c.Assert(err, jc.ErrorIsNil)
// 	c.Assert(result.Error, gc.IsNil)

// 	// Update mysql version and run update again.
// 	app, err := s.State.Application("mysql")
// 	c.Assert(err, jc.ErrorIsNil)
// 	ch := s.AddCharmWithRevision(c, "mysql", 23)
// 	cfg := state.SetCharmConfig{
// 		Charm:      ch,
// 		ForceUnits: true,
// 	}
// 	err = app.SetCharm(cfg)
// 	c.Assert(err, jc.ErrorIsNil)

// 	result, err = s.charmrevisionupdater.UpdateLatestRevisions()
// 	c.Assert(err, jc.ErrorIsNil)
// 	c.Assert(result.Error, gc.IsNil)

// 	// Latest mysql is now deployed, so no pending charm.
// 	curl := charm.MustParseURL("cs:quantal/mysql")
// 	_, err = s.State.LatestPlaceholderCharm(curl)
// 	c.Assert(err, jc.Satisfies, errors.IsNotFound)
// }

// 	s.PatchValue(&charmrevisionupdater.NewCharmStoreClient, func(st *state.State) (charmstore.Client, error) {
// 		minimal := apiservertesting.NewCharmstoreClient()
// 		client := charmstore.Client{minimal}
// 		return client, nil
// 	})

// 	s.AddMachine(c, "0", state.JobManageModel)
// 	s.SetupScenario(c)

// 	curl := charm.MustParseURL("cs:quantal/mysql")
// 	_, err := s.State.LatestPlaceholderCharm(curl)
// 	c.Assert(err, jc.Satisfies, errors.IsNotFound)

// 	curl = charm.MustParseURL("cs:quantal/wordpress")
// 	_, err = s.State.LatestPlaceholderCharm(curl)
// 	c.Assert(err, jc.Satisfies, errors.IsNotFound)

// 	result, err := s.charmrevisionupdater.UpdateLatestRevisions()
// 	c.Assert(err, jc.ErrorIsNil)
// 	c.Assert(result.Error, gc.IsNil)

// 	curl = charm.MustParseURL("cs:quantal/mysql")
// 	pending, err := s.State.LatestPlaceholderCharm(curl)
// 	c.Assert(err, jc.ErrorIsNil)
// 	c.Assert(pending.String(), gc.Equals, "cs:quantal/mysql-23")

// 	// Latest wordpress is already deployed, so no pending charm.
// 	curl = charm.MustParseURL("cs:quantal/wordpress")
// 	_, err = s.State.LatestPlaceholderCharm(curl)
// 	c.Assert(err, jc.Satisfies, errors.IsNotFound)

// 	// Varnish has an error when updating, so no pending charm.
// 	curl = charm.MustParseURL("cs:quantal/varnish")
// 	_, err = s.State.LatestPlaceholderCharm(curl)
// 	c.Assert(err, jc.Satisfies, errors.IsNotFound)

// 	// Update mysql version and run update again.
// 	app, err := s.State.Application("mysql")
// 	c.Assert(err, jc.ErrorIsNil)
// 	ch := s.AddCharmWithRevision(c, "mysql", 23)
// 	cfg := state.SetCharmConfig{
// 		Charm:      ch,
// 		ForceUnits: true,
// 	}
// 	err = app.SetCharm(cfg)
// 	c.Assert(err, jc.ErrorIsNil)

// 	result, err = s.charmrevisionupdater.UpdateLatestRevisions()
// 	c.Assert(err, jc.ErrorIsNil)
// 	c.Assert(result.Error, gc.IsNil)

// 	// Latest mysql is now deployed, so no pending charm.
// 	curl = charm.MustParseURL("cs:quantal/mysql")
// 	_, err = s.State.LatestPlaceholderCharm(curl)
// 	c.Assert(err, jc.Satisfies, errors.IsNotFound)
// }

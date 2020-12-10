// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmrevisionupdater_test

import (
	"github.com/golang/mock/gomock"
	"github.com/juju/charm/v8"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/facades/controller/charmrevisionupdater"
	apiservertesting "github.com/juju/juju/apiserver/testing"
)

type updaterSuite struct{}

var _ = gc.Suite(&updaterSuite{})

func (s *updaterSuite) TestNewAuthSuccess(c *gc.C) {
	authoriser := apiservertesting.FakeAuthorizer{Controller: true}
	facadeCtx := facadeContextShim{state: nil, authorizer: authoriser}
	updater, err := charmrevisionupdater.NewCharmRevisionUpdaterAPI(facadeCtx)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(updater, gc.NotNil)
}

func (s *updaterSuite) TestNewAuthFailure(c *gc.C) {
	authoriser := apiservertesting.FakeAuthorizer{Controller: false}
	facadeCtx := facadeContextShim{state: nil, authorizer: authoriser}
	updater, err := charmrevisionupdater.NewCharmRevisionUpdaterAPI(facadeCtx)
	c.Assert(updater, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *updaterSuite) TestCharmhubUpdate(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	state := makeState(c, ctrl, nil)

	state.EXPECT().AllApplications().Return([]charmrevisionupdater.Application{
		makeApplication(ctrl, "ch", "mysql", "charm-1", "app-1", 22),
		makeApplication(ctrl, "ch", "postgresql", "charm-2", "app-2", 41),
	}, nil).AnyTimes()

	state.EXPECT().AddCharmPlaceholder(charm.MustParseURL("ch:mysql-23")).Return(nil)
	state.EXPECT().AddCharmPlaceholder(charm.MustParseURL("ch:postgresql-42")).Return(nil)

	updater, err := charmrevisionupdater.NewCharmRevisionUpdaterAPIState(state, nil, newFakeCharmhubClient)
	c.Assert(err, jc.ErrorIsNil)

	result, err := updater.UpdateLatestRevisions()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Error, gc.IsNil)
}

func (s *updaterSuite) TestCharmhubNoUpdate(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	state := makeState(c, ctrl, nil)

	state.EXPECT().AllApplications().Return([]charmrevisionupdater.Application{
		makeApplication(ctrl, "ch", "postgresql", "charm-2", "app-2", 42),
	}, nil).AnyTimes()

	state.EXPECT().AddCharmPlaceholder(charm.MustParseURL("ch:postgresql-42")).Return(nil)

	updater, err := charmrevisionupdater.NewCharmRevisionUpdaterAPIState(state, nil, newFakeCharmhubClient)
	c.Assert(err, jc.ErrorIsNil)

	result, err := updater.UpdateLatestRevisions()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Error, gc.IsNil)
}

func (s *updaterSuite) TestCharmNotInStore(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	state := makeState(c, ctrl, nil)

	state.EXPECT().AllApplications().Return([]charmrevisionupdater.Application{
		makeApplication(ctrl, "ch", "varnish", "charm-5", "app-1", 1),
		makeApplication(ctrl, "cs", "varnish", "charm-6", "app-2", 2),
	}, nil).AnyTimes()

	updater, err := charmrevisionupdater.NewCharmRevisionUpdaterAPIState(state, newFakeCharmstoreClient, newFakeCharmhubClient)
	c.Assert(err, jc.ErrorIsNil)

	result, err := updater.UpdateLatestRevisions()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Error, gc.IsNil)
}

func (s *updaterSuite) TestCharmstoreUpdate(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	state := makeState(c, ctrl, nil)

	state.EXPECT().AllApplications().Return([]charmrevisionupdater.Application{
		makeApplication(ctrl, "cs", "mysql", "charm-1", "app-1", 22),
		makeApplication(ctrl, "cs", "wordpress", "charm-2", "app-2", 26),
		makeApplication(ctrl, "cs", "varnish", "charm-3", "app-3", 5), // doesn't exist in store
	}, nil)

	state.EXPECT().AddCharmPlaceholder(charm.MustParseURL("cs:mysql-23")).Return(nil)
	state.EXPECT().AddCharmPlaceholder(charm.MustParseURL("cs:wordpress-26")).Return(nil)

	updater, err := charmrevisionupdater.NewCharmRevisionUpdaterAPIState(state, newFakeCharmstoreClient, nil)
	c.Assert(err, jc.ErrorIsNil)

	result, err := updater.UpdateLatestRevisions()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Error, gc.IsNil)

	// Update mysql version and run update again.
	state.EXPECT().AllApplications().Return([]charmrevisionupdater.Application{
		makeApplication(ctrl, "cs", "mysql", "charm-1", "app-1", 23),
	}, nil)

	state.EXPECT().AddCharmPlaceholder(charm.MustParseURL("cs:mysql-23")).Return(nil)

	result, err = updater.UpdateLatestRevisions()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Error, gc.IsNil)
}

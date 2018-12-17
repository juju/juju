// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradecharmprofile_test

import (
	"errors"

	"github.com/golang/mock/gomock"
	"github.com/juju/testing"
	gc "gopkg.in/check.v1"
	corecharm "gopkg.in/juju/charm.v6"

	"github.com/juju/juju/core/lxdprofile"
	"github.com/juju/juju/worker/uniter/operation/mocks"
	"github.com/juju/juju/worker/uniter/remotestate"
	"github.com/juju/juju/worker/uniter/resolver"
	"github.com/juju/juju/worker/uniter/upgradecharmprofile"
)

type ResolverSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&ResolverSuite{})

func (ResolverSuite) TestNextOpWithNoRemoveStatus(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockFactory := mocks.NewMockFactory(ctrl)

	res := upgradecharmprofile.NewResolver()
	_, err := res.NextOp(resolver.LocalState{}, remotestate.Snapshot{}, mockFactory)
	c.Assert(err, gc.Equals, resolver.ErrDoNotProceed)
}

func (ResolverSuite) TestNextOpWithNotRequiredStatus(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockFactory := mocks.NewMockFactory(ctrl)

	res := upgradecharmprofile.NewResolver()
	_, err := res.NextOp(resolver.LocalState{}, remotestate.Snapshot{
		UpgradeCharmProfileStatus: lxdprofile.NotRequiredStatus,
	}, mockFactory)
	c.Assert(err, gc.Equals, resolver.ErrNoOperation)
}

func (ResolverSuite) TestNextOpWithSuccessStatus(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockFactory := mocks.NewMockFactory(ctrl)

	res := upgradecharmprofile.NewResolver()
	_, err := res.NextOp(resolver.LocalState{}, remotestate.Snapshot{
		UpgradeCharmProfileStatus: lxdprofile.SuccessStatus,
	}, mockFactory)
	c.Assert(err, gc.Equals, resolver.ErrNoOperation)
}

func (ResolverSuite) TestNextOpWithErrorStatus(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	charmURL, err := corecharm.ParseURL("cs:~user/series/name")
	c.Assert(err, gc.IsNil)

	mockOp := mocks.NewMockOperation(ctrl)

	mockFactory := mocks.NewMockFactory(ctrl)
	mockFactory.EXPECT().NewFinishUpgradeCharmProfile(charmURL).Return(mockOp, nil)

	res := upgradecharmprofile.NewResolver()
	op, err := res.NextOp(resolver.LocalState{}, remotestate.Snapshot{
		CharmURL:                  charmURL,
		UpgradeCharmProfileStatus: lxdprofile.AnnotateErrorStatus(errors.New("foo bar")),
	}, mockFactory)
	c.Assert(err, gc.IsNil)
	c.Assert(op, gc.NotNil)
}

func (ResolverSuite) TestNextOpWithErrorStatusWithALocalState(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	charmURL, err := corecharm.ParseURL("cs:~user/series/name")
	c.Assert(err, gc.IsNil)

	mockFactory := mocks.NewMockFactory(ctrl)

	res := upgradecharmprofile.NewResolver()
	_, err = res.NextOp(resolver.LocalState{
		UpgradeCharmProfileStatus: lxdprofile.NotKnownStatus,
	}, remotestate.Snapshot{
		CharmURL:                  charmURL,
		UpgradeCharmProfileStatus: lxdprofile.AnnotateErrorStatus(errors.New("foo bar")),
	}, mockFactory)
	c.Assert(err, gc.Equals, resolver.ErrNoOperation)
}

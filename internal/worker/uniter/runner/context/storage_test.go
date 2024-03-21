// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context_test

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/rpc/params"
	coretesting "github.com/juju/juju/testing"
)

type StorageSuite struct {
	BaseHookContextSuite
}

var _ = gc.Suite(&StorageSuite{})

func (s *StorageSuite) TestAddUnitStorage(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	count := uint64(1)
	s.assertUnitStorageAdded(c, ctrl,
		map[string]params.StorageDirectives{
			"allecto": {Count: &count}})
}

func (s *StorageSuite) TestAddUnitStorageAccumulated(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	count := uint64(1)
	s.assertUnitStorageAdded(c, ctrl,
		map[string]params.StorageDirectives{
			"multi2up": {Count: &count}},
		map[string]params.StorageDirectives{
			"multi1to10": {Count: &count}})
}

func (s *StorageSuite) assertUnitStorageAdded(c *gc.C, ctrl *gomock.Controller, cons ...map[string]params.StorageDirectives) {
	// Get the context.
	ctx := s.getHookContext(c, ctrl, coretesting.ModelTag.Id(), -1, "", names.StorageTag{})
	c.Assert(ctx.UnitName(), gc.Equals, s.unit.Name())

	arg := params.CommitHookChangesArg{
		Tag: s.unit.Tag().String(),
	}
	for _, one := range cons {
		for storage, scons := range one {
			arg.AddStorage = append(arg.AddStorage, params.StorageAddParams{
				UnitTag:     s.unit.Tag().String(),
				StorageName: storage,
				Directives:  scons,
			})
		}
		err := ctx.AddUnitStorage(one)
		c.Check(err, jc.ErrorIsNil)
	}

	s.unit.EXPECT().CommitHookChanges(hookCommitMatcher{c, params.CommitHookChangesArgs{
		Args: []params.CommitHookChangesArg{arg},
	}}).Return(nil)

	// Flush the context with a success.
	err := ctx.Flush(context.Background(), "success", nil)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *StorageSuite) TestRunHookAddStorageOnFailure(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	ctx := s.getHookContext(c, ctrl, coretesting.ModelTag.Id(), -1, "", names.StorageTag{})
	c.Assert(ctx.UnitName(), gc.Equals, s.unit.Name())

	size := uint64(1)
	err := ctx.AddUnitStorage(
		map[string]params.StorageDirectives{
			"allecto": {Size: &size},
		})
	c.Assert(err, jc.ErrorIsNil)

	// Flush the context with an error.
	msg := "test fail run hook"
	err = ctx.Flush(context.Background(), "test fail run hook", errors.New(msg))
	c.Assert(errors.Cause(err), gc.ErrorMatches, msg)
}

// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context_test

import (
	stdtesting "testing"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
)

type StorageSuite struct {
	BaseHookContextSuite
}

func TestStorageSuite(t *stdtesting.T) { tc.Run(t, &StorageSuite{}) }
func (s *StorageSuite) TestAddUnitStorage(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	count := uint64(1)
	s.assertUnitStorageAdded(c, ctrl,
		map[string]params.StorageDirectives{
			"allecto": {Count: &count}})
}

func (s *StorageSuite) TestAddUnitStorageAccumulated(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	count := uint64(1)
	s.assertUnitStorageAdded(c, ctrl,
		map[string]params.StorageDirectives{
			"multi2up": {Count: &count}},
		map[string]params.StorageDirectives{
			"multi1to10": {Count: &count}})
}

func (s *StorageSuite) assertUnitStorageAdded(c *tc.C, ctrl *gomock.Controller, cons ...map[string]params.StorageDirectives) {
	// Get the context.
	ctx := s.getHookContext(c, ctrl, coretesting.ModelTag.Id(), -1, "", names.StorageTag{})
	c.Assert(ctx.UnitName(), tc.Equals, s.unit.Name())

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
		c.Check(err, tc.ErrorIsNil)
	}

	s.unit.EXPECT().CommitHookChanges(gomock.Any(), hookCommitMatcher{c: c, expected: params.CommitHookChangesArgs{
		Args: []params.CommitHookChangesArg{arg},
	}}).Return(nil)

	// Flush the context with a success.
	err := ctx.Flush(c.Context(), "success", nil)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *StorageSuite) TestRunHookAddStorageOnFailure(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	ctx := s.getHookContext(c, ctrl, coretesting.ModelTag.Id(), -1, "", names.StorageTag{})
	c.Assert(ctx.UnitName(), tc.Equals, s.unit.Name())

	size := uint64(1)
	err := ctx.AddUnitStorage(
		map[string]params.StorageDirectives{
			"allecto": {Size: &size},
		})
	c.Assert(err, tc.ErrorIsNil)

	// Flush the context with an error.
	msg := "test fail run hook"
	err = ctx.Flush(c.Context(), "test fail run hook", errors.New(msg))
	c.Assert(errors.Cause(err), tc.ErrorMatches, msg)
}

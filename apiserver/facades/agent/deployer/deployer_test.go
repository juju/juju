// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package deployer

import (
	"context"
	stdtesting "testing"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	facademocks "github.com/juju/juju/apiserver/facade/mocks"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/watchertest"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/internal/testhelpers"
	internaltesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
)

type deployerSuite struct {
	testhelpers.IsolationSuite

	passwordService    *MockAgentPasswordService
	applicationService *MockApplicationService
	watcherRegistry    *facademocks.MockWatcherRegistry

	badTag names.Tag
}

func TestDeployerSuite(t *stdtesting.T) {
	tc.Run(t, &deployerSuite{})
}

func (s *deployerSuite) TestStub(c *tc.C) {
	c.Skip(`This suite is missing tests for the following scenarios:
	
 - Test deployer fails with non-machine agent user
 - Test life
 - Test remove
 - Test connection info
 - Test set status`)
}

func (s *deployerSuite) TestWatchUnitsPermissionDenied(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.badTag = names.NewMachineTag("0")
	c.Cleanup(func() { s.badTag = nil })

	api := &DeployerAPI{
		applicationService: s.applicationService,
		getCanWatch:        s.getCanWatch,
	}

	result, err := api.WatchUnits(c.Context(), params.Entities{
		Entities: []params.Entity{
			{
				Tag: names.NewMachineTag("0").String(),
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results[0].Error, tc.Satisfies, params.IsCodeUnauthorized)
}

func (s *deployerSuite) TestWatchUnitsMachineNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.applicationService.EXPECT().
		WatchUnitAddRemoveOnMachine(gomock.Any(), machine.Name("0")).
		Return(nil, applicationerrors.MachineNotFound)

	api := &DeployerAPI{
		applicationService: s.applicationService,
		getCanWatch:        s.getCanWatch,
	}

	result, err := api.WatchUnits(c.Context(), params.Entities{
		Entities: []params.Entity{
			{
				Tag: names.NewMachineTag("0").String(),
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results[0].Error, tc.Satisfies, params.IsCodeNotFound)
}

func (s *deployerSuite) TestWatchUnits(c *tc.C) {
	defer s.setupMocks(c).Finish()

	done := make(chan struct{})
	defer close(done)
	ch := make(chan []string)
	w := watchertest.NewMockStringsWatcher(ch)

	s.applicationService.EXPECT().WatchUnitAddRemoveOnMachine(gomock.Any(), machine.Name("0")).
		DoAndReturn(func(context.Context, machine.Name) (watcher.Watcher[[]string], error) {
			time.AfterFunc(internaltesting.ShortWait, func() {
				// Send initial event.
				select {
				case ch <- []string{"foo/0", "foo/1"}:
				case <-done:
					c.Error("watcher (unit) did not fire")
				}
			})
			return w, nil
		})
	s.watcherRegistry.EXPECT().Register(gomock.Any()).Return("87", nil)

	api := &DeployerAPI{
		applicationService: s.applicationService,
		getCanWatch:        s.getCanWatch,
		watcherRegistry:    s.watcherRegistry,
	}

	result, err := api.WatchUnits(c.Context(), params.Entities{
		Entities: []params.Entity{
			{Tag: names.NewMachineTag("0").String()},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.DeepEquals, params.StringsWatchResults{
		Results: []params.StringsWatchResult{
			{StringsWatcherId: "87", Changes: []string{"foo/0", "foo/1"}},
		},
	})
}

func (s *deployerSuite) TestSetUnitPassword(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.passwordService.EXPECT().
		SetUnitPassword(gomock.Any(), unit.Name("foo/1"), "password").
		Return(nil)

	api := &DeployerAPI{
		PasswordChanger: common.NewPasswordChanger(s.passwordService, nil, alwaysAllow),
	}

	result, err := api.SetPasswords(c.Context(), params.EntityPasswords{
		Changes: []params.EntityPassword{
			{
				Tag:      names.NewUnitTag("foo/1").String(),
				Password: "password",
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{
				Error: nil,
			},
		},
	})
}

func (s *deployerSuite) TestSetUnitPasswordUnitNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.passwordService.EXPECT().
		SetUnitPassword(gomock.Any(), unit.Name("foo/1"), "password").
		Return(applicationerrors.UnitNotFound)

	api := &DeployerAPI{
		PasswordChanger: common.NewPasswordChanger(s.passwordService, nil, alwaysAllow),
	}

	result, err := api.SetPasswords(c.Context(), params.EntityPasswords{
		Changes: []params.EntityPassword{
			{
				Tag:      names.NewUnitTag("foo/1").String(),
				Password: "password",
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{
				Error: apiservererrors.ServerError(errors.NotFoundf(`unit "foo/1"`)),
			},
		},
	})
}

func (s *deployerSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.passwordService = NewMockAgentPasswordService(ctrl)
	s.applicationService = NewMockApplicationService(ctrl)
	s.watcherRegistry = facademocks.NewMockWatcherRegistry(ctrl)

	c.Cleanup(func() {
		s.passwordService = nil
		s.applicationService = nil
		s.watcherRegistry = nil
	})

	return ctrl
}

func (s *deployerSuite) getCanWatch(ctx context.Context) (common.AuthFunc, error) {
	return func(tag names.Tag) bool {
		return s.badTag == nil || tag.Id() != s.badTag.Id()
	}, nil
}

func alwaysAllow(context.Context) (common.AuthFunc, error) {
	return func(tag names.Tag) bool {
		return true
	}, nil
}

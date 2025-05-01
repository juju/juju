// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"context"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/mocks"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	cmachine "github.com/juju/juju/core/machine"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
)

type MachineRebootTestSuite struct {
	testing.BaseSuite
	mockRebootService *mocks.MockMachineRebootService
}

var _ = gc.Suite(&MachineRebootTestSuite{})

func (s *MachineRebootTestSuite) setup(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.mockRebootService = mocks.NewMockMachineRebootService(ctrl)
	return ctrl
}

func (s *MachineRebootTestSuite) TestRebootRequestedNoEntity(c *gc.C) {
	// Arrange
	defer s.setup(c).Finish()
	canAccess := func(ctx context.Context) (common.AuthFunc, error) {
		return func(names.Tag) bool {
			return true
		}, nil
	}
	requester := common.NewRebootRequester(s.mockRebootService, canAccess)
	entitiesToRequest := entities() // None

	// Act
	result, err := requester.RequestReboot(context.Background(), entitiesToRequest)

	// Assert
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{},
	})
}

func (s *MachineRebootTestSuite) TestRebootRequestedAuthError(c *gc.C) {
	// Arrange
	defer s.setup(c).Finish()
	authError := errors.New("Oh nooo!!!")
	canAccess := func(ctx context.Context) (common.AuthFunc, error) {
		return func(tag names.Tag) bool {
			return true
		}, authError // THis cause an auth error
	}
	requester := common.NewRebootRequester(s.mockRebootService, canAccess)
	entitiesToRequest := entities("foo/0") // any valid entity would make it

	// Act
	_, err := requester.RequestReboot(context.Background(), entitiesToRequest)

	// Assert
	c.Assert(err, jc.ErrorIs, authError)
}

func (s *MachineRebootTestSuite) TestRebootRequested(c *gc.C) {
	// Arrange
	defer s.setup(c).Finish()
	canAccess := func(ctx context.Context) (common.AuthFunc, error) {
		return func(tag names.Tag) bool {
			// will authorize only machine starting with "42"
			return strings.HasPrefix(tag.Id(), "42")
		}, nil
	}
	requester := common.NewRebootRequester(s.mockRebootService, canAccess)
	entitiesToRequest := entities(
		"invalid_tag",
		"machine-13-notauthorized-0", // not authorized
		"machine-42-nouuid-0",        // should fails on GetUUID
		"machine-42-requestfailed-0", // should fails on requesting reboot
		"machine-42-autorized-0",     // should trigger a reboot
	)
	expect := s.mockRebootService.EXPECT()

	gomock.InOrder(
		expect.GetMachineUUID(gomock.Any(), cmachine.Name("42/nouuid/0")).Return("", errors.New("machine not found")),

		expect.GetMachineUUID(gomock.Any(), cmachine.Name("42/requestfailed/0")).Return("requestfailed-uuid", nil),
		expect.RequireMachineReboot(gomock.Any(), "requestfailed-uuid").Return(errors.New("request failed")),

		expect.GetMachineUUID(gomock.Any(), cmachine.Name("42/autorized/0")).Return("authorized-uuid", nil),
		expect.RequireMachineReboot(gomock.Any(), "authorized-uuid").Return(nil /* no error */),
	)

	// Act
	result, err := requester.RequestReboot(context.Background(), entitiesToRequest)

	// Assert
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: &params.Error{Message: `find machine uuid for machine "42/nouuid/0": machine not found`}},
			{Error: &params.Error{Message: `requires reboot for machine "42/requestfailed/0" (requestfailed-uuid): request failed`}},
			{Error: nil},
		},
	})
}

func (s *MachineRebootTestSuite) TestRebootActionGetNoEntity(c *gc.C) {
	// Arrange
	defer s.setup(c).Finish()
	canAccess := func(ctx context.Context) (common.AuthFunc, error) {
		return func(names.Tag) bool {
			return true
		}, nil
	}
	requester := common.NewRebootActionGetter(s.mockRebootService, canAccess)
	entitiesToRequest := entities() // None

	// Act
	result, err := requester.GetRebootAction(context.Background(), entitiesToRequest)

	// Assert
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.RebootActionResults{
		Results: []params.RebootActionResult{},
	})
}

func (s *MachineRebootTestSuite) TestRebootActionGetAuthError(c *gc.C) {
	// Arrange
	defer s.setup(c).Finish()
	authError := errors.New("Oh nooo!!!")
	canAccess := func(ctx context.Context) (common.AuthFunc, error) {
		return func(tag names.Tag) bool {
			return true
		}, authError // THis cause an auth error
	}
	requester := common.NewRebootActionGetter(s.mockRebootService, canAccess)
	entitiesToRequest := entities("foo/0") // any valid entity would make it

	// Act
	_, err := requester.GetRebootAction(context.Background(), entitiesToRequest)

	// Assert
	c.Assert(err, jc.ErrorIs, authError)
}

func (s *MachineRebootTestSuite) TestRebootActionGet(c *gc.C) {
	// Arrange
	defer s.setup(c).Finish()
	canAccess := func(ctx context.Context) (common.AuthFunc, error) {
		return func(tag names.Tag) bool {
			// will authorize only machine starting with "42"
			return strings.HasPrefix(tag.Id(), "42")
		}, nil
	}
	requester := common.NewRebootActionGetter(s.mockRebootService, canAccess)
	entitiesToRequest := entities(
		"invalid_tag",
		"machine-13-notauthorized-0",      // not authorized
		"machine-42-nouuid-0",             // should fails on GetUUID
		"machine-42-getfailed-0",          // should fails on getting reboot action
		"machine-42-autorizedreboot-0",    // should get shouldReboot
		"machine-42-autorizedshutdown-0",  // should get shouldShutdown
		"machine-42-autorizeddonothing-0", // should get shouldDoNothing
	)
	expect := s.mockRebootService.EXPECT()

	gomock.InOrder(
		expect.GetMachineUUID(gomock.Any(), cmachine.Name("42/nouuid/0")).Return("", errors.New("machine not found")),

		expect.GetMachineUUID(gomock.Any(), cmachine.Name("42/getfailed/0")).Return("getfailed-uuid", nil),
		expect.ShouldRebootOrShutdown(gomock.Any(), "getfailed-uuid").Return("", errors.New("request failed")),

		expect.GetMachineUUID(gomock.Any(), cmachine.Name("42/autorizedreboot/0")).Return("autorizedreboot-uuid", nil),
		expect.ShouldRebootOrShutdown(gomock.Any(), "autorizedreboot-uuid").Return(cmachine.ShouldReboot, nil /* no error */),

		expect.GetMachineUUID(gomock.Any(), cmachine.Name("42/autorizedshutdown/0")).Return("autorizedshutdown-uuid", nil),
		expect.ShouldRebootOrShutdown(gomock.Any(), "autorizedshutdown-uuid").Return(cmachine.ShouldShutdown, nil /* no error */),

		expect.GetMachineUUID(gomock.Any(), cmachine.Name("42/autorizeddonothing/0")).Return("autorizeddonothing-uuid", nil),
		expect.ShouldRebootOrShutdown(gomock.Any(), "autorizeddonothing-uuid").Return(cmachine.ShouldDoNothing, nil /* no error */),
	)

	// Act
	result, err := requester.GetRebootAction(context.Background(), entitiesToRequest)

	// Assert
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.RebootActionResults{
		Results: []params.RebootActionResult{
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: &params.Error{Message: `find machine uuid for machine "42/nouuid/0": machine not found`}},
			{Error: &params.Error{Message: `get reboot action for machine "42/getfailed/0" (getfailed-uuid): request failed`}},
			{Result: params.ShouldReboot},
			{Result: params.ShouldShutdown},
			{Result: params.ShouldDoNothing},
		},
	})
}

func (s *MachineRebootTestSuite) TestRebootClearedNoEntity(c *gc.C) {
	// Arrange
	defer s.setup(c).Finish()
	canAccess := func(ctx context.Context) (common.AuthFunc, error) {
		return func(names.Tag) bool {
			return true
		}, nil
	}
	requester := common.NewRebootFlagClearer(s.mockRebootService, canAccess)
	entitiesToRequest := entities() // None

	// Act
	result, err := requester.ClearReboot(context.Background(), entitiesToRequest)

	// Assert
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{},
	})
}

func (s *MachineRebootTestSuite) TestRebootClearedAuthError(c *gc.C) {
	// Arrange
	defer s.setup(c).Finish()
	authError := errors.New("Oh nooo!!!")
	canAccess := func(ctx context.Context) (common.AuthFunc, error) {
		return func(tag names.Tag) bool {
			return true
		}, authError // THis cause an auth error
	}
	requester := common.NewRebootFlagClearer(s.mockRebootService, canAccess)
	entitiesToRequest := entities("foo/0") // any valid entity would make it

	// Act
	_, err := requester.ClearReboot(context.Background(), entitiesToRequest)

	// Assert
	c.Assert(err, jc.ErrorIs, authError)
}

func (s *MachineRebootTestSuite) TestRebootCleared(c *gc.C) {
	// Arrange
	defer s.setup(c).Finish()
	canAccess := func(ctx context.Context) (common.AuthFunc, error) {
		return func(tag names.Tag) bool {
			// will authorize only machine starting with "42"
			return strings.HasPrefix(tag.Id(), "42")
		}, nil
	}
	requester := common.NewRebootFlagClearer(s.mockRebootService, canAccess)
	entitiesToRequest := entities(
		"invalid_tag",
		"machine-13-notauthorized-0", // not authorized
		"machine-42-nouuid-0",        // should fails on GetUUID
		"machine-42-requestfailed-0", // should fails on requesting reboot
		"machine-42-autorized-0",     // should trigger a reboot
	)

	expect := s.mockRebootService.EXPECT()
	gomock.InOrder(
		expect.GetMachineUUID(gomock.Any(), cmachine.Name("42/nouuid/0")).Return("", errors.New("machine not found")),

		expect.GetMachineUUID(gomock.Any(), cmachine.Name("42/requestfailed/0")).Return("requestfailed-uuid", nil),
		expect.ClearMachineReboot(gomock.Any(), "requestfailed-uuid").Return(errors.New("request failed")),

		expect.GetMachineUUID(gomock.Any(), cmachine.Name("42/autorized/0")).Return("authorized-uuid", nil),
		expect.ClearMachineReboot(gomock.Any(), "authorized-uuid").Return(nil /* no error */),
	)

	// Act
	result, err := requester.ClearReboot(context.Background(), entitiesToRequest)

	// Assert
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: &params.Error{Message: `find machine uuid for machine "42/nouuid/0": machine not found`}},
			{Error: &params.Error{Message: `clear reboot flag for machine "42/requestfailed/0" (requestfailed-uuid): request failed`}},
			{Error: nil},
		},
	})
}

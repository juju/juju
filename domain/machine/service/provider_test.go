// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"testing"

	"github.com/juju/clock"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/status"
	domainmachine "github.com/juju/juju/domain/machine"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	domainstatus "github.com/juju/juju/domain/status"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	statushistory "github.com/juju/juju/internal/statushistory"
	"github.com/juju/juju/internal/testhelpers"
)

type providerServiceSuite struct {
	testhelpers.IsolationSuite

	state         *MockState
	statusHistory *MockStatusHistory
	provider      *MockProvider

	service *ProviderService
}

func TestProviderServiceSuite(t *testing.T) {
	tc.Run(t, &providerServiceSuite{})
}

func (s *providerServiceSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.state = NewMockState(ctrl)
	s.statusHistory = NewMockStatusHistory(ctrl)
	s.provider = NewMockProvider(ctrl)

	providerGetter := func(ctx context.Context) (Provider, error) {
		return s.provider, nil
	}

	s.service = NewProviderService(s.state, s.statusHistory, providerGetter, clock.WallClock, loggertesting.WrapCheckLog(c))

	c.Cleanup(func() {
		s.state = nil
		s.statusHistory = nil
		s.provider = nil
		s.service = nil
	})

	return ctrl
}

func (s *providerServiceSuite) TestCreateMachineProviderNotSupported(c *tc.C) {
	defer s.setupMocks(c).Finish()

	providerGetter := func(ctx context.Context) (Provider, error) {
		return s.provider, coreerrors.NotSupported
	}

	service := NewProviderService(s.state, s.statusHistory, providerGetter, clock.WallClock, loggertesting.WrapCheckLog(c))
	createArgs := CreateMachineArgs{}
	_, _, err := service.CreateMachine(c.Context(), createArgs)
	c.Assert(err, tc.ErrorIs, coreerrors.NotSupported)
}

func (s *providerServiceSuite) TestCreateMachineProviderFailed(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.provider.EXPECT().PrecheckInstance(gomock.Any(), environs.PrecheckInstanceParams{}).Return(errors.Errorf("boom"))

	createArgs := CreateMachineArgs{}
	_, _, err := s.service.CreateMachine(c.Context(), createArgs)
	c.Assert(err, tc.ErrorMatches, `.*boom`)
}

func (s *providerServiceSuite) TestCreateMachine(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.provider.EXPECT().PrecheckInstance(gomock.Any(), environs.PrecheckInstanceParams{}).Return(nil)
	var expectedUUID machine.UUID
	s.state.EXPECT().CreateMachine(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, args domainmachine.CreateMachineArgs) (machine.Name, error) {
			expectedUUID = args.MachineUUID
			return machine.Name("name"), nil
		})

	s.expectCreateMachineStatusHistory(c, machine.Name("name"))

	createArgs := CreateMachineArgs{}
	obtainedUUID, obtainedName, err := s.service.CreateMachine(c.Context(), createArgs)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(obtainedUUID, tc.Equals, expectedUUID)
	c.Check(obtainedName, tc.Equals, machine.Name("name"))
}

func (s *providerServiceSuite) TestCreateMachineSuccessNonce(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.provider.EXPECT().PrecheckInstance(gomock.Any(), environs.PrecheckInstanceParams{}).Return(nil)
	var expectedUUID machine.UUID
	s.state.EXPECT().CreateMachine(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, args domainmachine.CreateMachineArgs) (machine.Name, error) {
			expectedUUID = args.MachineUUID
			c.Check(*args.Nonce, tc.Equals, "foo")
			return machine.Name("name"), nil
		})

	s.expectCreateMachineStatusHistory(c, machine.Name("name"))
	createArgs := CreateMachineArgs{
		Nonce: ptr("foo"),
	}
	obtainedUUID, obtainedName, err := s.service.CreateMachine(c.Context(), createArgs)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(obtainedUUID, tc.Equals, expectedUUID)
	c.Check(obtainedName, tc.Equals, machine.Name("name"))
}

// TestCreateError asserts that an error coming from the state layer is
// preserved, passed over to the service layer to be maintained there.
func (s *providerServiceSuite) TestCreateMachineError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.provider.EXPECT().PrecheckInstance(gomock.Any(), environs.PrecheckInstanceParams{}).Return(nil)

	rErr := errors.New("boom")
	s.state.EXPECT().CreateMachine(gomock.Any(), gomock.Any()).
		Return(machine.Name(""), rErr)

	createArgs := CreateMachineArgs{}
	_, _, err := s.service.CreateMachine(c.Context(), createArgs)
	c.Assert(err, tc.ErrorIs, rErr)
	c.Check(err, tc.ErrorMatches, `creating machine "666": boom`)
}

// TestCreateMachineWithParentSuccess asserts the happy path of the
// CreateMachineWithParent service.
func (s *providerServiceSuite) TestCreateMachineWithParentSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.provider.EXPECT().PrecheckInstance(gomock.Any(), environs.PrecheckInstanceParams{}).Return(nil)
	s.state.EXPECT().GetMachineUUID(gomock.Any(), machine.Name("parent-name")).Return(machine.UUID("uuid"), nil)
	s.state.EXPECT().CreateMachineWithParent(gomock.Any(), gomock.Any(), "uuid").Return(machine.Name("name"), nil)

	s.expectCreateMachineStatusHistory(c, machine.Name("name"))

	_, _, err := s.service.CreateMachineWithParent(c.Context(), CreateMachineArgs{}, machine.Name("parent-name"))
	c.Assert(err, tc.ErrorIsNil)
}

// TestCreateMachineWithParentError asserts that an error coming from the state
// layer is preserved, passed over to the service layer to be maintained there.
func (s *providerServiceSuite) TestCreateMachineWithParentError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.provider.EXPECT().PrecheckInstance(gomock.Any(), environs.PrecheckInstanceParams{}).Return(nil)
	s.state.EXPECT().GetMachineUUID(gomock.Any(), machine.Name("parent-name")).Return(machine.UUID("uuid"), nil)
	rErr := errors.New("boom")
	s.state.EXPECT().CreateMachineWithParent(gomock.Any(), gomock.Any(), "uuid").Return(machine.Name(""), rErr)

	_, _, err := s.service.CreateMachineWithParent(c.Context(), CreateMachineArgs{}, machine.Name("parent-name"))
	c.Assert(err, tc.ErrorIs, rErr)
	c.Assert(err, tc.ErrorMatches, `creating machine with parent "parent-name": boom`)
}

// TestCreateMachineWithParentParentNotFound asserts that the state layer
// returns a NotFound Error if a machine is not found with the given parent
// machineName, and that error is preserved and passed on to the service layer
// to be handled there.
func (s *providerServiceSuite) TestCreateMachineWithParentParentNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.provider.EXPECT().PrecheckInstance(gomock.Any(), environs.PrecheckInstanceParams{}).Return(nil)
	s.state.EXPECT().GetMachineUUID(gomock.Any(), machine.Name("parent-name")).Return(machine.UUID(""), machineerrors.MachineNotFound)

	_, _, err := s.service.CreateMachineWithParent(c.Context(), CreateMachineArgs{}, machine.Name("parent-name"))
	c.Assert(err, tc.ErrorIs, coreerrors.NotFound)
}

func (s *providerServiceSuite) expectCreateMachineStatusHistory(c *tc.C, machineName machine.Name) {
	s.statusHistory.EXPECT().RecordStatus(gomock.Any(), domainstatus.MachineNamespace.WithID(machineName.String()), gomock.Any()).
		DoAndReturn(func(ctx context.Context, n statushistory.Namespace, si status.StatusInfo) error {
			c.Check(si.Status, tc.Equals, status.Pending)
			return nil
		})
	s.statusHistory.EXPECT().RecordStatus(gomock.Any(), domainstatus.MachineInstanceNamespace.WithID(machineName.String()), gomock.Any()).
		DoAndReturn(func(ctx context.Context, n statushistory.Namespace, si status.StatusInfo) error {
			c.Check(si.Status, tc.Equals, status.Pending)
			return nil
		})
}

func (s *providerServiceSuite) expectCreateMachineParentStatusHistory(c *tc.C) {
	s.statusHistory.EXPECT().RecordStatus(gomock.Any(), domainstatus.MachineNamespace.WithID("0/lxd/1"), gomock.Any()).
		DoAndReturn(func(ctx context.Context, n statushistory.Namespace, si status.StatusInfo) error {
			c.Check(si.Status, tc.Equals, status.Pending)
			return nil
		})
	s.statusHistory.EXPECT().RecordStatus(gomock.Any(), domainstatus.MachineInstanceNamespace.WithID("0/lxd/1"), gomock.Any()).
		DoAndReturn(func(ctx context.Context, n statushistory.Namespace, si status.StatusInfo) error {
			c.Check(si.Status, tc.Equals, status.Pending)
			return nil
		})
}

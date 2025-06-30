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

	_, err := service.CreateMachine(c.Context(), "666", nil)
	c.Assert(err, tc.ErrorIs, coreerrors.NotSupported)
}

func (s *providerServiceSuite) TestCreateMachineProviderFailed(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.provider.EXPECT().PrecheckInstance(gomock.Any(), environs.PrecheckInstanceParams{}).Return(errors.Errorf("boom"))

	_, err := s.service.CreateMachine(c.Context(), "666", nil)
	c.Assert(err, tc.ErrorMatches, `.*boom`)
}

func (s *providerServiceSuite) TestCreateMachine(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.provider.EXPECT().PrecheckInstance(gomock.Any(), environs.PrecheckInstanceParams{}).Return(nil)
	s.state.EXPECT().CreateMachine(gomock.Any(), machine.Name("666"), gomock.Any(), gomock.Any(), nil).Return(nil)

	s.expectCreateMachineStatusHistory(c)

	_, err := s.service.CreateMachine(c.Context(), "666", nil)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *providerServiceSuite) TestCreateMachineSuccessNonce(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.provider.EXPECT().PrecheckInstance(gomock.Any(), environs.PrecheckInstanceParams{}).Return(nil)
	s.state.EXPECT().CreateMachine(gomock.Any(), machine.Name("666"), gomock.Any(), gomock.Any(), ptr("foo")).Return(nil)

	s.expectCreateMachineStatusHistory(c)

	_, err := s.service.CreateMachine(c.Context(), "666", ptr("foo"))
	c.Assert(err, tc.ErrorIsNil)
}

// TestCreateError asserts that an error coming from the state layer is
// preserved, passed over to the service layer to be maintained there.
func (s *providerServiceSuite) TestCreateMachineError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.provider.EXPECT().PrecheckInstance(gomock.Any(), environs.PrecheckInstanceParams{}).Return(nil)

	rErr := errors.New("boom")
	s.state.EXPECT().CreateMachine(gomock.Any(), machine.Name("666"), gomock.Any(), gomock.Any(), nil).Return(rErr)

	_, err := s.service.CreateMachine(c.Context(), "666", nil)
	c.Assert(err, tc.ErrorIs, rErr)
	c.Check(err, tc.ErrorMatches, `creating machine "666": boom`)
}

// TestCreateMachineAlreadyExists asserts that the state layer returns a
// MachineAlreadyExists Error if a machine is already found with the given
// machineName, and that error is preserved and passed on to the service layer
// to be handled there.
func (s *providerServiceSuite) TestCreateMachineAlreadyExists(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.provider.EXPECT().PrecheckInstance(gomock.Any(), environs.PrecheckInstanceParams{}).Return(nil)

	s.state.EXPECT().CreateMachine(gomock.Any(), machine.Name("666"), gomock.Any(), gomock.Any(), nil).Return(machineerrors.MachineAlreadyExists)

	_, err := s.service.CreateMachine(c.Context(), machine.Name("666"), nil)
	c.Assert(err, tc.ErrorIs, machineerrors.MachineAlreadyExists)
}

// TestCreateMachineWithParentSuccess asserts the happy path of the
// CreateMachineWithParent service.
func (s *providerServiceSuite) TestCreateMachineWithParentSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.provider.EXPECT().PrecheckInstance(gomock.Any(), environs.PrecheckInstanceParams{}).Return(nil)

	s.state.EXPECT().CreateMachineWithParent(gomock.Any(), machine.Name("0/lxd/1"), gomock.Any(), gomock.Any()).Return(nil)

	s.expectCreateMachineParentStatusHistory(c)

	_, err := s.service.CreateMachineWithParent(c.Context(), machine.Name("1"), machine.Name("0"))
	c.Assert(err, tc.ErrorIsNil)
}

// TestCreateMachineWithParentError asserts that an error coming from the state
// layer is preserved, passed over to the service layer to be maintained there.
func (s *providerServiceSuite) TestCreateMachineWithParentError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.provider.EXPECT().PrecheckInstance(gomock.Any(), environs.PrecheckInstanceParams{}).Return(nil)

	rErr := errors.New("boom")
	s.state.EXPECT().CreateMachineWithParent(gomock.Any(), machine.Name("0/lxd/1"), gomock.Any(), gomock.Any()).Return(rErr)

	_, err := s.service.CreateMachineWithParent(c.Context(), machine.Name("1"), machine.Name("0"))
	c.Assert(err, tc.ErrorIs, rErr)
	c.Check(err, tc.ErrorMatches, `creating machine "0/lxd/1": boom`)
}

// TestCreateMachineWithParentParentNotFound asserts that the state layer
// returns a NotFound Error if a machine is not found with the given parent
// machineName, and that error is preserved and passed on to the service layer
// to be handled there.
func (s *providerServiceSuite) TestCreateMachineWithParentParentNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.provider.EXPECT().PrecheckInstance(gomock.Any(), environs.PrecheckInstanceParams{}).Return(nil)

	s.state.EXPECT().CreateMachineWithParent(gomock.Any(), machine.Name("0/lxd/1"), gomock.Any(), gomock.Any()).Return(coreerrors.NotFound)

	_, err := s.service.CreateMachineWithParent(c.Context(), machine.Name("1"), machine.Name("0"))
	c.Assert(err, tc.ErrorIs, coreerrors.NotFound)
}

// TestCreateMachineWithParentMachineAlreadyExists asserts that the state layer
// returns a MachineAlreadyExists Error if a machine is already found with the
// given machineName, and that error is preserved and passed on to the service
// layer to be handled there.
func (s *providerServiceSuite) TestCreateMachineWithParentMachineAlreadyExists(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.provider.EXPECT().PrecheckInstance(gomock.Any(), environs.PrecheckInstanceParams{}).Return(nil)

	s.state.EXPECT().CreateMachineWithParent(gomock.Any(), machine.Name("0/lxd/1"), gomock.Any(), gomock.Any()).Return(machineerrors.MachineAlreadyExists)

	_, err := s.service.CreateMachineWithParent(c.Context(), machine.Name("1"), machine.Name("0"))
	c.Assert(err, tc.ErrorIs, machineerrors.MachineAlreadyExists)
}

func (s *providerServiceSuite) expectCreateMachineStatusHistory(c *tc.C) {
	s.statusHistory.EXPECT().RecordStatus(gomock.Any(), domainstatus.MachineNamespace.WithID("666"), gomock.Any()).
		DoAndReturn(func(ctx context.Context, n statushistory.Namespace, si status.StatusInfo) error {
			c.Check(si.Status, tc.Equals, status.Pending)
			return nil
		})
	s.statusHistory.EXPECT().RecordStatus(gomock.Any(), domainstatus.MachineInstanceNamespace.WithID("666"), gomock.Any()).
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

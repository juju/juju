// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"testing"

	"github.com/juju/clock"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/base"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/domain/deployment"
	domainmachine "github.com/juju/juju/domain/machine"
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

func (s *providerServiceSuite) TestAddMachineProviderNotSupported(c *tc.C) {
	defer s.setupMocks(c).Finish()

	providerGetter := func(ctx context.Context) (Provider, error) {
		return s.provider, coreerrors.NotSupported
	}

	service := NewProviderService(s.state, s.statusHistory, providerGetter, clock.WallClock, loggertesting.WrapCheckLog(c))
	_, _, err := service.AddMachine(c.Context(), domainmachine.AddMachineArgs{})
	c.Assert(err, tc.ErrorIs, coreerrors.NotSupported)
}

func (s *providerServiceSuite) TestAddMachineProviderFailed(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.provider.EXPECT().PrecheckInstance(gomock.Any(), environs.PrecheckInstanceParams{
		Base: base.Base{
			OS:      "ubuntu",
			Channel: base.Channel{Risk: base.Stable, Track: "22.04"},
		},
	}).Return(errors.Errorf("boom"))

	_, _, err := s.service.AddMachine(c.Context(), domainmachine.AddMachineArgs{
		Platform: deployment.Platform{
			OSType:  deployment.Ubuntu,
			Channel: "22.04",
		},
	})
	c.Assert(err, tc.ErrorMatches, `.*boom`)
}

func (s *providerServiceSuite) TestAddMachine(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.provider.EXPECT().PrecheckInstance(gomock.Any(), environs.PrecheckInstanceParams{
		Base: base.Base{
			OS:      "ubuntu",
			Channel: base.Channel{Risk: base.Stable, Track: "22.04"},
		},
	}).Return(nil)
	s.state.EXPECT().AddMachine(gomock.Any(), gomock.Any()).Return("netNodeUUID", []machine.Name{"name"}, nil)

	s.expectCreateMachineStatusHistory(c, machine.Name("name"))

	netNodeUUID, obtainedNames, err := s.service.AddMachine(c.Context(), domainmachine.AddMachineArgs{
		Platform: deployment.Platform{
			OSType:  deployment.Ubuntu,
			Channel: "22.04",
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(netNodeUUID, tc.Equals, "netNodeUUID")
	c.Check(obtainedNames[0], tc.Equals, machine.Name("name"))
}

func (s *providerServiceSuite) TestAddMachineSuccessNonce(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.provider.EXPECT().PrecheckInstance(gomock.Any(), environs.PrecheckInstanceParams{
		Base: base.Base{
			OS:      "ubuntu",
			Channel: base.Channel{Risk: base.Stable, Track: "22.04"},
		},
	}).Return(nil)
	s.state.EXPECT().AddMachine(gomock.Any(), domainmachine.AddMachineArgs{
		Nonce: ptr("foo"),
		Platform: deployment.Platform{
			OSType:  deployment.Ubuntu,
			Channel: "22.04",
		},
	}).Return("netNodeUUID", []machine.Name{"name"}, nil)

	s.expectCreateMachineStatusHistory(c, machine.Name("name"))

	netNodeUUID, obtainedNames, err := s.service.AddMachine(c.Context(), domainmachine.AddMachineArgs{
		Nonce: ptr("foo"),
		Platform: deployment.Platform{
			OSType:  deployment.Ubuntu,
			Channel: "22.04",
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(netNodeUUID, tc.Equals, "netNodeUUID")
	c.Check(obtainedNames[0], tc.Equals, machine.Name("name"))
}

// TestAddMachineError asserts that an error coming from the state layer is
// preserved, passed over to the service layer to be maintained there.
func (s *providerServiceSuite) TestAddMachineError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.provider.EXPECT().PrecheckInstance(gomock.Any(), environs.PrecheckInstanceParams{
		Base: base.Base{
			OS:      "ubuntu",
			Channel: base.Channel{Risk: base.Stable, Track: "22.04"},
		},
	}).Return(nil)

	rErr := errors.New("boom")
	s.state.EXPECT().AddMachine(gomock.Any(), gomock.Any()).Return("", nil, rErr)

	_, _, err := s.service.AddMachine(c.Context(), domainmachine.AddMachineArgs{
		Platform: deployment.Platform{
			OSType:  deployment.Ubuntu,
			Channel: "22.04",
		},
	})
	c.Assert(err, tc.ErrorIs, rErr)
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

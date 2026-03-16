// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"testing"

	"github.com/juju/clock"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/arch"
	"github.com/juju/juju/core/base"
	coreconstraints "github.com/juju/juju/core/constraints"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/domain/constraints"
	"github.com/juju/juju/domain/deployment"
	domainmachine "github.com/juju/juju/domain/machine"
	modelerrors "github.com/juju/juju/domain/model/errors"
	domainstatus "github.com/juju/juju/domain/status"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/statushistory"
	"github.com/juju/juju/internal/testhelpers"
)

type providerServiceSuite struct {
	testhelpers.IsolationSuite

	state         *MockState
	statusHistory *MockStatusHistory
	provider      *MockProvider
	validator     *MockValidator

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
	s.validator = NewMockValidator(ctrl)

	providerGetter := func(ctx context.Context) (Provider, error) {
		return s.provider, nil
	}

	s.service = NewProviderService(
		s.state,
		s.statusHistory,
		providerGetter,
		clock.WallClock,
		loggertesting.WrapCheckLog(c),
	)

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

	service := NewProviderService(
		s.state, s.statusHistory, providerGetter, clock.WallClock, loggertesting.WrapCheckLog(c))

	_, err := service.AddMachine(c.Context(), domainmachine.AddMachineArgs{
		Platform: deployment.Platform{
			OSType:  deployment.Ubuntu,
			Channel: "22.04",
		},
	})
	c.Assert(err, tc.ErrorIs, coreerrors.NotSupported)
}

func (s *providerServiceSuite) TestAddMachineProviderFailed(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.provider.EXPECT().ConstraintsValidator(gomock.Any()).Return(nil, errors.Errorf("boom"))

	_, err := s.service.AddMachine(c.Context(), domainmachine.AddMachineArgs{
		Platform: deployment.Platform{
			OSType:  deployment.Ubuntu,
			Channel: "22.04",
		},
	})
	c.Assert(err, tc.ErrorMatches, `.*boom`)
}

func (s *providerServiceSuite) TestAddMachine(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetModelConstraints(gomock.Any()).Return(constraints.Constraints{}, nil)
	s.provider.EXPECT().ConstraintsValidator(gomock.Any()).Return(coreconstraints.NewValidator(), nil)
	s.provider.EXPECT().PrecheckInstance(gomock.Any(), environs.PrecheckInstanceParams{
		Base: base.Base{
			OS:      "ubuntu",
			Channel: base.Channel{Risk: base.Stable, Track: "22.04"},
		},
	}).Return(nil)
	s.state.EXPECT().AddMachine(gomock.Any(), gomock.Any()).Return("netNodeUUID", []machine.Name{"name"}, nil)

	s.expectCreateMachineStatusHistory(c, machine.Name("name"))

	res, err := s.service.AddMachine(c.Context(), domainmachine.AddMachineArgs{
		Platform: deployment.Platform{
			OSType:  deployment.Ubuntu,
			Channel: "22.04",
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(res.MachineName, tc.Equals, machine.Name("name"))
}

func (s *providerServiceSuite) TestAddMachineSuccessNonce(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetModelConstraints(gomock.Any()).Return(constraints.Constraints{}, nil)
	s.provider.EXPECT().ConstraintsValidator(gomock.Any()).Return(coreconstraints.NewValidator(), nil)
	s.provider.EXPECT().PrecheckInstance(gomock.Any(), environs.PrecheckInstanceParams{
		Base: base.Base{
			OS:      "ubuntu",
			Channel: base.Channel{Risk: base.Stable, Track: "22.04"},
		},
	}).Return(nil)
	s.state.EXPECT().AddMachine(gomock.Any(), domainmachine.AddMachineArgs{
		Nonce: new("foo"),
		Platform: deployment.Platform{
			OSType:  deployment.Ubuntu,
			Channel: "22.04",
		},
	}).Return("netNodeUUID", []machine.Name{"name"}, nil)

	s.expectCreateMachineStatusHistory(c, machine.Name("name"))

	res, err := s.service.AddMachine(c.Context(), domainmachine.AddMachineArgs{
		Nonce: new("foo"),
		Platform: deployment.Platform{
			OSType:  deployment.Ubuntu,
			Channel: "22.04",
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(res.MachineName, tc.Equals, machine.Name("name"))
}

func (s *providerServiceSuite) TestAddMachineContainer(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetModelConstraints(gomock.Any()).Return(constraints.Constraints{}, nil)
	s.provider.EXPECT().ConstraintsValidator(gomock.Any()).Return(coreconstraints.NewValidator(), nil)
	s.provider.EXPECT().PrecheckInstance(gomock.Any(), environs.PrecheckInstanceParams{
		Base: base.Base{
			OS:      "ubuntu",
			Channel: base.Channel{Risk: base.Stable, Track: "22.04"},
		},
	}).Return(nil)
	s.state.EXPECT().AddMachine(gomock.Any(), gomock.Any()).Return("netNodeUUID", []machine.Name{"0", "0/lxd/0"}, nil)

	s.expectCreateMachineStatusHistory(c, machine.Name("0"))
	s.expectCreateMachineStatusHistory(c, machine.Name("0/lxd/0"))

	res, err := s.service.AddMachine(c.Context(), domainmachine.AddMachineArgs{
		Directive: deployment.Placement{
			Type:      deployment.PlacementTypeContainer,
			Container: deployment.ContainerTypeLXD,
			Directive: "0",
		},
		Platform: deployment.Platform{
			OSType:  deployment.Ubuntu,
			Channel: "22.04",
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(res.MachineName, tc.Equals, machine.Name("0"))
	c.Assert(res.ChildMachineName, tc.NotNil)
	c.Check(*res.ChildMachineName, tc.Equals, machine.Name("0/lxd/0"))
}

// TestAddMachineError asserts that an error coming from the state layer is
// preserved, passed over to the service layer to be maintained there.
func (s *providerServiceSuite) TestAddMachineError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetModelConstraints(gomock.Any()).Return(constraints.Constraints{}, nil)
	s.provider.EXPECT().ConstraintsValidator(gomock.Any()).Return(coreconstraints.NewValidator(), nil)
	s.provider.EXPECT().PrecheckInstance(gomock.Any(), environs.PrecheckInstanceParams{
		Base: base.Base{
			OS:      "ubuntu",
			Channel: base.Channel{Risk: base.Stable, Track: "22.04"},
		},
	}).Return(nil)

	rErr := errors.New("boom")
	s.state.EXPECT().AddMachine(gomock.Any(), gomock.Any()).Return("", nil, rErr)

	_, err := s.service.AddMachine(c.Context(), domainmachine.AddMachineArgs{
		Platform: deployment.Platform{
			OSType:  deployment.Ubuntu,
			Channel: "22.04",
		},
	})
	c.Assert(err, tc.ErrorIs, rErr)
}

func (s *providerServiceSuite) TestMergeApplicationAndModelConstraintsNotSupported(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.provider.EXPECT().ConstraintsValidator(gomock.Any()).Return(s.validator, errors.Errorf("not supported %w", coreerrors.NotSupported))

	_, err := s.service.mergeMachineAndModelConstraints(c.Context(), constraints.Constraints{})
	c.Assert(err, tc.ErrorIs, coreerrors.NotSupported)
}

func (s *providerServiceSuite) TestMergeApplicationAndModelConstraintsNilValidator(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetModelConstraints(gomock.Any()).Return(constraints.Constraints{}, nil)
	s.provider.EXPECT().ConstraintsValidator(gomock.Any()).Return(nil, nil)

	cons, err := s.service.mergeMachineAndModelConstraints(c.Context(), constraints.Constraints{})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(cons, tc.DeepEquals, coreconstraints.Value{})
}

func (s *providerServiceSuite) TestMergeApplicationAndModelConstraintsConstraintsNotFound(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.provider.EXPECT().ConstraintsValidator(gomock.Any()).Return(s.validator, nil)

	s.state.EXPECT().GetModelConstraints(gomock.Any()).Return(constraints.Constraints{}, modelerrors.ConstraintsNotFound)

	s.validator.EXPECT().Merge(
		constraints.EncodeConstraints(constraints.Constraints{}),
		constraints.EncodeConstraints(constraints.Constraints{})).
		Return(coreconstraints.Value{}, nil)

	_, err := s.service.mergeMachineAndModelConstraints(c.Context(), constraints.Constraints{})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *providerServiceSuite) TestMergeApplicationAndModelConstraintsNotSubordinateWithArch(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.provider.EXPECT().ConstraintsValidator(gomock.Any()).Return(s.validator, nil)

	s.state.EXPECT().GetModelConstraints(gomock.Any()).Return(constraints.Constraints{}, modelerrors.ConstraintsNotFound)

	s.validator.EXPECT().Merge(
		constraints.EncodeConstraints(constraints.Constraints{}),
		constraints.EncodeConstraints(constraints.Constraints{
			Arch: new(arch.AMD64),
		})).
		Return(coreconstraints.Value{
			Arch: new(arch.AMD64),
		}, nil)

	merged, err := s.service.mergeMachineAndModelConstraints(c.Context(), constraints.Constraints{
		Arch: new(arch.AMD64),
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(*merged.Arch, tc.Equals, arch.AMD64)
}

func (s *providerServiceSuite) TestMergeApplicationAndModelConstraintsSubordinateWithArch(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.provider.EXPECT().ConstraintsValidator(gomock.Any()).Return(s.validator, nil)

	s.state.EXPECT().GetModelConstraints(gomock.Any()).Return(constraints.Constraints{
		RootDiskSource: new("source-disk"),
		Mem:            new(uint64(42)),
	}, modelerrors.ConstraintsNotFound)

	s.validator.EXPECT().Merge(
		constraints.EncodeConstraints(constraints.Constraints{
			RootDiskSource: new("source-disk"),
			Mem:            new(uint64(42)),
		}),
		constraints.EncodeConstraints(constraints.Constraints{
			Arch: new(arch.AMD64),
		})).
		Return(coreconstraints.Value{
			Arch:           new(arch.AMD64),
			RootDiskSource: new("source-disk"),
			Mem:            new(uint64(42)),
		}, nil)

	merged, err := s.service.mergeMachineAndModelConstraints(c.Context(), constraints.Constraints{
		Arch: new(arch.AMD64),
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(*merged.RootDiskSource, tc.Equals, "source-disk")
	c.Check(*merged.Mem, tc.Equals, uint64(42))
}

func (s *providerServiceSuite) TestMergeApplicationAndModelConstraintsNotSubordinateWithoutArch(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.provider.EXPECT().ConstraintsValidator(gomock.Any()).Return(s.validator, nil)

	s.state.EXPECT().GetModelConstraints(gomock.Any()).Return(constraints.Constraints{
		Mem: new(uint64(42)),
	}, modelerrors.ConstraintsNotFound)

	s.validator.EXPECT().Merge(
		constraints.EncodeConstraints(constraints.Constraints{
			Mem: new(uint64(42)),
		}),
		constraints.EncodeConstraints(constraints.Constraints{
			RootDiskSource: new("source-disk"),
		})).
		Return(coreconstraints.Value{
			RootDiskSource: new("source-disk"),
			Mem:            new(uint64(42)),
		}, nil)

	merged, err := s.service.mergeMachineAndModelConstraints(c.Context(), constraints.Constraints{
		RootDiskSource: new("source-disk"),
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(*merged.RootDiskSource, tc.Equals, "source-disk")
	c.Check(*merged.Mem, tc.Equals, uint64(42))
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

func (s *providerServiceSuite) TestGetBootstrapEnviron(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	// Arrange
	providerGetter := func(ctx context.Context) (Provider, error) {
		return s.provider, nil
	}
	service := NewProviderService(s.state, nil, providerGetter, nil, loggertesting.WrapCheckLog(c))

	// Act
	p, err := service.GetBootstrapEnviron(c.Context())

	// Assert
	c.Assert(err, tc.IsNil)
	c.Assert(p, tc.NotNil)
}

func (s *providerServiceSuite) TestGetBootstrapEnvironFail(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	// Arrange
	providerGetter := func(ctx context.Context) (Provider, error) {
		return nil, errors.Errorf("boom")
	}
	service := NewProviderService(s.state, nil, providerGetter, nil, loggertesting.WrapCheckLog(c))

	// Act
	_, err := service.GetBootstrapEnviron(c.Context())

	// Assert
	c.Assert(err, tc.ErrorMatches, "boom")
}

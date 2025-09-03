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
	"github.com/juju/juju/core/lxdprofile"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/domain/constraints"
	"github.com/juju/juju/domain/deployment"
	domainmachine "github.com/juju/juju/domain/machine"
	"github.com/juju/juju/domain/machine/internal"
	modelerrors "github.com/juju/juju/domain/model/errors"
	domainstatus "github.com/juju/juju/domain/status"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	statushistory "github.com/juju/juju/internal/statushistory"
	"github.com/juju/juju/internal/testhelpers"
	coretesting "github.com/juju/juju/internal/testing"
)

type providerServiceSuite struct {
	testhelpers.IsolationSuite

	state              *MockState
	statusHistory      *MockStatusHistory
	provider           *MockProvider
	lxdProfileProvider *MockLXDProfileProvider
	validator          *MockValidator

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

	lxdProfileProviderGetter := func(ctx context.Context) (LXDProfileProvider, error) {
		return s.lxdProfileProvider, nil
	}

	s.service = NewProviderService(
		s.state,
		s.statusHistory,
		providerGetter,
		lxdProfileProviderGetter,
		clock.WallClock,
		loggertesting.WrapCheckLog(c),
	)

	c.Cleanup(func() {
		s.state = nil
		s.statusHistory = nil
		s.provider = nil
		s.lxdProfileProvider = nil
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
		s.state, s.statusHistory, providerGetter, nil, clock.WallClock, loggertesting.WrapCheckLog(c))

	_, err := service.AddMachine(c.Context(), domainmachine.AddMachineArgs{})
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
		Constraints: coreconstraints.Value{
			// Default arch should be added in this case.
			Arch: ptr(arch.AMD64),
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
		Constraints: coreconstraints.Value{
			// Default arch should be added in this case.
			Arch: ptr(arch.AMD64),
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

	res, err := s.service.AddMachine(c.Context(), domainmachine.AddMachineArgs{
		Nonce: ptr("foo"),
		Platform: deployment.Platform{
			OSType:  deployment.Ubuntu,
			Channel: "22.04",
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(res.MachineName, tc.Equals, machine.Name("name"))
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
		Constraints: coreconstraints.Value{
			// Default arch should be added in this case.
			Arch: ptr(arch.AMD64),
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
	c.Check(cons, tc.DeepEquals, coreconstraints.Value{
		Arch: ptr(arch.AMD64),
	})
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
			Arch: ptr(arch.AMD64),
		})).
		Return(coreconstraints.Value{
			Arch: ptr(arch.AMD64),
		}, nil)

	merged, err := s.service.mergeMachineAndModelConstraints(c.Context(), constraints.Constraints{
		Arch: ptr(arch.AMD64),
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(*merged.Arch, tc.Equals, arch.AMD64)
}

func (s *providerServiceSuite) TestMergeApplicationAndModelConstraintsSubordinateWithArch(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.provider.EXPECT().ConstraintsValidator(gomock.Any()).Return(s.validator, nil)

	s.state.EXPECT().GetModelConstraints(gomock.Any()).Return(constraints.Constraints{
		RootDiskSource: ptr("source-disk"),
		Mem:            ptr(uint64(42)),
	}, modelerrors.ConstraintsNotFound)

	s.validator.EXPECT().Merge(
		constraints.EncodeConstraints(constraints.Constraints{
			RootDiskSource: ptr("source-disk"),
			Mem:            ptr(uint64(42)),
		}),
		constraints.EncodeConstraints(constraints.Constraints{
			Arch: ptr(arch.AMD64),
		})).
		Return(coreconstraints.Value{
			Arch:           ptr(arch.AMD64),
			RootDiskSource: ptr("source-disk"),
			Mem:            ptr(uint64(42)),
		}, nil)

	merged, err := s.service.mergeMachineAndModelConstraints(c.Context(), constraints.Constraints{
		Arch: ptr(arch.AMD64),
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(*merged.Arch, tc.Equals, arch.AMD64)
	c.Check(*merged.RootDiskSource, tc.Equals, "source-disk")
	c.Check(*merged.Mem, tc.Equals, uint64(42))
}

func (s *providerServiceSuite) TestMergeApplicationAndModelConstraintsNotSubordinateWithoutArch(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.provider.EXPECT().ConstraintsValidator(gomock.Any()).Return(s.validator, nil)

	s.state.EXPECT().GetModelConstraints(gomock.Any()).Return(constraints.Constraints{
		Mem: ptr(uint64(42)),
	}, modelerrors.ConstraintsNotFound)

	s.validator.EXPECT().Merge(
		constraints.EncodeConstraints(constraints.Constraints{
			Mem: ptr(uint64(42)),
		}),
		constraints.EncodeConstraints(constraints.Constraints{
			RootDiskSource: ptr("source-disk"),
		})).
		Return(coreconstraints.Value{
			RootDiskSource: ptr("source-disk"),
			Mem:            ptr(uint64(42)),
		}, nil)

	merged, err := s.service.mergeMachineAndModelConstraints(c.Context(), constraints.Constraints{
		RootDiskSource: ptr("source-disk"),
	})
	c.Assert(err, tc.ErrorIsNil)
	// Default arch should be added in this case.
	c.Check(*merged.Arch, tc.Equals, arch.AMD64)
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

type lxdProviderServiceSuite struct {
	testhelpers.IsolationSuite

	state              *MockState
	provider           *MockProvider
	lxdProfileProvider *MockLXDProfileProvider
}

func TestLXDProviderServiceSuite(t *testing.T) {
	tc.Run(t, &lxdProviderServiceSuite{})
}

func (s *lxdProviderServiceSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.provider = NewMockProvider(ctrl)
	s.lxdProfileProvider = NewMockLXDProfileProvider(ctrl)
	s.state = NewMockState(ctrl)

	c.Cleanup(func() {
		s.provider = nil
		s.state = nil
	})

	return ctrl
}

func (s *lxdProviderServiceSuite) TestUpdateLXDProfiles(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	// Arrange
	machineID := "7"
	result := []internal.CreateLXDProfileDetails{
		{
			ApplicationName: "ubuntu",
			CharmRevision:   4,
			LXDProfile:      []byte(`{"config": {"foo":"bar"}, "description": "description", "devices": {"gpu":{"baz": "x"}}}`),
		}, {
			ApplicationName: "test",
			CharmRevision:   8,
			LXDProfile:      []byte(`{"config": {"foo":"baz"}, "description": "another"}`),
		},
	}
	pName0 := "juju-test-deadbe-ubuntu-4"
	pName1 := "juju-test-deadbe-test-8"
	s.state.EXPECT().GetLXDProfilesForMachine(gomock.Any(), machineID).Return(result, nil)
	s.lxdProfileProvider.EXPECT().MaybeWriteLXDProfile(pName0, lxdprofile.Profile{
		Config:      map[string]string{"foo": "bar"},
		Description: "description",
		Devices:     map[string]map[string]string{"gpu": {"baz": "x"}},
	}).Return(nil)
	s.lxdProfileProvider.EXPECT().MaybeWriteLXDProfile(pName1, lxdprofile.Profile{
		Config:      map[string]string{"foo": "baz"},
		Description: "another",
	}).Return(nil)

	providerGetter := func(ctx context.Context) (LXDProfileProvider, error) {
		return s.lxdProfileProvider, nil
	}
	service := NewProviderService(s.state, nil, nil, providerGetter, nil, loggertesting.WrapCheckLog(c))

	// Act
	obtainedProfileNames, err := service.UpdateLXDProfiles(c.Context(), "test", coretesting.ModelTag.Id(), machineID)

	// Assert:
	c.Assert(err, tc.IsNil)
	c.Assert(obtainedProfileNames, tc.SameContents, []string{pName0, pName1})
}

func (s *lxdProviderServiceSuite) TestUpdateLXDProfilesNoSupport(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	// Arrange: the provider does not support LXDProfiles
	providerGetter := func(ctx context.Context) (LXDProfileProvider, error) {
		return nil, coreerrors.NotSupported
	}
	service := NewProviderService(s.state, nil, nil, providerGetter, nil, loggertesting.WrapCheckLog(c))

	// Act
	_, err := service.UpdateLXDProfiles(c.Context(), "blue", coretesting.ModelTag.Id(), "7")

	// Assert: no work is done and the method doesn't fail
	c.Assert(err, tc.IsNil)
}

func (s *lxdProviderServiceSuite) TestUpdateLXDProfilesFail(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	// Arrange
	providerGetter := func(ctx context.Context) (LXDProfileProvider, error) {
		return nil, errors.Errorf("boom")
	}
	service := NewProviderService(s.state, nil, nil, providerGetter, nil, loggertesting.WrapCheckLog(c))

	// Act
	_, err := service.UpdateLXDProfiles(c.Context(), "blue", coretesting.ModelTag.Id(), "7")

	// Assert
	c.Assert(err, tc.ErrorMatches, "getting provider: boom")
}

func (s *lxdProviderServiceSuite) TestGetBootstrapEnviron(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	// Arrange
	providerGetter := func(ctx context.Context) (Provider, error) {
		return s.provider, nil
	}
	service := NewProviderService(s.state, nil, providerGetter, nil, nil, loggertesting.WrapCheckLog(c))

	// Act
	p, err := service.GetBootstrapEnviron(c.Context())

	// Assert
	c.Assert(err, tc.IsNil)
	c.Assert(p, tc.NotNil)
}

func (s *lxdProviderServiceSuite) TestGetBootstrapEnvironFail(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	// Arrange
	providerGetter := func(ctx context.Context) (Provider, error) {
		return nil, errors.Errorf("boom")
	}
	service := NewProviderService(s.state, nil, providerGetter, nil, nil, loggertesting.WrapCheckLog(c))

	// Act
	_, err := service.GetBootstrapEnviron(c.Context())

	// Assert
	c.Assert(err, tc.ErrorMatches, "boom")
}

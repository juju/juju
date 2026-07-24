// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"testing"
	"time"

	"github.com/canonical/gomock/gomock"
	"github.com/juju/clock"
	"github.com/juju/clock/testclock"
	"github.com/juju/tc"

	"github.com/juju/juju/core/arch"
	"github.com/juju/juju/core/base"
	coreconstraints "github.com/juju/juju/core/constraints"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/machine"
	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/core/network/ipfamily"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/domain/constraints"
	"github.com/juju/juju/domain/deployment"
	domainmachine "github.com/juju/juju/domain/machine"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	modelerrors "github.com/juju/juju/domain/model/errors"
	domainstatus "github.com/juju/juju/domain/status"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/instances"
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

func (s *providerServiceSuite) TestAddMachineUnsupportedConstraintWarns(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	// Use a real validator with ip-family registered as unsupported.
	// The real validator's Validate will return ip-family in the
	// unsupported slice, triggering the warn-and-ignore log.
	realValidator := coreconstraints.NewValidator()
	realValidator.RegisterUnsupported([]string{"ip-family"})

	s.provider.EXPECT().ConstraintsValidator(gomock.Any()).Return(realValidator, nil)
	s.state.EXPECT().GetModelConstraints(gomock.Any()).Return(constraints.Constraints{}, nil)
	s.provider.EXPECT().PrecheckInstance(gomock.Any(), gomock.Any()).Return(nil)
	s.state.EXPECT().AddMachine(gomock.Any(), gomock.Any()).Return("netNodeUUID", []machine.Name{"name"}, nil)
	s.expectCreateMachineStatusHistory(c, machine.Name("name"))

	res, err := s.service.AddMachine(c.Context(), domainmachine.AddMachineArgs{
		Constraints: constraints.Constraints{
			IPFamily: new(ipfamily.Dual),
		},
		Platform: deployment.Platform{
			OSType:  deployment.Ubuntu,
			Channel: "22.04",
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(res.MachineName, tc.Equals, machine.Name("name"))
}

func (s *providerServiceSuite) TestAddMachineVocabError(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	// Use a real validator with ip-family vocabulary restricted to ipv4.
	// ip-family=dual will cause a vocab error, which should propagate
	// as a hard error from AddMachine.
	realValidator := coreconstraints.NewValidator()
	realValidator.RegisterVocabulary("ip-family", []string{"ipv4"})

	s.provider.EXPECT().ConstraintsValidator(gomock.Any()).Return(realValidator, nil)
	s.state.EXPECT().GetModelConstraints(gomock.Any()).Return(constraints.Constraints{}, nil)

	_, err := s.service.AddMachine(c.Context(), domainmachine.AddMachineArgs{
		Constraints: constraints.Constraints{
			IPFamily: new(ipfamily.Dual),
		},
		Platform: deployment.Platform{
			OSType:  deployment.Ubuntu,
			Channel: "22.04",
		},
	})
	c.Assert(err, tc.ErrorMatches, "(?s).*invalid constraint value.*")
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
	s.validator.EXPECT().Validate(coreconstraints.Value{}).
		Return(nil, nil)

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
	s.validator.EXPECT().Validate(coreconstraints.Value{
		Arch: new(arch.AMD64),
	}).Return(nil, nil)

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
	s.validator.EXPECT().Validate(coreconstraints.Value{
		Arch:           new(arch.AMD64),
		RootDiskSource: new("source-disk"),
		Mem:            new(uint64(42)),
	}).Return(nil, nil)

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
	s.validator.EXPECT().Validate(coreconstraints.Value{
		RootDiskSource: new("source-disk"),
		Mem:            new(uint64(42)),
	}).Return(nil, nil)

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

func (s *providerServiceSuite) TestReprovisionMachineAgentPresent(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectReprovisionMachineValidated(c, "i-1234")
	s.state.EXPECT().IsMachineAgentPresent(gomock.Any(), machine.Name("0")).Return(true, nil)

	err := s.service.ReprovisionMachine(c.Context(), machine.Name("0"))
	c.Assert(err, tc.ErrorIs, machineerrors.MachineAgentPresent)
}

func (s *providerServiceSuite) TestReprovisionMachineAgentAbsentNoInstance(c *tc.C) {
	defer s.setupMocks(c).Finish()

	instanceID := instance.Id("i-1234")
	s.expectReprovisionMachineValidated(c, instanceID)
	s.expectMachineAgentAbsent()
	s.provider.EXPECT().Instances(gomock.Any(), []instance.Id{instanceID}).Return(nil, environs.ErrNoInstances)

	err := s.service.ReprovisionMachine(c.Context(), machine.Name("0"))
	c.Assert(err, tc.ErrorIsNil)
}

func (s *providerServiceSuite) TestReprovisionMachineProviderRunning(c *tc.C) {
	defer s.setupMocks(c).Finish()

	instanceID := instance.Id("i-1234")
	s.expectReprovisionMachineValidated(c, instanceID)
	s.expectMachineAgentAbsent()
	s.provider.EXPECT().Instances(gomock.Any(), []instance.Id{instanceID}).Return([]instances.Instance{
		reprovisionInstance{id: instanceID, status: status.Running},
	}, nil)

	err := s.service.ReprovisionMachine(c.Context(), machine.Name("0"))
	c.Assert(err, tc.ErrorIs, machineerrors.MachineProviderInstanceRunning)
}

func (s *providerServiceSuite) TestReprovisionMachineProviderLookupError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	instanceID := instance.Id("i-1234")
	s.expectReprovisionMachineValidated(c, instanceID)
	s.expectMachineAgentAbsent()
	s.provider.EXPECT().Instances(gomock.Any(), []instance.Id{instanceID}).Return(nil, errors.New("provider lookup failed"))

	err := s.service.ReprovisionMachine(c.Context(), machine.Name("0"))
	c.Assert(err, tc.ErrorMatches, `checking provider instance "i-1234" for machine "0": provider lookup failed`)
}

func (s *providerServiceSuite) TestReprovisionMachineProviderNoInstance(c *tc.C) {
	defer s.setupMocks(c).Finish()

	instanceID := instance.Id("i-1234")
	s.expectReprovisionMachineValidated(c, instanceID)
	s.expectMachineAgentAbsent()
	s.provider.EXPECT().Instances(gomock.Any(), []instance.Id{instanceID}).Return(nil, environs.ErrNoInstances)

	err := s.service.ReprovisionMachine(c.Context(), machine.Name("0"))
	c.Assert(err, tc.ErrorIsNil)
}

func (s *providerServiceSuite) TestReprovisionMachineDetachError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	instanceID := instance.Id("i-1234")
	s.expectReprovisionMachineValidated(c, instanceID)
	s.expectMachineAgentAbsent()
	s.provider.EXPECT().Instances(gomock.Any(), []instance.Id{instanceID}).Return([]instances.Instance{
		reprovisionInstance{id: instanceID, status: status.Error},
	}, nil)
	statusData := []byte(`{"old-instance-id":"i-1234"}`)
	s.state.EXPECT().DetachLostMachineCloudInstance(
		gomock.Any(), "0", instanceID.String(), reprovisioningStatusMessage,
		statusData, gomock.Any(),
	).Return(errors.New("detach failed"))

	err := s.service.ReprovisionMachine(c.Context(), machine.Name("0"))
	c.Assert(err, tc.ErrorMatches, `detaching lost cloud instance for machine "0": detach failed`)
}

func (s *providerServiceSuite) TestDetachLostMachineCloudInstanceStatusData(c *tc.C) {
	defer s.setupMocks(c).Finish()

	now := time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)
	s.service.clock = testclock.NewClock(now)
	statusData := map[string]any{
		"old-instance-id": "i-1234",
	}
	encodedStatusData := []byte(`{"old-instance-id":"i-1234"}`)
	statusInfo := status.StatusInfo{
		Status:  status.Pending,
		Message: reprovisioningStatusMessage,
		Data:    statusData,
		Since:   &now,
	}
	s.state.EXPECT().DetachLostMachineCloudInstance(
		gomock.Any(), "0", "i-1234",
		reprovisioningStatusMessage, encodedStatusData, now,
	).Return(nil)
	s.statusHistory.EXPECT().RecordStatus(
		gomock.Any(), domainstatus.MachineNamespace.WithID("0"), statusInfo,
	).Return(nil)
	s.statusHistory.EXPECT().RecordStatus(
		gomock.Any(), domainstatus.MachineInstanceNamespace.WithID("0"), statusInfo,
	).Return(nil)

	err := s.service.detachLostMachineCloudInstance(
		c.Context(), machine.Name("0"), instance.Id("i-1234"),
	)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *providerServiceSuite) TestReprovisionMachineProviderPartialNoInstance(c *tc.C) {
	defer s.setupMocks(c).Finish()

	instanceID := instance.Id("i-1234")
	s.expectReprovisionMachineValidated(c, instanceID)
	s.expectMachineAgentAbsent()
	s.provider.EXPECT().Instances(gomock.Any(), []instance.Id{instanceID}).Return([]instances.Instance{nil}, environs.ErrPartialInstances)

	err := s.service.ReprovisionMachine(c.Context(), machine.Name("0"))
	c.Assert(err, tc.ErrorIsNil)
}

func (s *providerServiceSuite) TestReprovisionMachineProviderNonRunningStatuses(c *tc.C) {
	for _, providerStatus := range []status.Status{
		status.Empty,
		status.Pending,
		status.Provisioning,
		status.ProvisioningError,
		status.Error,
		status.Unknown,
	} {
		c.Logf("provider status %q", providerStatus)
		func() {
			ctrl := s.setupMocks(c)
			defer ctrl.Finish()

			instanceID := instance.Id("i-1234")
			s.expectReprovisionMachineValidated(c, instanceID)
			s.expectMachineAgentAbsent()
			s.provider.EXPECT().Instances(gomock.Any(), []instance.Id{instanceID}).Return([]instances.Instance{
				reprovisionInstance{id: instanceID, status: providerStatus},
			}, nil)
			s.expectMachineDetached()

			err := s.service.ReprovisionMachine(c.Context(), machine.Name("0"))
			c.Assert(err, tc.ErrorIsNil)
		}()
	}
}

func (s *providerServiceSuite) expectReprovisionMachineValidated(c *tc.C, instanceID instance.Id) {
	s.state.EXPECT().CheckMachineReprovisioningEligibility(gomock.Any(), machine.Name("0")).Return(nil)
	s.state.EXPECT().GetInstanceIDByMachineName(gomock.Any(), machine.Name("0")).Return(instanceID.String(), nil)
}

func (s *providerServiceSuite) expectMachineAgentAbsent() {
	s.state.EXPECT().IsMachineAgentPresent(gomock.Any(), machine.Name("0")).Return(false, nil)
}

func (s *providerServiceSuite) expectMachineDetached() {
	statusData := []byte(`{"old-instance-id":"i-1234"}`)
	s.state.EXPECT().DetachLostMachineCloudInstance(
		gomock.Any(), "0", "i-1234",
		reprovisioningStatusMessage, statusData, gomock.Any(),
	).Return(nil)
	s.statusHistory.EXPECT().RecordStatus(
		gomock.Any(), domainstatus.MachineNamespace.WithID("0"), gomock.Any(),
	).Return(nil)
	s.statusHistory.EXPECT().RecordStatus(
		gomock.Any(), domainstatus.MachineInstanceNamespace.WithID("0"), gomock.Any(),
	).Return(nil)
}

type reprovisionInstance struct {
	id     instance.Id
	status status.Status
}

func (i reprovisionInstance) Id() instance.Id {
	return i.id
}

func (i reprovisionInstance) Status(context.Context) instance.Status {
	return instance.Status{Status: i.status}
}

func (i reprovisionInstance) Addresses(context.Context) (corenetwork.ProviderAddresses, error) {
	return nil, nil
}

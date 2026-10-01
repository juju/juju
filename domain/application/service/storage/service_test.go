// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"slices"
	"testing"

	"github.com/canonical/gomock/gomock"
	"github.com/juju/tc"

	coreapplication "github.com/juju/juju/core/application"
	coreerrors "github.com/juju/juju/core/errors"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/application/charm"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/application/internal"
	domainnetwork "github.com/juju/juju/domain/network"
	domainstorage "github.com/juju/juju/domain/storage"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	internalstorage "github.com/juju/juju/internal/storage"
)

// serviceSuite is a suite of tests for asserting the functionality on
// offer by the [Service].
type serviceSuite struct {
	poolProvider *MockStoragePoolProvider
	state        *MockState
}

func TestServiceSuite(t *testing.T) {
	tc.Run(t, &serviceSuite{})
}

func (s *serviceSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.state = NewMockState(ctrl)
	s.poolProvider = NewMockStoragePoolProvider(ctrl)
	c.Cleanup(func() {
		s.state = nil
		s.poolProvider = nil
	})
	return ctrl
}

// TestMakeUnitStorageArgs tests the makeUnitStorageArgs method of the
// [Service] as a happy path tests. This is a large test that asserts a
// complex composition of storage.
//
// This test wants to see that for 2 storage directives:
// - Existing storage is used first.
// - No new storage intances are created when existing storage is used.
// - Only new storage instances are assigned as owned.
// - Storage attachments are made on to the supplied net node uuid.
func (s *serviceSuite) TestMakeUnitStorageArgs(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	attachNetNodeUUID := tc.Must(c, domainnetwork.NewNetNodeUUID)
	poolUUID := tc.Must(c, domainstorage.NewStoragePoolUUID)
	storageDirectives := []internal.StorageDirective{
		{
			CharmMetadataName: "big-beautiful-charm",
			CharmStorageType:  charm.StorageFilesystem,
			Count:             3,
			MaxCount:          3,
			Name:              "st1",
			PoolUUID:          poolUUID,
			Size:              1024,
		},
		{
			CharmMetadataName: "big-beautiful-charm",
			CharmStorageType:  charm.StorageBlock,
			Count:             0,
			MaxCount:          3,
			Name:              "st2",
			PoolUUID:          poolUUID,
			Size:              1024,
		},
	}

	existingSt1Storage := []internal.StorageInstanceComposition{
		{
			Filesystem: &internal.StorageInstanceCompositionFilesystem{
				ProvisionScope: domainstorage.ProvisionScopeMachine,
				UUID:           tc.Must(c, domainstorage.NewFilesystemUUID),
			},
			StorageName: "st1",
			UUID:        tc.Must(c, domainstorage.NewStorageInstanceUUID),
		},
	}

	existingSt2Storage := []internal.StorageInstanceComposition{
		{
			StorageName: "st2",
			Volume: &internal.StorageInstanceCompositionVolume{
				ProvisionScope: domainstorage.ProvisionScopeMachine,
				UUID:           tc.Must(c, domainstorage.NewVolumeUUID),
			},
			UUID: tc.Must(c, domainstorage.NewStorageInstanceUUID),
		},
		{
			StorageName: "st2",
			Volume: &internal.StorageInstanceCompositionVolume{
				ProvisionScope: domainstorage.ProvisionScopeMachine,
				UUID:           tc.Must(c, domainstorage.NewVolumeUUID),
			},
			UUID: tc.Must(c, domainstorage.NewStorageInstanceUUID),
		},
	}

	provider := NewMockStorageProvider(ctrl)
	provider.EXPECT().Scope().Return(internalstorage.ScopeMachine).AnyTimes()
	provider.EXPECT().Supports(internalstorage.StorageKindFilesystem).Return(true).AnyTimes()
	provider.EXPECT().Supports(internalstorage.StorageKindBlock).Return(true).AnyTimes()
	s.poolProvider.EXPECT().GetProviderForPool(gomock.Any(), poolUUID).Return(
		provider, nil,
	).AnyTimes()

	svc := NewService(s.state, s.poolProvider, loggertesting.WrapCheckLog(c))

	arg, err := svc.MakeUnitStorageArgs(
		c.Context(),
		attachNetNodeUUID,
		storageDirectives,
		append(existingSt1Storage, existingSt2Storage...),
		nil,
	)
	c.Check(err, tc.IsNil)

	expectStorageDirectives := []domainstorage.DirectiveArg{
		{
			Count:    3,
			Name:     "st1",
			PoolUUID: poolUUID,
			Size:     1024,
		},
		{
			Count:    0,
			Name:     "st2",
			PoolUUID: poolUUID,
			Size:     1024,
		},
	}

	expectedStorageInstances := []domainstorage.CreateUnitStorageInstanceArg{
		{
			CharmName: "big-beautiful-charm",
			Filesystem: &domainstorage.CreateUnitStorageFilesystemArg{
				ProvisionScope: domainstorage.ProvisionScopeMachine,
			},
			Kind:            domainstorage.StorageKindFilesystem,
			Name:            "st1",
			RequestSizeMiB:  1024,
			StoragePoolUUID: poolUUID,
		},
		{
			CharmName: "big-beautiful-charm",
			Filesystem: &domainstorage.CreateUnitStorageFilesystemArg{
				ProvisionScope: domainstorage.ProvisionScopeMachine,
			},
			Kind:            domainstorage.StorageKindFilesystem,
			Name:            "st1",
			RequestSizeMiB:  1024,
			StoragePoolUUID: poolUUID,
		},
	}

	expectedStorageToAttach := []domainstorage.CreateUnitStorageAttachmentArg{
		// Existing st1 storage
		{
			FilesystemAttachment: &domainstorage.CreateUnitStorageFilesystemAttachmentArg{
				FilesystemUUID: existingSt1Storage[0].Filesystem.UUID,
				NetNodeUUID:    attachNetNodeUUID,
				ProvisionScope: existingSt1Storage[0].Filesystem.ProvisionScope,
			},
			StorageInstanceUUID: existingSt1Storage[0].UUID,
		},

		// Existing st2 storage
		{
			StorageInstanceUUID: existingSt2Storage[0].UUID,
			VolumeAttachment: &domainstorage.CreateUnitStorageVolumeAttachmentArg{
				NetNodeUUID:    attachNetNodeUUID,
				ProvisionScope: existingSt2Storage[0].Volume.ProvisionScope,
				VolumeUUID:     existingSt2Storage[0].Volume.UUID,
			},
		},
		{
			StorageInstanceUUID: existingSt2Storage[1].UUID,
			VolumeAttachment: &domainstorage.CreateUnitStorageVolumeAttachmentArg{
				NetNodeUUID:    attachNetNodeUUID,
				ProvisionScope: existingSt2Storage[1].Volume.ProvisionScope,
				VolumeUUID:     existingSt2Storage[1].Volume.UUID,
			},
		},
	}
	// Loop through the new storage instances being created and set their
	// attachment expectations.
	expectedStorageToAttach = slices.Grow(expectedStorageToAttach, len(arg.StorageInstances))
	for _, si := range arg.StorageInstances {
		attachArg := domainstorage.CreateUnitStorageAttachmentArg{
			StorageInstanceUUID: si.UUID,
		}

		if si.Filesystem != nil {
			attachArg.FilesystemAttachment =
				&domainstorage.CreateUnitStorageFilesystemAttachmentArg{
					FilesystemUUID: si.Filesystem.UUID,
					NetNodeUUID:    attachNetNodeUUID,
					ProvisionScope: si.Filesystem.ProvisionScope,
				}
		}
		if si.Volume != nil {
			attachArg.VolumeAttachment =
				&domainstorage.CreateUnitStorageVolumeAttachmentArg{
					VolumeUUID:     si.Volume.UUID,
					NetNodeUUID:    attachNetNodeUUID,
					ProvisionScope: si.Volume.ProvisionScope,
				}
		}
		expectedStorageToAttach = append(expectedStorageToAttach, attachArg)
	}

	expectedStorageToOwn := make([]domainstorage.StorageInstanceUUID, 0, len(arg.StorageInstances))
	for _, si := range arg.StorageInstances {
		expectedStorageToOwn = append(expectedStorageToOwn, si.UUID)
	}

	c.Check(arg, createUnitStorageArgChecker(), domainstorage.CreateUnitStorageArg{
		StorageDirectives: expectStorageDirectives,
		StorageInstances:  expectedStorageInstances,
		StorageToAttach:   expectedStorageToAttach,
		StorageToOwn:      expectedStorageToOwn,
	})
}

func (s *serviceSuite) TestMakeIAASUnitStorageArgs(c *tc.C) {
	defer s.setupMocks(c).Finish()

	fsUUID1 := tc.Must(c, domainstorage.NewFilesystemUUID)
	fsUUID2 := tc.Must(c, domainstorage.NewFilesystemUUID)
	volUUID1 := tc.Must(c, domainstorage.NewVolumeUUID)
	volUUID2 := tc.Must(c, domainstorage.NewVolumeUUID)

	expectedStorageInstances := []domainstorage.CreateUnitStorageInstanceArg{
		{
			Filesystem: &domainstorage.CreateUnitStorageFilesystemArg{
				UUID:           tc.Must(c, domainstorage.NewFilesystemUUID),
				ProvisionScope: domainstorage.ProvisionScopeMachine,
			},
			Volume: &domainstorage.CreateUnitStorageVolumeArg{
				UUID:           tc.Must(c, domainstorage.NewVolumeUUID),
				ProvisionScope: domainstorage.ProvisionScopeModel,
			},
		},
		{
			Filesystem: &domainstorage.CreateUnitStorageFilesystemArg{
				UUID:           tc.Must(c, domainstorage.NewFilesystemUUID),
				ProvisionScope: domainstorage.ProvisionScopeModel,
			},
		},
		{
			Volume: &domainstorage.CreateUnitStorageVolumeArg{
				UUID:           tc.Must(c, domainstorage.NewVolumeUUID),
				ProvisionScope: domainstorage.ProvisionScopeModel,
			},
		},
		{
			Filesystem: &domainstorage.CreateUnitStorageFilesystemArg{
				UUID:           fsUUID1,
				ProvisionScope: domainstorage.ProvisionScopeMachine,
			},
		},
		{
			Volume: &domainstorage.CreateUnitStorageVolumeArg{
				UUID:           volUUID1,
				ProvisionScope: domainstorage.ProvisionScopeMachine,
			},
		},
		{
			Filesystem: &domainstorage.CreateUnitStorageFilesystemArg{
				UUID:           fsUUID2,
				ProvisionScope: domainstorage.ProvisionScopeMachine,
			},
			Volume: &domainstorage.CreateUnitStorageVolumeArg{
				UUID:           volUUID2,
				ProvisionScope: domainstorage.ProvisionScopeMachine,
			},
		},
	}

	svc := NewService(s.state, s.poolProvider, loggertesting.WrapCheckLog(c))
	arg, err := svc.MakeIAASUnitStorageArgs(c.Context(), expectedStorageInstances)
	c.Assert(err, tc.IsNil)
	c.Check(arg.FilesystemsToOwn, tc.SameContents,
		[]domainstorage.FilesystemUUID{
			fsUUID1,
			fsUUID2,
		},
	)
	c.Check(arg.VolumesToOwn, tc.SameContents,
		[]domainstorage.VolumeUUID{
			volUUID1,
			volUUID2,
		},
	)
}

func (s *serviceSuite) TestMakeUnitAddStorageArgs(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	attachNetNodeUUID := tc.Must(c, domainnetwork.NewNetNodeUUID)
	poolUUID := tc.Must(c, domainstorage.NewStoragePoolUUID)
	unitUUID := tc.Must(c, coreunit.NewUUID)
	storageDirective := internal.StorageDirective{
		CharmMetadataName: "big-beautiful-charm",
		CharmStorageType:  charm.StorageFilesystem,
		MaxCount:          3,
		Name:              "st1",
		PoolUUID:          poolUUID,
		Size:              1024,
	}

	provider := NewMockStorageProvider(ctrl)
	provider.EXPECT().Scope().Return(internalstorage.ScopeMachine).AnyTimes()
	provider.EXPECT().Supports(internalstorage.StorageKindFilesystem).Return(true).AnyTimes()
	provider.EXPECT().Supports(internalstorage.StorageKindBlock).Return(true).AnyTimes()
	s.poolProvider.EXPECT().GetProviderForPool(gomock.Any(), poolUUID).Return(
		provider, nil,
	).AnyTimes()

	s.state.EXPECT().GetUnitNetNodeUUID(gomock.Any(), unitUUID).Return(attachNetNodeUUID.String(), nil)

	svc := NewService(s.state, s.poolProvider, loggertesting.WrapCheckLog(c))

	arg, err := svc.MakeUnitAddStorageArgs(
		c.Context(),
		unitUUID,
		2,
		storageDirective,
	)
	c.Check(err, tc.ErrorIsNil)

	expectedStorageInstances := []domainstorage.CreateUnitStorageInstanceArg{
		{
			CharmName: "big-beautiful-charm",
			Filesystem: &domainstorage.CreateUnitStorageFilesystemArg{
				ProvisionScope: domainstorage.ProvisionScopeMachine,
			},
			Kind:            domainstorage.StorageKindFilesystem,
			Name:            "st1",
			RequestSizeMiB:  1024,
			StoragePoolUUID: poolUUID,
		},
		{
			CharmName: "big-beautiful-charm",
			Filesystem: &domainstorage.CreateUnitStorageFilesystemArg{
				ProvisionScope: domainstorage.ProvisionScopeMachine,
			},
			Kind:            domainstorage.StorageKindFilesystem,
			Name:            "st1",
			RequestSizeMiB:  1024,
			StoragePoolUUID: poolUUID,
		},
	}

	expectedStorageToAttach := make(
		[]domainstorage.CreateUnitStorageAttachmentArg,
		0,
		len(arg.StorageInstances),
	)
	// Loop through the new storage instances being created and set their
	// attachment expectations.
	for _, si := range arg.StorageInstances {
		attachArg := domainstorage.CreateUnitStorageAttachmentArg{
			StorageInstanceUUID: si.UUID,
		}

		if si.Filesystem != nil {
			attachArg.FilesystemAttachment =
				&domainstorage.CreateUnitStorageFilesystemAttachmentArg{
					FilesystemUUID: si.Filesystem.UUID,
					NetNodeUUID:    attachNetNodeUUID,
					ProvisionScope: si.Filesystem.ProvisionScope,
				}
		}
		if si.Volume != nil {
			attachArg.VolumeAttachment =
				&domainstorage.CreateUnitStorageVolumeAttachmentArg{
					VolumeUUID:     si.Volume.UUID,
					NetNodeUUID:    attachNetNodeUUID,
					ProvisionScope: si.Volume.ProvisionScope,
				}
		}
		expectedStorageToAttach = append(expectedStorageToAttach, attachArg)
	}

	expectedStorageToOwn := make(
		[]domainstorage.StorageInstanceUUID, 0, len(arg.StorageInstances))
	for _, si := range arg.StorageInstances {
		expectedStorageToOwn = append(expectedStorageToOwn, si.UUID)
	}

	mc := tc.NewMultiChecker()
	mc.AddExpr("_.StorageToAttach[_].UUID", tc.IsNonZeroUUID)
	mc.AddExpr("_.StorageToAttach[_].FilesystemAttachment.UUID", tc.IsNonZeroUUID)
	mc.AddExpr("_.StorageToAttach[_].VolumeAttachment.UUID", tc.IsNonZeroUUID)
	mc.AddExpr("_.StorageInstances[_].UUID", tc.IsNonZeroUUID)
	mc.AddExpr("_.StorageInstances[_].Volume.UUID", tc.IsNonZeroUUID)
	mc.AddExpr("_.StorageInstances[_].Filesystem.UUID", tc.IsNonZeroUUID)
	c.Check(arg, mc, domainstorage.UnitAddStorageArg{
		StorageInstances: expectedStorageInstances,
		StorageToAttach:  expectedStorageToAttach,
		StorageToOwn:     expectedStorageToOwn,
	})
}

// TestMakeIAASSubordinateUnitStorageArgs tests the
// MakeIAASSubordinateUnitStorageArgs method of the [Service]. It wants to
// see that the storage arguments for a subordinate unit are made from the
// storage directives of the subordinate application, with the storage
// instances attached to the net node of the machine hosting the principal
// unit, and that the machine scoped storage entities are owned by the
// machine.
func (s *serviceSuite) TestMakeIAASSubordinateUnitStorageArgs(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	appUUID := tc.Must(c, coreapplication.NewUUID)
	machineNetNodeUUID := tc.Must(c, domainnetwork.NewNetNodeUUID)
	poolUUID := tc.Must(c, domainstorage.NewStoragePoolUUID)
	storageDirectives := []internal.StorageDirective{
		{
			CharmMetadataName: "sub-charm",
			CharmStorageType:  charm.StorageFilesystem,
			Count:             1,
			MaxCount:          charm.StorageNoMaxCount,
			Name:              "data",
			PoolUUID:          poolUUID,
			Size:              1024,
		},
	}

	s.state.EXPECT().GetApplicationStorageDirectives(gomock.Any(), appUUID).
		Return(storageDirectives, nil)

	provider := NewMockStorageProvider(ctrl)
	provider.EXPECT().Scope().Return(internalstorage.ScopeMachine).AnyTimes()
	provider.EXPECT().Supports(internalstorage.StorageKindFilesystem).Return(true).AnyTimes()
	s.poolProvider.EXPECT().GetProviderForPool(gomock.Any(), poolUUID).Return(
		provider, nil,
	).AnyTimes()

	svc := NewService(s.state, s.poolProvider, loggertesting.WrapCheckLog(c))

	unitStorageArgs, iaasUnitStorageArgs, err := svc.MakeIAASSubordinateUnitStorageArgs(
		c.Context(),
		appUUID,
		machineNetNodeUUID,
	)
	c.Assert(err, tc.ErrorIsNil)

	// The storage directive is passed through to the unit storage arguments.
	c.Check(unitStorageArgs.StorageDirectives, tc.DeepEquals, []domainstorage.DirectiveArg{
		{
			Count:    1,
			Name:     "data",
			PoolUUID: poolUUID,
			Size:     1024,
		},
	})

	// A single new storage instance is created for the subordinate unit, and
	// is attached to the machine net node of the principal unit.
	c.Check(unitStorageArgs.StorageInstances, tc.HasLen, 1)
	c.Check(unitStorageArgs.StorageInstances[0].Name, tc.Equals, domainstorage.Name("data"))
	c.Check(unitStorageArgs.StorageInstances[0].CharmName, tc.Equals, "sub-charm")
	c.Check(unitStorageArgs.StorageInstances[0].Kind, tc.Equals, domainstorage.StorageKindFilesystem)
	c.Check(unitStorageArgs.StorageInstances[0].RequestSizeMiB, tc.Equals, uint64(1024))
	c.Check(unitStorageArgs.StorageInstances[0].StoragePoolUUID, tc.Equals, poolUUID)
	c.Check(unitStorageArgs.StorageToAttach, tc.HasLen, 1)
	c.Check(unitStorageArgs.StorageToAttach[0].StorageInstanceUUID, tc.Equals, unitStorageArgs.StorageInstances[0].UUID)
	c.Check(unitStorageArgs.StorageToAttach[0].FilesystemAttachment, tc.NotNil)
	if filesystemAttachment := unitStorageArgs.StorageToAttach[0].FilesystemAttachment; filesystemAttachment != nil {
		c.Check(filesystemAttachment.NetNodeUUID, tc.Equals, machineNetNodeUUID)
		c.Check(filesystemAttachment.ProvisionScope, tc.Equals, domainstorage.ProvisionScopeMachine)
	}
	c.Check(unitStorageArgs.StorageToOwn, tc.DeepEquals, []domainstorage.StorageInstanceUUID{
		unitStorageArgs.StorageInstances[0].UUID,
	})

	// The machine scoped filesystem is owned by the subordinate unit's
	// machine.
	c.Check(iaasUnitStorageArgs.FilesystemsToOwn, tc.DeepEquals, []domainstorage.FilesystemUUID{
		unitStorageArgs.StorageInstances[0].Filesystem.UUID,
	})
	c.Check(iaasUnitStorageArgs.VolumesToOwn, tc.IsNil)
}

// TestMakeIAASSubordinateUnitStorageArgsApplicationNotFound tests that an
// error making the storage arguments for a subordinate unit of an
// application that does not exist is returned.
func (s *serviceSuite) TestMakeIAASSubordinateUnitStorageArgsApplicationNotFound(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	appUUID := tc.Must(c, coreapplication.NewUUID)
	machineNetNodeUUID := tc.Must(c, domainnetwork.NewNetNodeUUID)
	expectedError := applicationerrors.ApplicationNotFound
	s.state.EXPECT().GetApplicationStorageDirectives(gomock.Any(), appUUID).
		Return(nil, expectedError)

	svc := NewService(s.state, s.poolProvider, loggertesting.WrapCheckLog(c))

	_, _, err := svc.MakeIAASSubordinateUnitStorageArgs(c.Context(), appUUID, machineNetNodeUUID)
	c.Assert(err, tc.ErrorIs, expectedError)
}

// TestMakeIAASSubordinateUnitStorageArgsNoStorageDirectives tests that
// making the storage arguments for a subordinate unit of an application
// without storage directives returns zero-value arguments, and does not
// block subordinate unit creation for charms without storage.
func (s *serviceSuite) TestMakeIAASSubordinateUnitStorageArgsNoStorageDirectives(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	appUUID := tc.Must(c, coreapplication.NewUUID)
	machineNetNodeUUID := tc.Must(c, domainnetwork.NewNetNodeUUID)

	s.state.EXPECT().GetApplicationStorageDirectives(gomock.Any(), appUUID).
		Return(nil, nil)

	svc := NewService(s.state, s.poolProvider, loggertesting.WrapCheckLog(c))

	unitStorageArgs, iaasUnitStorageArgs, err := svc.MakeIAASSubordinateUnitStorageArgs(
		c.Context(),
		appUUID,
		machineNetNodeUUID,
	)
	c.Assert(err, tc.ErrorIsNil)

	// No storage is provisioned for an application without storage
	// directives.
	c.Check(unitStorageArgs.StorageDirectives, tc.HasLen, 0)
	c.Check(unitStorageArgs.StorageInstances, tc.HasLen, 0)
	c.Check(unitStorageArgs.StorageToAttach, tc.HasLen, 0)
	c.Check(unitStorageArgs.StorageToOwn, tc.HasLen, 0)
	c.Check(iaasUnitStorageArgs, tc.DeepEquals, domainstorage.CreateIAASUnitStorageArg{})
}

// TestMakeIAASSubordinateUnitStorageArgsVolume tests that a block storage
// directive results in a block storage instance with its volume attached
// to the machine net node of the principal unit, and the machine owning
// the volume.
func (s *serviceSuite) TestMakeIAASSubordinateUnitStorageArgsVolume(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	appUUID := tc.Must(c, coreapplication.NewUUID)
	machineNetNodeUUID := tc.Must(c, domainnetwork.NewNetNodeUUID)
	poolUUID := tc.Must(c, domainstorage.NewStoragePoolUUID)
	storageDirectives := []internal.StorageDirective{
		{
			CharmMetadataName: "sub-charm",
			CharmStorageType:  charm.StorageBlock,
			Count:             1,
			MaxCount:          charm.StorageNoMaxCount,
			Name:              "data",
			PoolUUID:          poolUUID,
			Size:              1024,
		},
	}

	s.state.EXPECT().GetApplicationStorageDirectives(gomock.Any(), appUUID).
		Return(storageDirectives, nil)

	provider := NewMockStorageProvider(ctrl)
	provider.EXPECT().Scope().Return(internalstorage.ScopeMachine).AnyTimes()
	provider.EXPECT().Supports(internalstorage.StorageKindBlock).Return(true).AnyTimes()
	s.poolProvider.EXPECT().GetProviderForPool(gomock.Any(), poolUUID).Return(
		provider, nil,
	).AnyTimes()

	svc := NewService(s.state, s.poolProvider, loggertesting.WrapCheckLog(c))

	unitStorageArgs, iaasUnitStorageArgs, err := svc.MakeIAASSubordinateUnitStorageArgs(
		c.Context(),
		appUUID,
		machineNetNodeUUID,
	)
	c.Assert(err, tc.ErrorIsNil)

	// A single new block storage instance is created for the subordinate
	// unit.
	c.Assert(unitStorageArgs.StorageInstances, tc.HasLen, 1)
	instance := unitStorageArgs.StorageInstances[0]
	c.Check(instance.CharmName, tc.Equals, "sub-charm")
	c.Check(instance.Kind, tc.Equals, domainstorage.StorageKindBlock)
	c.Assert(instance.Volume, tc.NotNil)
	c.Check(instance.Volume.ProvisionScope, tc.Equals, domainstorage.ProvisionScopeMachine)

	// The volume is attached to the machine net node of the principal unit.
	c.Assert(unitStorageArgs.StorageToAttach, tc.HasLen, 1)
	volAttachment := unitStorageArgs.StorageToAttach[0].VolumeAttachment
	c.Assert(volAttachment, tc.NotNil)
	c.Check(volAttachment.VolumeUUID, tc.Equals, instance.Volume.UUID)
	c.Check(volAttachment.NetNodeUUID, tc.Equals, machineNetNodeUUID)
	c.Check(volAttachment.ProvisionScope, tc.Equals, domainstorage.ProvisionScopeMachine)

	// The unit owns the storage instance, and the machine owns the volume.
	c.Check(unitStorageArgs.StorageToOwn, tc.DeepEquals, []domainstorage.StorageInstanceUUID{
		instance.UUID,
	})
	c.Check(iaasUnitStorageArgs.VolumesToOwn, tc.DeepEquals, []domainstorage.VolumeUUID{
		instance.Volume.UUID,
	})
	c.Check(iaasUnitStorageArgs.FilesystemsToOwn, tc.IsNil)
}

// TestMakeIAASSubordinateUnitStorageArgsPoolProviderError tests that an
// error looking up the storage provider for a pool is returned when making
// the storage arguments for a subordinate unit.
func (s *serviceSuite) TestMakeIAASSubordinateUnitStorageArgsPoolProviderError(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	appUUID := tc.Must(c, coreapplication.NewUUID)
	machineNetNodeUUID := tc.Must(c, domainnetwork.NewNetNodeUUID)
	poolUUID := tc.Must(c, domainstorage.NewStoragePoolUUID)
	storageDirectives := []internal.StorageDirective{
		{
			CharmMetadataName: "sub-charm",
			CharmStorageType:  charm.StorageFilesystem,
			Count:             1,
			MaxCount:          charm.StorageNoMaxCount,
			Name:              "data",
			PoolUUID:          poolUUID,
			Size:              1024,
		},
	}
	s.state.EXPECT().GetApplicationStorageDirectives(gomock.Any(), appUUID).
		Return(storageDirectives, nil)

	expectedError := errors.New("boom")
	s.poolProvider.EXPECT().GetProviderForPool(gomock.Any(), poolUUID).
		Return(nil, expectedError)

	svc := NewService(s.state, s.poolProvider, loggertesting.WrapCheckLog(c))

	_, _, err := svc.MakeIAASSubordinateUnitStorageArgs(c.Context(), appUUID, machineNetNodeUUID)
	c.Assert(err, tc.ErrorIs, expectedError)
}

// TestMakeIAASSubordinateUnitStorageArgsNotValid tests that invalid
// arguments are rejected when making the storage arguments for a
// subordinate unit.
func (s *serviceSuite) TestMakeIAASSubordinateUnitStorageArgsNotValid(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	svc := NewService(s.state, s.poolProvider, loggertesting.WrapCheckLog(c))

	machineNetNodeUUID := tc.Must(c, domainnetwork.NewNetNodeUUID)
	_, _, err := svc.MakeIAASSubordinateUnitStorageArgs(c.Context(), "", machineNetNodeUUID)
	c.Check(err, tc.ErrorIs, coreerrors.NotValid)

	appUUID := tc.Must(c, coreapplication.NewUUID)
	_, _, err = svc.MakeIAASSubordinateUnitStorageArgs(c.Context(), appUUID, "")
	c.Check(err, tc.ErrorIs, coreerrors.NotValid)
}

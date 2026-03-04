// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"slices"
	"testing"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/application"
	"github.com/juju/juju/domain/application/charm"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/application/internal"
	internalcharm "github.com/juju/juju/domain/deployment/charm"
	domainnetwork "github.com/juju/juju/domain/network"
	domainstorage "github.com/juju/juju/domain/storage"
	domainstorageprov "github.com/juju/juju/domain/storageprovisioning"
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
	storageDirectives := []application.StorageDirective{
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
				ProvisionScope: domainstorageprov.ProvisionScopeMachine,
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
				ProvisionScope: domainstorageprov.ProvisionScopeMachine,
				UUID:           tc.Must(c, domainstorage.NewVolumeUUID),
			},
			UUID: tc.Must(c, domainstorage.NewStorageInstanceUUID),
		},
		{
			StorageName: "st2",
			Volume: &internal.StorageInstanceCompositionVolume{
				ProvisionScope: domainstorageprov.ProvisionScopeMachine,
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

	expectStorageDirectives := []internal.CreateUnitStorageDirectiveArg{
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

	expectedStorageInstances := []internal.CreateUnitStorageInstanceArg{
		{
			CharmName: "big-beautiful-charm",
			Filesystem: &internal.CreateUnitStorageFilesystemArg{
				ProvisionScope: domainstorageprov.ProvisionScopeMachine,
			},
			Kind:            domainstorage.StorageKindFilesystem,
			Name:            "st1",
			RequestSizeMiB:  1024,
			StoragePoolUUID: poolUUID,
		},
		{
			CharmName: "big-beautiful-charm",
			Filesystem: &internal.CreateUnitStorageFilesystemArg{
				ProvisionScope: domainstorageprov.ProvisionScopeMachine,
			},
			Kind:            domainstorage.StorageKindFilesystem,
			Name:            "st1",
			RequestSizeMiB:  1024,
			StoragePoolUUID: poolUUID,
		},
	}

	expectedStorageToAttach := []internal.CreateUnitStorageAttachmentArg{
		// Existing st1 storage
		{
			FilesystemAttachment: &internal.CreateUnitStorageFilesystemAttachmentArg{
				FilesystemUUID: existingSt1Storage[0].Filesystem.UUID,
				NetNodeUUID:    attachNetNodeUUID,
				ProvisionScope: existingSt1Storage[0].Filesystem.ProvisionScope,
			},
			StorageInstanceUUID: existingSt1Storage[0].UUID,
		},

		// Existing st2 storage
		{
			StorageInstanceUUID: existingSt2Storage[0].UUID,
			VolumeAttachment: &internal.CreateUnitStorageVolumeAttachmentArg{
				NetNodeUUID:    attachNetNodeUUID,
				ProvisionScope: existingSt2Storage[0].Volume.ProvisionScope,
				VolumeUUID:     existingSt2Storage[0].Volume.UUID,
			},
		},
		{
			StorageInstanceUUID: existingSt2Storage[1].UUID,
			VolumeAttachment: &internal.CreateUnitStorageVolumeAttachmentArg{
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
		attachArg := internal.CreateUnitStorageAttachmentArg{
			StorageInstanceUUID: si.UUID,
		}

		if si.Filesystem != nil {
			attachArg.FilesystemAttachment =
				&internal.CreateUnitStorageFilesystemAttachmentArg{
					FilesystemUUID: si.Filesystem.UUID,
					NetNodeUUID:    attachNetNodeUUID,
					ProvisionScope: si.Filesystem.ProvisionScope,
				}
		}
		if si.Volume != nil {
			attachArg.VolumeAttachment =
				&internal.CreateUnitStorageVolumeAttachmentArg{
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

	c.Check(arg, createUnitStorageArgChecker(), internal.CreateUnitStorageArg{
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

	expectedStorageInstances := []internal.AddStorageInstanceArg{
		{
			Filesystem: &internal.CreateUnitStorageFilesystemArg{
				UUID:           tc.Must(c, domainstorage.NewFilesystemUUID),
				ProvisionScope: domainstorageprov.ProvisionScopeMachine,
			},
			Volume: &internal.CreateUnitStorageVolumeArg{
				UUID:           tc.Must(c, domainstorage.NewVolumeUUID),
				ProvisionScope: domainstorageprov.ProvisionScopeModel,
			},
		},
		{
			Filesystem: &internal.CreateUnitStorageFilesystemArg{
				UUID:           tc.Must(c, domainstorage.NewFilesystemUUID),
				ProvisionScope: domainstorageprov.ProvisionScopeModel,
			},
		},
		{
			Volume: &internal.CreateUnitStorageVolumeArg{
				UUID:           tc.Must(c, domainstorage.NewVolumeUUID),
				ProvisionScope: domainstorageprov.ProvisionScopeModel,
			},
		},
		{
			Filesystem: &internal.CreateUnitStorageFilesystemArg{
				UUID:           fsUUID1,
				ProvisionScope: domainstorageprov.ProvisionScopeMachine,
			},
		},
		{
			Volume: &internal.CreateUnitStorageVolumeArg{
				UUID:           volUUID1,
				ProvisionScope: domainstorageprov.ProvisionScopeMachine,
			},
		},
		{
			Filesystem: &internal.CreateUnitStorageFilesystemArg{
				UUID:           fsUUID2,
				ProvisionScope: domainstorageprov.ProvisionScopeMachine,
			},
			Volume: &internal.CreateUnitStorageVolumeArg{
				UUID:           volUUID2,
				ProvisionScope: domainstorageprov.ProvisionScopeMachine,
			},
		},
	}

	svc := NewService(s.state, s.poolProvider, loggertesting.WrapCheckLog(c))
	arg, err := svc.MakeIAASUnitStorageArgs(expectedStorageInstances)
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
	storageDirective := application.StorageDirective{
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

	expectedStorageInstances := []internal.CreateUnitStorageInstanceArg{
		{
			CharmName: "big-beautiful-charm",
			Filesystem: &internal.CreateUnitStorageFilesystemArg{
				ProvisionScope: domainstorageprov.ProvisionScopeMachine,
			},
			Kind:            domainstorage.StorageKindFilesystem,
			Name:            "st1",
			RequestSizeMiB:  1024,
			StoragePoolUUID: poolUUID,
		},
		{
			CharmName: "big-beautiful-charm",
			Filesystem: &internal.CreateUnitStorageFilesystemArg{
				ProvisionScope: domainstorageprov.ProvisionScopeMachine,
			},
			Kind:            domainstorage.StorageKindFilesystem,
			Name:            "st1",
			RequestSizeMiB:  1024,
			StoragePoolUUID: poolUUID,
		},
	}

	expectedStorageToAttach := make([]internal.CreateUnitStorageAttachmentArg, 0, len(arg.StorageInstances))
	// Loop through the new storage instances being created and set their
	// attachment expectations.
	expectedStorageToAttach = slices.Grow(expectedStorageToAttach, len(arg.StorageInstances))
	for _, si := range arg.StorageInstances {
		attachArg := internal.CreateUnitStorageAttachmentArg{
			StorageInstanceUUID: si.UUID,
		}

		if si.Filesystem != nil {
			attachArg.FilesystemAttachment =
				&internal.CreateUnitStorageFilesystemAttachmentArg{
					FilesystemUUID: si.Filesystem.UUID,
					NetNodeUUID:    attachNetNodeUUID,
					ProvisionScope: si.Filesystem.ProvisionScope,
				}
		}
		if si.Volume != nil {
			attachArg.VolumeAttachment =
				&internal.CreateUnitStorageVolumeAttachmentArg{
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

	c.Check(arg, createUnitStorageArgChecker(), internal.AddStorageToUnitArg{
		StorageInstances: expectedStorageInstances,
		StorageToAttach:  expectedStorageToAttach,
		StorageToOwn:     expectedStorageToOwn,
	})
}

func (s *serviceSuite) TestMakeUnitAttachStorageArgs(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	attachNetNodeUUID := tc.Must(c, domainnetwork.NewNetNodeUUID)
	storageUUID := tc.Must(c, domainstorage.NewStorageInstanceUUID)
	instComposition := internal.StorageInstanceComposition{
		UUID: storageUUID,
		Filesystem: &internal.StorageInstanceCompositionFilesystem{
			UUID:           tc.Must(c, domainstorage.NewFilesystemUUID),
			ProvisionScope: domainstorageprov.ProvisionScopeMachine,
		},
		Volume: &internal.StorageInstanceCompositionVolume{
			UUID:           tc.Must(c, domainstorage.NewVolumeUUID),
			ProvisionScope: domainstorageprov.ProvisionScopeMachine,
		},
	}

	provider := NewMockStorageProvider(ctrl)
	provider.EXPECT().Scope().Return(internalstorage.ScopeMachine).AnyTimes()
	provider.EXPECT().Supports(internalstorage.StorageKindFilesystem).Return(true).AnyTimes()
	provider.EXPECT().Supports(internalstorage.StorageKindBlock).Return(true).AnyTimes()

	s.state.EXPECT().GetStorageInstanceCompositionByUUID(gomock.Any(), storageUUID).Return(instComposition, nil)

	svc := NewService(s.state, s.poolProvider, loggertesting.WrapCheckLog(c))

	attachArg, err := svc.MakeUnitAttachStorageArgs(
		c.Context(),
		attachNetNodeUUID.String(),
		storageUUID,
	)
	c.Check(err, tc.ErrorIsNil)

	expectedStorageToAttach := internal.CreateUnitStorageAttachmentArg{
		StorageInstanceUUID: instComposition.UUID,
	}
	if instComposition.Filesystem != nil {
		expectedStorageToAttach.FilesystemAttachment =
			&internal.CreateUnitStorageFilesystemAttachmentArg{
				FilesystemUUID: instComposition.Filesystem.UUID,
				NetNodeUUID:    attachNetNodeUUID,
				ProvisionScope: instComposition.Filesystem.ProvisionScope,
			}
	}
	if instComposition.Volume != nil {
		expectedStorageToAttach.VolumeAttachment =
			&internal.CreateUnitStorageVolumeAttachmentArg{
				VolumeUUID:     instComposition.Volume.UUID,
				NetNodeUUID:    attachNetNodeUUID,
				ProvisionScope: instComposition.Volume.ProvisionScope,
			}
	}

	c.Assert(attachArg.UUID, tc.Not(tc.Equals), "")
	attachArg.UUID = ""
	if instComposition.Filesystem != nil {
		c.Assert(attachArg.FilesystemAttachment.UUID, tc.Not(tc.Equals), "")
		attachArg.FilesystemAttachment.UUID = ""
	}
	if instComposition.Volume != nil {
		c.Assert(attachArg.VolumeAttachment.UUID, tc.Not(tc.Equals), "")
		attachArg.VolumeAttachment.UUID = ""
	}
	arg := internal.AttachStorageToUnitArg{
		StorageToAttach: attachArg,
	}
	c.Check(arg, tc.DeepEquals, internal.AttachStorageToUnitArg{
		StorageToAttach: expectedStorageToAttach,
	})
}

func (s *serviceSuite) TestValidateAttachStorageExceedMax(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	charmStorageDef := internal.ValidateStorageArg{
		CountMin:    0,
		CountMax:    2,
		Name:        "st1",
		MinimumSize: 1024,
		Type:        internalcharm.StorageBlock,
	}

	svc := NewService(s.state, s.poolProvider, loggertesting.WrapCheckLog(c))
	err := svc.ValidateAttachStorage(
		charmStorageDef, 2, 1024,
	)

	errVal, is := errors.AsType[applicationerrors.StorageCountLimitExceeded](err)
	c.Check(is, tc.IsTrue)
	c.Check(errVal, tc.DeepEquals, applicationerrors.StorageCountLimitExceeded{
		Maximum:     ptr(2),
		Minimum:     0,
		Requested:   3,
		StorageName: "st1",
	})
}

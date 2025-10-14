// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"slices"
	"testing"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/domain/application"
	"github.com/juju/juju/domain/application/charm"
	"github.com/juju/juju/domain/application/internal"
	domainnetwork "github.com/juju/juju/domain/network"
	domainstorage "github.com/juju/juju/domain/storage"
	domainstorageprov "github.com/juju/juju/domain/storageprovisioning"
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
				UUID:           tc.Must(c, domainstorageprov.NewFilesystemUUID),
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
				UUID:           tc.Must(c, domainstorageprov.NewVolumeUUID),
			},
			UUID: tc.Must(c, domainstorage.NewStorageInstanceUUID),
		},
		{
			StorageName: "st2",
			Volume: &internal.StorageInstanceCompositionVolume{
				ProvisionScope: domainstorageprov.ProvisionScopeMachine,
				UUID:           tc.Must(c, domainstorageprov.NewVolumeUUID),
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

	svc := NewService(s.state, s.poolProvider)

	arg, err := svc.MakeUnitStorageArgs(
		c.Context(),
		attachNetNodeUUID,
		storageDirectives,
		append(existingSt1Storage, existingSt2Storage...),
	)
	c.Check(err, tc.IsNil)

	expectStorageDirectives := []application.CreateUnitStorageDirectiveArg{
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

	expectedStorageInstances := []application.CreateUnitStorageInstanceArg{
		{
			CharmName: "big-beautiful-charm",
			Filesystem: &application.CreateUnitStorageFilesystemArg{
				ProvisionScope: domainstorageprov.ProvisionScopeMachine,
			},
			Kind:            domainstorage.StorageKindFilesystem,
			Name:            "st1",
			RequestSizeMiB:  1024,
			StoragePoolUUID: poolUUID,
		},
		{
			CharmName: "big-beautiful-charm",
			Filesystem: &application.CreateUnitStorageFilesystemArg{
				ProvisionScope: domainstorageprov.ProvisionScopeMachine,
			},
			Kind:            domainstorage.StorageKindFilesystem,
			Name:            "st1",
			RequestSizeMiB:  1024,
			StoragePoolUUID: poolUUID,
		},
	}

	expectedStorageToAttach := []application.CreateUnitStorageAttachmentArg{
		// Existing st1 storage
		{
			FilesystemAttachment: &application.CreateUnitStorageFilesystemAttachmentArg{
				FilesystemUUID: existingSt1Storage[0].Filesystem.UUID,
				NetNodeUUID:    attachNetNodeUUID,
				ProvisionScope: existingSt1Storage[0].Filesystem.ProvisionScope,
			},
			StorageInstanceUUID: existingSt1Storage[0].UUID,
		},

		// Existing st2 storage
		{
			StorageInstanceUUID: existingSt2Storage[0].UUID,
			VolumeAttachment: &application.CreateUnitStorageVolumeAttachmentArg{
				NetNodeUUID:    attachNetNodeUUID,
				ProvisionScope: existingSt2Storage[0].Volume.ProvisionScope,
				VolumeUUID:     existingSt2Storage[0].Volume.UUID,
			},
		},
		{
			StorageInstanceUUID: existingSt2Storage[1].UUID,
			VolumeAttachment: &application.CreateUnitStorageVolumeAttachmentArg{
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
		attachArg := application.CreateUnitStorageAttachmentArg{
			StorageInstanceUUID: si.UUID,
		}

		if si.Filesystem != nil {
			attachArg.FilesystemAttachment =
				&application.CreateUnitStorageFilesystemAttachmentArg{
					FilesystemUUID: si.Filesystem.UUID,
					NetNodeUUID:    attachNetNodeUUID,
					ProvisionScope: si.Filesystem.ProvisionScope,
				}
		}
		if si.Volume != nil {
			attachArg.VolumeAttachment =
				&application.CreateUnitStorageVolumeAttachmentArg{
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

	c.Check(arg, createUnitStorageArgChecker(), application.CreateUnitStorageArg{
		StorageDirectives: expectStorageDirectives,
		StorageInstances:  expectedStorageInstances,
		StorageToAttach:   expectedStorageToAttach,
		StorageToOwn:      expectedStorageToOwn,
	})
}

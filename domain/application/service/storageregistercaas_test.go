// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"testing"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	caas "github.com/juju/juju/caas"
	coreapplication "github.com/juju/juju/core/application"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/application"
	"github.com/juju/juju/domain/application/charm"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/application/internal"
	domainnetwork "github.com/juju/juju/domain/network"
	domainstorage "github.com/juju/juju/domain/storage"
	domainstorageprov "github.com/juju/juju/domain/storageprovisioning"
	internalstorage "github.com/juju/juju/internal/storage"
)

// registerCAASStorageSuite is a suite tests concerned with testing
// functionality around storage registration for caas units.
type registerCAASStorageSuite struct {
	state        *MockStorageState
	poolProvider *MockStoragePoolProvider
}

func TestRegisterCAASStorageSuite(t *testing.T) {
	tc.Run(t, &registerCAASStorageSuite{})
}

func (s *registerCAASStorageSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.state = NewMockStorageState(ctrl)
	s.poolProvider = NewMockStoragePoolProvider(ctrl)
	c.Cleanup(func() {
		s.state = nil
		s.poolProvider = nil
	})
	return ctrl
}

func (s *registerCAASStorageSuite) newFilesystemStoragePool(
	c *tc.C,
	ctrl *gomock.Controller,
) domainstorage.StoragePoolUUID {
	poolUUID := tc.Must(c, domainstorage.NewStoragePoolUUID)
	provider := NewMockStorageProvider(ctrl)

	provider.EXPECT().Scope().Return(internalstorage.ScopeEnviron).AnyTimes()
	provider.EXPECT().Supports(internalstorage.StorageKindFilesystem).Return(true).AnyTimes()
	s.poolProvider.EXPECT().GetProviderForPool(gomock.Any(), poolUUID).Return(
		provider, nil,
	).AnyTimes()

	return poolUUID
}

// TestGetRegisterCAASUnitStorageArgNewUnit tests the happy path of seeing new
// storage instances created for a unit that does not exist in the model yet.
//
// This test asserts the following preconditions:
// - The unit does not exist in the model.
// - No existing provider storage exists in the model.
func (s *registerCAASStorageSuite) TestGetRegisterCAASUnitStorageArgNewUnit(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	appUUID := tc.Must(c, coreapplication.NewID)
	unitUUID := tc.Must(c, coreunit.NewUUID)
	poolUUID := s.newFilesystemStoragePool(c, ctrl)
	attachNetNodeUUID := tc.Must(c, domainnetwork.NewNetNodeUUID)
	providerFSInfo := []caas.FilesystemInfo{
		{
			FilesystemId: "fs-1",
			StorageName:  "st1",
		},
	}

	s.state.EXPECT().GetStorageInstancesForProviderIDs(gomock.Any(), []string{
		"fs-1",
	}).Return([]internal.StorageInstanceComposition{}, nil)
	s.state.EXPECT().CheckUnitExists(gomock.Any(), unitUUID).Return(false, nil)
	s.state.EXPECT().GetApplicationStorageDirectives(gomock.Any(), appUUID).Return(
		[]application.StorageDirective{
			{
				CharmMetadataName: "big-beautiful-charm",
				Count:             1,
				CharmStorageType:  charm.StorageFilesystem,
				MaxCount:          1,
				Name:              "st1",
				PoolUUID:          poolUUID,
				Size:              1024,
			},
		},
		nil,
	)
	svc := storageService{
		st:                  s.state,
		storagePoolProvider: s.poolProvider,
	}

	arg, err := svc.getRegisterCAASUnitStorageArg(
		c.Context(), appUUID, unitUUID, attachNetNodeUUID, providerFSInfo,
	)
	c.Check(err, tc.IsNil)

	expectedStorageDirectives := []application.CreateUnitStorageDirectiveArg{
		{
			Count:    1,
			Name:     "st1",
			PoolUUID: poolUUID,
			Size:     1024,
		},
	}

	expectedStorageInstances := []application.CreateUnitStorageInstanceArg{
		{
			CharmName: "big-beautiful-charm",
			Filesystem: &application.CreateUnitStorageFilesystemArg{
				ProvisionScope: domainstorageprov.ProvisionScopeModel,
			},
			Kind:            domainstorageprov.KindFilesystem,
			Name:            "st1",
			RequestSizeMiB:  1024,
			StoragePoolUUID: poolUUID,
		},
	}

	expectedStorageToAttach := []application.CreateUnitStorageAttachmentArg{
		{
			FilesystemAttachment: &application.CreateUnitStorageFilesystemAttachmentArg{
				FilesystemUUID: arg.StorageInstances[0].Filesystem.UUID,
				NetNodeUUID:    attachNetNodeUUID,
				ProvisionScope: domainstorageprov.ProvisionScopeModel,
			},
			StorageInstanceUUID: arg.StorageInstances[0].UUID,
		},
	}

	expectedStorageToOwn := []domainstorage.StorageInstanceUUID{
		arg.StorageInstances[0].UUID,
	}

	c.Check(arg, registerUnitStorageArgChecker(), application.RegisterUnitStorageArg{
		CreateUnitStorageArg: application.CreateUnitStorageArg{
			StorageDirectives: expectedStorageDirectives,
			StorageInstances:  expectedStorageInstances,
			StorageToAttach:   expectedStorageToAttach,
			StorageToOwn:      expectedStorageToOwn,
		},
		FilesystemProviderIDs: map[domainstorageprov.FilesystemUUID]string{
			arg.StorageInstances[0].Filesystem.UUID: "fs-1",
		},
	})
}

// TestGetRegisterCAASUnitStorageArgExistingUnit tests the happy path of seeing
// an already existing unit being registered with it's own storage.
//
// This test asserts the following preconditions:
// - No new storage is created for the unit.
// - The already re-identified storage for the unit is consumed again.
func (s *registerCAASStorageSuite) TestGetRegisterCAASUnitStorageArgExistingUnit(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	appUUID := tc.Must(c, coreapplication.NewID)
	unitUUID := tc.Must(c, coreunit.NewUUID)
	poolUUID := s.newFilesystemStoragePool(c, ctrl)
	attachNetNodeUUID := tc.Must(c, domainnetwork.NewNetNodeUUID)
	providerFSInfo := []caas.FilesystemInfo{
		{
			FilesystemId: "fs-1",
			StorageName:  "st1",
		},
		{
			FilesystemId: "fs-2",
			StorageName:  "st2",
		},
	}

	// unitOwnedStorage represents the storage instances that are already owned
	// by the unit in the model.
	unitOwnedStorage := []internal.StorageInstanceComposition{
		{
			Filesystem: &internal.StorageInstanceCompositionFilesystem{
				ProviderID:     "fs-1",
				ProvisionScope: domainstorageprov.ProvisionScopeModel,
				UUID:           tc.Must(c, domainstorageprov.NewFilesystemUUID),
			},
			StorageName: "st1",
			UUID:        tc.Must(c, domainstorage.NewStorageInstanceUUID),
		},
		{
			Filesystem: &internal.StorageInstanceCompositionFilesystem{
				ProviderID:     "fs-2",
				ProvisionScope: domainstorageprov.ProvisionScopeModel,
				UUID:           tc.Must(c, domainstorageprov.NewFilesystemUUID),
			},
			StorageName: "st2",
			UUID:        tc.Must(c, domainstorage.NewStorageInstanceUUID),
		},
	}

	// GetStorageInstancesForProviderIDs returns an empty slice because the
	// storage instances identified by the provider ids are already owned by a
	// unit in the model.
	s.state.EXPECT().GetStorageInstancesForProviderIDs(gomock.Any(), []string{
		"fs-1", "fs-2",
	}).Return([]internal.StorageInstanceComposition{}, nil).AnyTimes()
	s.state.EXPECT().CheckUnitExists(gomock.Any(), unitUUID).Return(true, nil).AnyTimes()
	s.state.EXPECT().GetUnitStorageDirectives(gomock.Any(), unitUUID).Return(
		[]application.StorageDirective{
			{
				CharmMetadataName: "big-beautiful-charm",
				Count:             1,
				CharmStorageType:  charm.StorageFilesystem,
				MaxCount:          1,
				Name:              "st1",
				PoolUUID:          poolUUID,
				Size:              1024,
			},
			{
				CharmMetadataName: "big-beautiful-charm",
				Count:             1,
				CharmStorageType:  charm.StorageFilesystem,
				MaxCount:          1,
				Name:              "st2",
				PoolUUID:          poolUUID,
				Size:              1024,
			},
		}, nil,
	).AnyTimes()
	// The storage instances associated with the provider ids are return here
	// because they are already owned by the unit in question.
	s.state.EXPECT().GetUnitOwnedStorageInstances(gomock.Any(), unitUUID).Return(
		unitOwnedStorage, nil,
	).AnyTimes()

	svc := storageService{
		st:                  s.state,
		storagePoolProvider: s.poolProvider,
	}
	arg, err := svc.getRegisterCAASUnitStorageArg(
		c.Context(), appUUID, unitUUID, attachNetNodeUUID, providerFSInfo,
	)
	c.Check(err, tc.IsNil)

	expectedStorageDirectives := []application.CreateUnitStorageDirectiveArg{
		{
			Count:    1,
			Name:     "st1",
			PoolUUID: poolUUID,
			Size:     1024,
		},
		{
			Count:    1,
			Name:     "st2",
			PoolUUID: poolUUID,
			Size:     1024,
		},
	}

	// expectedStorageInstances is empty because no new storage WILL be created.
	expectedStorageInstances := []application.CreateUnitStorageInstanceArg{}

	// We expect to see the existing storage come back in the attachments. This
	// is to make sure the storage is attached.
	expectedStorageToAttach := []application.CreateUnitStorageAttachmentArg{
		{
			FilesystemAttachment: &application.CreateUnitStorageFilesystemAttachmentArg{
				FilesystemUUID: unitOwnedStorage[0].Filesystem.UUID,
				NetNodeUUID:    attachNetNodeUUID,
				ProvisionScope: domainstorageprov.ProvisionScopeModel,
			},
			StorageInstanceUUID: unitOwnedStorage[0].UUID,
		},
		{
			FilesystemAttachment: &application.CreateUnitStorageFilesystemAttachmentArg{
				FilesystemUUID: unitOwnedStorage[1].Filesystem.UUID,
				NetNodeUUID:    attachNetNodeUUID,
				ProvisionScope: domainstorageprov.ProvisionScopeModel,
			},
			StorageInstanceUUID: unitOwnedStorage[1].UUID,
		},
	}

	expectedStorageToOwn := []domainstorage.StorageInstanceUUID{}

	c.Check(arg, registerUnitStorageArgChecker(), application.RegisterUnitStorageArg{
		CreateUnitStorageArg: application.CreateUnitStorageArg{
			StorageDirectives: expectedStorageDirectives,
			StorageInstances:  expectedStorageInstances,
			StorageToAttach:   expectedStorageToAttach,
			StorageToOwn:      expectedStorageToOwn,
		},
	})
}

// TestGetRegisterCAASUnitStorageArgExistingUnitAttachStorage tests the happy
// path of seeing an already existing unit being registered with it's own
// storage. This tests wants to see that when the unit has new storage that
// isn't owned or attached we correctly associate it with the unit.
//
// This test asserts the following preconditions:
// - No new storage is created for the unit.
// - The already re-identified storage for the unit is consumed again.
// - The existing provider storage in the model is associated with the unit.
func (s *registerCAASStorageSuite) TestGetRegisterCAASUnitStorageArgExistingUnitAttachStorage(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	appUUID := tc.Must(c, coreapplication.NewID)
	unitUUID := tc.Must(c, coreunit.NewUUID)
	poolUUID := s.newFilesystemStoragePool(c, ctrl)
	attachNetNodeUUID := tc.Must(c, domainnetwork.NewNetNodeUUID)
	providerFSInfo := []caas.FilesystemInfo{
		{
			FilesystemId: "fs-1",
			StorageName:  "st1",
		},
		{
			FilesystemId: "fs-2",
			StorageName:  "st2",
		},
	}

	// unitOwnedStorage represents the storage instances that are already owned
	// by the unit in the model.
	unitOwnedStorage := []internal.StorageInstanceComposition{
		{
			Filesystem: &internal.StorageInstanceCompositionFilesystem{
				ProviderID:     "fs-1",
				ProvisionScope: domainstorageprov.ProvisionScopeModel,
				UUID:           tc.Must(c, domainstorageprov.NewFilesystemUUID),
			},
			StorageName: "st1",
			UUID:        tc.Must(c, domainstorage.NewStorageInstanceUUID),
		},
	}
	existingProviderStorage := []internal.StorageInstanceComposition{
		{
			Filesystem: &internal.StorageInstanceCompositionFilesystem{
				ProviderID:     "fs-2",
				ProvisionScope: domainstorageprov.ProvisionScopeModel,
				UUID:           tc.Must(c, domainstorageprov.NewFilesystemUUID),
			},
			StorageName: "st2",
			UUID:        tc.Must(c, domainstorage.NewStorageInstanceUUID),
		},
	}

	// GetStorageInstancesForProviderIDs returns an empty slice because the
	// storage instances identified by the provider ids are already owned by a
	// unit in the model.
	s.state.EXPECT().GetStorageInstancesForProviderIDs(gomock.Any(), []string{
		"fs-1", "fs-2",
	}).Return(existingProviderStorage, nil).AnyTimes()
	s.state.EXPECT().CheckUnitExists(gomock.Any(), unitUUID).Return(true, nil).AnyTimes()
	s.state.EXPECT().GetUnitStorageDirectives(gomock.Any(), unitUUID).Return(
		[]application.StorageDirective{
			{
				CharmMetadataName: "big-beautiful-charm",
				Count:             1,
				CharmStorageType:  charm.StorageFilesystem,
				MaxCount:          1,
				Name:              "st1",
				PoolUUID:          poolUUID,
				Size:              1024,
			},
			{
				CharmMetadataName: "big-beautiful-charm",
				Count:             1,
				CharmStorageType:  charm.StorageFilesystem,
				MaxCount:          1,
				Name:              "st2",
				PoolUUID:          poolUUID,
				Size:              1024,
			},
		}, nil,
	).AnyTimes()
	// The storage instances associated with the provider ids are return here
	// because they are already owned by the unit in question.
	s.state.EXPECT().GetUnitOwnedStorageInstances(gomock.Any(), unitUUID).Return(
		unitOwnedStorage, nil,
	).AnyTimes()

	svc := storageService{
		st:                  s.state,
		storagePoolProvider: s.poolProvider,
	}
	arg, err := svc.getRegisterCAASUnitStorageArg(
		c.Context(), appUUID, unitUUID, attachNetNodeUUID, providerFSInfo,
	)
	c.Check(err, tc.IsNil)

	expectedStorageDirectives := []application.CreateUnitStorageDirectiveArg{
		{
			Count:    1,
			Name:     "st1",
			PoolUUID: poolUUID,
			Size:     1024,
		},
		{
			Count:    1,
			Name:     "st2",
			PoolUUID: poolUUID,
			Size:     1024,
		},
	}

	// expectedStorageInstances is empty because no new storage WILL be created.
	expectedStorageInstances := []application.CreateUnitStorageInstanceArg{}

	// We expect to see the existing storage come back in the attachments. This
	// is to make sure the storage is attached.
	expectedStorageToAttach := []application.CreateUnitStorageAttachmentArg{
		{
			FilesystemAttachment: &application.CreateUnitStorageFilesystemAttachmentArg{
				FilesystemUUID: unitOwnedStorage[0].Filesystem.UUID,
				NetNodeUUID:    attachNetNodeUUID,
				ProvisionScope: domainstorageprov.ProvisionScopeModel,
			},
			StorageInstanceUUID: unitOwnedStorage[0].UUID,
		},
		{
			FilesystemAttachment: &application.CreateUnitStorageFilesystemAttachmentArg{
				FilesystemUUID: existingProviderStorage[0].Filesystem.UUID,
				NetNodeUUID:    attachNetNodeUUID,
				ProvisionScope: domainstorageprov.ProvisionScopeModel,
			},
			StorageInstanceUUID: existingProviderStorage[0].UUID,
		},
	}

	expectedStorageToOwn := []domainstorage.StorageInstanceUUID{
		existingProviderStorage[0].UUID,
	}

	c.Check(arg, registerUnitStorageArgChecker(), application.RegisterUnitStorageArg{
		CreateUnitStorageArg: application.CreateUnitStorageArg{
			StorageDirectives: expectedStorageDirectives,
			StorageInstances:  expectedStorageInstances,
			StorageToAttach:   expectedStorageToAttach,
			StorageToOwn:      expectedStorageToOwn,
		},
	})
}

// TestGetRegisterCAASUnitNewUnitExistingStorage is a test to assert that when
// a new unit comes into the model it uses all of the existing storage in the
// model and doesn't re-create any new storage.
//
// This tests is to handle a common Kubernetes case where we will see a unit
// (pod) go away because of a scale down event. However we expect the storage
// for the unit to still exist in the model. We want to see that on scale up
// when the unit comes back we correctly associate the existing storage with
// the new unit.
//
// This test asserts the following preconditions:
// - No new storage is created for the unit.
// - The existing provider storage in the model is associated with the unit.
func (s *registerCAASStorageSuite) TestGetRegisterCAASUnitNewUnitExistingStorage(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	appUUID := tc.Must(c, coreapplication.NewID)
	unitUUID := tc.Must(c, coreunit.NewUUID)
	poolUUID := s.newFilesystemStoragePool(c, ctrl)
	attachNetNodeUUID := tc.Must(c, domainnetwork.NewNetNodeUUID)
	providerFSInfo := []caas.FilesystemInfo{
		{
			FilesystemId: "fs-1",
			StorageName:  "st1",
		},
		{
			FilesystemId: "fs-2",
			StorageName:  "st2",
		},
	}

	existingProviderStorage := []internal.StorageInstanceComposition{
		{
			Filesystem: &internal.StorageInstanceCompositionFilesystem{
				ProviderID:     "fs-1",
				ProvisionScope: domainstorageprov.ProvisionScopeModel,
				UUID:           tc.Must(c, domainstorageprov.NewFilesystemUUID),
			},
			StorageName: "st1",
			UUID:        tc.Must(c, domainstorage.NewStorageInstanceUUID),
		},
		{
			Filesystem: &internal.StorageInstanceCompositionFilesystem{
				ProviderID:     "fs-2",
				ProvisionScope: domainstorageprov.ProvisionScopeModel,
				UUID:           tc.Must(c, domainstorageprov.NewFilesystemUUID),
			},
			StorageName: "st2",
			UUID:        tc.Must(c, domainstorage.NewStorageInstanceUUID),
		},
	}

	// GetStorageInstancesForProviderIDs returns an empty slice because the
	// storage instances identified by the provider ids are already owned by a
	// unit in the model.
	s.state.EXPECT().GetStorageInstancesForProviderIDs(gomock.Any(), []string{
		"fs-1", "fs-2",
	}).Return(existingProviderStorage, nil).AnyTimes()
	s.state.EXPECT().CheckUnitExists(gomock.Any(), unitUUID).Return(true, nil).AnyTimes()
	s.state.EXPECT().GetUnitStorageDirectives(gomock.Any(), unitUUID).Return(
		[]application.StorageDirective{
			{
				CharmMetadataName: "big-beautiful-charm",
				Count:             1,
				CharmStorageType:  charm.StorageFilesystem,
				MaxCount:          1,
				Name:              "st1",
				PoolUUID:          poolUUID,
				Size:              1024,
			},
			{
				CharmMetadataName: "big-beautiful-charm",
				Count:             1,
				CharmStorageType:  charm.StorageFilesystem,
				MaxCount:          1,
				Name:              "st2",
				PoolUUID:          poolUUID,
				Size:              1024,
			},
		}, nil,
	).AnyTimes()
	// The storage instances associated with the provider ids are return here
	// because they are already owned by the unit in question.
	s.state.EXPECT().GetUnitOwnedStorageInstances(gomock.Any(), unitUUID).Return(
		[]internal.StorageInstanceComposition{}, nil,
	).AnyTimes()

	svc := storageService{
		st:                  s.state,
		storagePoolProvider: s.poolProvider,
	}
	arg, err := svc.getRegisterCAASUnitStorageArg(
		c.Context(), appUUID, unitUUID, attachNetNodeUUID, providerFSInfo,
	)
	c.Check(err, tc.IsNil)

	expectedStorageDirectives := []application.CreateUnitStorageDirectiveArg{
		{
			Count:    1,
			Name:     "st1",
			PoolUUID: poolUUID,
			Size:     1024,
		},
		{
			Count:    1,
			Name:     "st2",
			PoolUUID: poolUUID,
			Size:     1024,
		},
	}

	// expectedStorageInstances is empty because no new storage WILL be created.
	expectedStorageInstances := []application.CreateUnitStorageInstanceArg{}

	// We expect to see the existing storage come back in the attachments. This
	// is to make sure the storage is attached.
	expectedStorageToAttach := []application.CreateUnitStorageAttachmentArg{
		{
			FilesystemAttachment: &application.CreateUnitStorageFilesystemAttachmentArg{
				FilesystemUUID: existingProviderStorage[0].Filesystem.UUID,
				NetNodeUUID:    attachNetNodeUUID,
				ProvisionScope: domainstorageprov.ProvisionScopeModel,
			},
			StorageInstanceUUID: existingProviderStorage[0].UUID,
		},
		{
			FilesystemAttachment: &application.CreateUnitStorageFilesystemAttachmentArg{
				FilesystemUUID: existingProviderStorage[1].Filesystem.UUID,
				NetNodeUUID:    attachNetNodeUUID,
				ProvisionScope: domainstorageprov.ProvisionScopeModel,
			},
			StorageInstanceUUID: existingProviderStorage[1].UUID,
		},
	}

	expectedStorageToOwn := []domainstorage.StorageInstanceUUID{
		existingProviderStorage[0].UUID,
		existingProviderStorage[1].UUID,
	}

	c.Check(arg, registerUnitStorageArgChecker(), application.RegisterUnitStorageArg{
		CreateUnitStorageArg: application.CreateUnitStorageArg{
			StorageDirectives: expectedStorageDirectives,
			StorageInstances:  expectedStorageInstances,
			StorageToAttach:   expectedStorageToAttach,
			StorageToOwn:      expectedStorageToOwn,
		},
	})
}

// TestGetRegisterCAASUnitApplicationNotFound tests that when the application
// does not exist anymore the caller gets back an error satisfying
// [applicationerrors.ApplicationNotFound].
func (s *registerCAASStorageSuite) TestGetRegisterCAASUnitApplicationNotFound(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	appUUID := tc.Must(c, coreapplication.NewID)
	attachNetNodeUUID := tc.Must(c, domainnetwork.NewNetNodeUUID)
	unitUUID := tc.Must(c, coreunit.NewUUID)
	providerFSInfo := []caas.FilesystemInfo{
		{
			FilesystemId: "fs-1",
			StorageName:  "st1",
		},
	}

	s.state.EXPECT().GetStorageInstancesForProviderIDs(gomock.Any(), []string{
		"fs-1",
	}).Return([]internal.StorageInstanceComposition{}, nil)
	s.state.EXPECT().GetApplicationStorageDirectives(gomock.Any(), appUUID).Return(
		[]application.StorageDirective{},
		applicationerrors.ApplicationNotFound,
	)
	svc := storageService{
		st:                  s.state,
		storagePoolProvider: s.poolProvider,
	}

	_, err := svc.getRegisterCAASUnitStorageArg(
		c.Context(), appUUID, unitUUID, attachNetNodeUUID, providerFSInfo,
	)
	c.Check(err, tc.ErrorIs, applicationerrors.ApplicationNotFound)
}

// TestMakeCAASStorageInstanceProviderIDAssociations tests the happy path of
// [makeCAASStorageInstanceProviderIDAssociations]. This test is aimed at
// ensuring that new storage being created for a unit has a provide id assigned.
func (*registerCAASStorageSuite) TestMakeCAASStorageInstanceProviderIDAssociations(c *tc.C) {
	pFSInfo := []caas.FilesystemInfo{
		{
			FilesystemId: "fs-1",
			StorageName:  "st1",
		},
		{
			FilesystemId: "fs-2",
			StorageName:  "st1",
		},
		{
			FilesystemId: "fs-3",
			StorageName:  "st2",
		},
	}

	existingProviderStorage := []internal.StorageInstanceComposition{
		{
			Filesystem: &internal.StorageInstanceCompositionFilesystem{
				ProviderID: "fs-2",
			},
			StorageName: "st1",
			Volume: &internal.StorageInstanceCompositionVolume{
				ProviderID: "fs-2",
			},
		},
	}

	fs1UUID := tc.Must(c, domainstorageprov.NewFilesystemUUID)
	fs2UUID := tc.Must(c, domainstorageprov.NewFilesystemUUID)
	v1UUID := tc.Must(c, domainstorageprov.NewVolumeUUID)
	unitStorageToCreate := []application.CreateUnitStorageInstanceArg{
		{
			Filesystem: &application.CreateUnitStorageFilesystemArg{
				UUID: fs1UUID,
			},
			Name: "st1",
			Volume: &application.CreateUnitStorageVolumeArg{
				UUID: v1UUID,
			},
		},
		{
			Filesystem: &application.CreateUnitStorageFilesystemArg{
				UUID: fs2UUID,
			},
			Name: "st2",
		},
	}

	fsAssociations, vAssociations :=
		makeCAASStorageInstanceProviderIDAssociations(
			pFSInfo, existingProviderStorage, unitStorageToCreate,
		)

	c.Check(fsAssociations, tc.DeepEquals, map[domainstorageprov.FilesystemUUID]string{
		fs1UUID: "fs-1",
		fs2UUID: "fs-3",
	})
	c.Check(vAssociations, tc.DeepEquals, map[domainstorageprov.VolumeUUID]string{
		v1UUID: "fs-1",
	})
}

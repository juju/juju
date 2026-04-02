// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"testing"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	corecharm "github.com/juju/juju/core/charm"
	coreerrors "github.com/juju/juju/core/errors"
	coremachine "github.com/juju/juju/core/machine"
	coreunit "github.com/juju/juju/core/unit"
	applicationcharm "github.com/juju/juju/domain/application/charm"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/application/internal"
	domainlife "github.com/juju/juju/domain/life"
	domainnetwork "github.com/juju/juju/domain/network"
	domainstorage "github.com/juju/juju/domain/storage"
	storageerrors "github.com/juju/juju/domain/storage/errors"
	storageprovisioning "github.com/juju/juju/domain/storageprovisioning"
	"github.com/juju/juju/internal/errors"
)

// attachStorageSuite exercises ProviderService attach-storage workflows,
// covering validation and argument construction for attaching existing
// storage instances to units.
type attachStorageSuite struct {
	baseSuite
}

// TestAttachStorageSuite runs the tests defined in [attachStorageSuite].
func TestAttachStorageSuite(t *testing.T) {
	tc.Run(t, &attachStorageSuite{})
}

// makeValidAttachInfo returns a fully populated attachment info struct with
// consistent values, allowing validation tests to avoid relying on ordering.
func makeValidAttachInfo(
	c *tc.C,
	unitUUID coreunit.UUID,
	storageUUID domainstorage.StorageInstanceUUID,
) internal.StorageInstanceInfoForUnitAttach {
	charmName := "example"
	charmUUID := tc.Must(c, corecharm.NewID)
	netNodeUUID := tc.Must(c, domainnetwork.NewNetNodeUUID)
	filesystemUUID := tc.Must(c, domainstorage.NewFilesystemUUID)

	return internal.StorageInstanceInfoForUnitAttach{
		StorageInstanceInfo: internal.StorageInstanceInfo{
			UUID:             storageUUID,
			CharmName:        &charmName,
			Filesystem:       &internal.StorageInstanceFilesystemInfo{UUID: filesystemUUID, Size: 1024},
			Kind:             domainstorage.StorageKindFilesystem,
			Life:             domainlife.Alive,
			RequestedSizeMIB: 1024,
			StorageName:      "data",
		},
		UnitNamedStorageInfo: internal.UnitNamedStorageInfo{
			CharmStorageDefinitionForValidation: internal.CharmStorageDefinitionForValidation{
				Name:        "data",
				Type:        applicationcharm.StorageFilesystem,
				CountMin:    0,
				CountMax:    -1,
				MinimumSize: 1,
				Shared:      true,
			},
			UUID:                 unitUUID,
			AlreadyAttachedCount: 0,
			CharmMetadataName:    charmName,
			CharmUUID:            charmUUID,
			Life:                 domainlife.Alive,
			Name:                 "app/0",
			NetNodeUUID:          netNodeUUID,
		},
		StorageInstanceAttachments: []internal.StorageInstanceUnitAttachment{},
	}
}

// assertAttachStorageStateError ensures AttachStorageToUnit surfaces the error
// returned by state after validation succeeds.
func (s *attachStorageSuite) assertAttachStorageStateError(
	c *tc.C,
	stateErr error,
) {
	storageUUID := tc.Must(c, domainstorage.NewStorageInstanceUUID)
	unitUUID := tc.Must(c, coreunit.NewUUID)
	attachInfo := makeValidAttachInfo(c, unitUUID, storageUUID)
	attachArg := internal.AttachStorageInstanceToUnitArg{
		CreateStorageInstanceAttachmentArg: internal.CreateStorageInstanceAttachmentArg{
			StorageInstanceUUID: storageUUID,
		},
	}

	s.state.EXPECT().GetStorageAttachInfoByUnitUUIDAndStorageUUID(
		gomock.Any(), unitUUID, storageUUID,
	).Return(
		attachInfo,
		nil,
	)
	s.storageService.EXPECT().MakeAttachStorageInstanceToUnitArg(
		gomock.Any(),
		attachInfo,
	).Return(
		attachArg,
		nil,
	)
	s.state.EXPECT().AttachStorageToUnit(
		gomock.Any(), unitUUID, attachArg,
	).Return(
		stateErr,
	)

	err := s.service.AttachStorageToUnit(c.Context(), storageUUID, unitUUID)
	c.Check(err, tc.ErrorIs, stateErr)
}

// TestAttachStorageInvalidStorageUUID ensures invalid storage UUIDs are
// rejected with a [coreerrors.NotValid] error.
func (s *attachStorageSuite) TestAttachStorageInvalidStorageUUID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	storageUUID := domainstorage.StorageInstanceUUID("not-a-uuid")
	unitUUID := tc.Must(c, coreunit.NewUUID)

	err := s.service.AttachStorageToUnit(c.Context(), storageUUID, unitUUID)
	c.Check(err, tc.ErrorIs, coreerrors.NotValid)
}

// TestAttachStorageInvalidUnitUUID ensures invalid unit UUIDs are rejected
// with a [coreerrors.NotValid] error.
func (s *attachStorageSuite) TestAttachStorageInvalidUnitUUID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	storageUUID := tc.Must(c, domainstorage.NewStorageInstanceUUID)
	unitUUID := coreunit.UUID("not-a-uuid")

	err := s.service.AttachStorageToUnit(c.Context(), storageUUID, unitUUID)
	c.Check(err, tc.ErrorIs, coreerrors.NotValid)
}

// TestAttachStorageStorageInstanceNotFound ensures a missing storage instance
// returns [storageerrors.StorageInstanceNotFound].
func (s *attachStorageSuite) TestAttachStorageStorageInstanceNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	storageUUID := tc.Must(c, domainstorage.NewStorageInstanceUUID)
	unitUUID := tc.Must(c, coreunit.NewUUID)

	s.state.EXPECT().GetStorageAttachInfoByUnitUUIDAndStorageUUID(
		gomock.Any(), unitUUID, storageUUID,
	).Return(
		internal.StorageInstanceInfoForUnitAttach{},
		storageerrors.StorageInstanceNotFound,
	)

	err := s.service.AttachStorageToUnit(c.Context(), storageUUID, unitUUID)
	c.Check(err, tc.ErrorIs, storageerrors.StorageInstanceNotFound)
}

// TestAttachStorageUnitNotFound ensures a missing unit returns
// [applicationerrors.UnitNotFound].
func (s *attachStorageSuite) TestAttachStorageUnitNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	storageUUID := tc.Must(c, domainstorage.NewStorageInstanceUUID)
	unitUUID := tc.Must(c, coreunit.NewUUID)

	s.state.EXPECT().GetStorageAttachInfoByUnitUUIDAndStorageUUID(
		gomock.Any(), unitUUID, storageUUID,
	).Return(
		internal.StorageInstanceInfoForUnitAttach{},
		applicationerrors.UnitNotFound,
	)

	err := s.service.AttachStorageToUnit(c.Context(), storageUUID, unitUUID)
	c.Check(err, tc.ErrorIs, applicationerrors.UnitNotFound)
}

// TestAttachStorageStorageNameNotSupported ensures a storage name mismatch
// returns [applicationerrors.StorageNameNotSupported].
func (s *attachStorageSuite) TestAttachStorageStorageNameNotSupported(c *tc.C) {
	defer s.setupMocks(c).Finish()

	storageUUID := tc.Must(c, domainstorage.NewStorageInstanceUUID)
	unitUUID := tc.Must(c, coreunit.NewUUID)

	s.state.EXPECT().GetStorageAttachInfoByUnitUUIDAndStorageUUID(
		gomock.Any(), unitUUID, storageUUID,
	).Return(
		internal.StorageInstanceInfoForUnitAttach{},
		applicationerrors.StorageNameNotSupported,
	)

	err := s.service.AttachStorageToUnit(c.Context(), storageUUID, unitUUID)
	c.Check(err, tc.ErrorIs, applicationerrors.StorageNameNotSupported)
}

// TestAttachStorageStorageInstanceNotAlive ensures a non-alive storage instance
// returns [storageerrors.StorageInstanceNotAlive].
func (s *attachStorageSuite) TestAttachStorageStorageInstanceNotAlive(c *tc.C) {
	defer s.setupMocks(c).Finish()

	storageUUID := tc.Must(c, domainstorage.NewStorageInstanceUUID)
	unitUUID := tc.Must(c, coreunit.NewUUID)
	attachInfo := makeValidAttachInfo(c, unitUUID, storageUUID)
	attachInfo.StorageInstanceInfo.Life = domainlife.Dying

	s.state.EXPECT().GetStorageAttachInfoByUnitUUIDAndStorageUUID(
		gomock.Any(), unitUUID, storageUUID,
	).Return(
		attachInfo,
		nil,
	)

	err := s.service.AttachStorageToUnit(c.Context(), storageUUID, unitUUID)
	c.Check(err, tc.ErrorIs, storageerrors.StorageInstanceNotAlive)
}

// TestAttachStorageUnitNotAlive ensures a non-alive unit returns
// [applicationerrors.UnitNotAlive].
func (s *attachStorageSuite) TestAttachStorageUnitNotAlive(c *tc.C) {
	defer s.setupMocks(c).Finish()

	storageUUID := tc.Must(c, domainstorage.NewStorageInstanceUUID)
	unitUUID := tc.Must(c, coreunit.NewUUID)
	attachInfo := makeValidAttachInfo(c, unitUUID, storageUUID)
	attachInfo.UnitNamedStorageInfo.Life = domainlife.Dying

	s.state.EXPECT().GetStorageAttachInfoByUnitUUIDAndStorageUUID(
		gomock.Any(), unitUUID, storageUUID,
	).Return(
		attachInfo,
		nil,
	)

	err := s.service.AttachStorageToUnit(c.Context(), storageUUID, unitUUID)
	c.Check(err, tc.ErrorIs, applicationerrors.UnitNotAlive)
}

// TestAttachStorageCharmNameMismatch ensures a charm name mismatch returns
// [applicationerrors.StorageInstanceCharmNameMismatch].
func (s *attachStorageSuite) TestAttachStorageCharmNameMismatch(c *tc.C) {
	defer s.setupMocks(c).Finish()

	storageUUID := tc.Must(c, domainstorage.NewStorageInstanceUUID)
	unitUUID := tc.Must(c, coreunit.NewUUID)
	attachInfo := makeValidAttachInfo(c, unitUUID, storageUUID)
	attachInfo.StorageInstanceInfo.CharmName = new("other")

	s.state.EXPECT().GetStorageAttachInfoByUnitUUIDAndStorageUUID(
		gomock.Any(), unitUUID, storageUUID,
	).Return(
		attachInfo,
		nil,
	)

	err := s.service.AttachStorageToUnit(c.Context(), storageUUID, unitUUID)
	c.Check(err, tc.ErrorIs, applicationerrors.StorageInstanceCharmNameMismatch)
}

// TestAttachStorageKindMismatch ensures a kind mismatch returns
// [applicationerrors.StorageInstanceKindNotValidForCharmStorageDefinition].
func (s *attachStorageSuite) TestAttachStorageKindMismatch(c *tc.C) {
	defer s.setupMocks(c).Finish()

	storageUUID := tc.Must(c, domainstorage.NewStorageInstanceUUID)
	unitUUID := tc.Must(c, coreunit.NewUUID)
	attachInfo := makeValidAttachInfo(c, unitUUID, storageUUID)
	attachInfo.UnitNamedStorageInfo.CharmStorageDefinitionForValidation.Type =
		applicationcharm.StorageBlock

	s.state.EXPECT().GetStorageAttachInfoByUnitUUIDAndStorageUUID(
		gomock.Any(), unitUUID, storageUUID,
	).Return(
		attachInfo,
		nil,
	)

	err := s.service.AttachStorageToUnit(c.Context(), storageUUID, unitUUID)
	c.Check(err, tc.ErrorIs, applicationerrors.StorageInstanceKindNotValidForCharmStorageDefinition)
}

// TestAttachStorageSizeBelowMinimum ensures a size below the minimum returns
// [applicationerrors.StorageInstanceSizeNotValidForCharmStorageDefinition].
func (s *attachStorageSuite) TestAttachStorageSizeBelowMinimum(c *tc.C) {
	defer s.setupMocks(c).Finish()

	storageUUID := tc.Must(c, domainstorage.NewStorageInstanceUUID)
	unitUUID := tc.Must(c, coreunit.NewUUID)
	attachInfo := makeValidAttachInfo(c, unitUUID, storageUUID)
	attachInfo.StorageInstanceInfo.Filesystem.Size = 1
	attachInfo.StorageInstanceInfo.RequestedSizeMIB = 1
	attachInfo.UnitNamedStorageInfo.CharmStorageDefinitionForValidation.MinimumSize = 2

	s.state.EXPECT().GetStorageAttachInfoByUnitUUIDAndStorageUUID(
		gomock.Any(), unitUUID, storageUUID,
	).Return(
		attachInfo,
		nil,
	)

	err := s.service.AttachStorageToUnit(c.Context(), storageUUID, unitUUID)
	c.Check(err, tc.ErrorIs, applicationerrors.StorageInstanceSizeNotValidForCharmStorageDefinition)
}

// TestAttachStorageSizeFallbackToRequested ensures size validation falls back
// to requested size when provisioned size is unset and reports
// [applicationerrors.StorageInstanceSizeNotValidForCharmStorageDefinition].
func (s *attachStorageSuite) TestAttachStorageSizeFallbackToRequested(c *tc.C) {
	defer s.setupMocks(c).Finish()

	storageUUID := tc.Must(c, domainstorage.NewStorageInstanceUUID)
	unitUUID := tc.Must(c, coreunit.NewUUID)
	attachInfo := makeValidAttachInfo(c, unitUUID, storageUUID)
	attachInfo.StorageInstanceInfo.Filesystem.Size = 0
	attachInfo.StorageInstanceInfo.RequestedSizeMIB = 1
	attachInfo.UnitNamedStorageInfo.CharmStorageDefinitionForValidation.MinimumSize = 2

	s.state.EXPECT().GetStorageAttachInfoByUnitUUIDAndStorageUUID(
		gomock.Any(), unitUUID, storageUUID,
	).Return(
		attachInfo,
		nil,
	)

	err := s.service.AttachStorageToUnit(c.Context(), storageUUID, unitUUID)
	c.Check(err, tc.ErrorIs, applicationerrors.StorageInstanceSizeNotValidForCharmStorageDefinition)
}

// TestAttachStorageCountLimitExceeded ensures exceeding the charm max returns
// [applicationerrors.StorageCountLimitExceeded].
func (s *attachStorageSuite) TestAttachStorageCountLimitExceeded(c *tc.C) {
	defer s.setupMocks(c).Finish()

	storageUUID := tc.Must(c, domainstorage.NewStorageInstanceUUID)
	unitUUID := tc.Must(c, coreunit.NewUUID)
	attachInfo := makeValidAttachInfo(c, unitUUID, storageUUID)
	attachInfo.UnitNamedStorageInfo.CharmStorageDefinitionForValidation.CountMax = 0
	attachInfo.UnitNamedStorageInfo.AlreadyAttachedCount = 0

	s.state.EXPECT().GetStorageAttachInfoByUnitUUIDAndStorageUUID(
		gomock.Any(), unitUUID, storageUUID,
	).Return(
		attachInfo,
		nil,
	)

	err := s.service.AttachStorageToUnit(c.Context(), storageUUID, unitUUID)
	errVal, is := errors.AsType[applicationerrors.StorageCountLimitExceeded](err)
	c.Check(is, tc.IsTrue)
	c.Check(errVal, tc.DeepEquals, applicationerrors.StorageCountLimitExceeded{
		Maximum:     new(0),
		Minimum:     0,
		Requested:   1,
		StorageName: "data",
	})
}

// TestAttachStorageAlreadyAttached ensures an existing attachment returns
// [applicationerrors.StorageInstanceAlreadyAttachedToUnit].
func (s *attachStorageSuite) TestAttachStorageAlreadyAttached(c *tc.C) {
	defer s.setupMocks(c).Finish()

	storageUUID := tc.Must(c, domainstorage.NewStorageInstanceUUID)
	unitUUID := tc.Must(c, coreunit.NewUUID)
	attachmentUUID := tc.Must(c, domainstorage.NewStorageAttachmentUUID)
	attachInfo := makeValidAttachInfo(c, unitUUID, storageUUID)
	attachInfo.StorageInstanceAttachments = []internal.StorageInstanceUnitAttachment{
		{
			UnitUUID: unitUUID,
			UUID:     attachmentUUID,
		},
	}

	s.state.EXPECT().GetStorageAttachInfoByUnitUUIDAndStorageUUID(
		gomock.Any(), unitUUID, storageUUID,
	).Return(
		attachInfo,
		nil,
	)

	err := s.service.AttachStorageToUnit(c.Context(), storageUUID, unitUUID)
	c.Check(err, tc.ErrorIs, applicationerrors.StorageInstanceAlreadyAttachedToUnit)
}

// TestAttachStorageUnexpectedAttachments ensures non-shared attachments return
// [applicationerrors.StorageInstanceUnexpectedAttachments].
func (s *attachStorageSuite) TestAttachStorageUnexpectedAttachments(c *tc.C) {
	defer s.setupMocks(c).Finish()

	storageUUID := tc.Must(c, domainstorage.NewStorageInstanceUUID)
	unitUUID := tc.Must(c, coreunit.NewUUID)
	otherUnitUUID := tc.Must(c, coreunit.NewUUID)
	attachmentUUID := tc.Must(c, domainstorage.NewStorageAttachmentUUID)
	attachInfo := makeValidAttachInfo(c, unitUUID, storageUUID)
	attachInfo.UnitNamedStorageInfo.CharmStorageDefinitionForValidation.Shared = false
	attachInfo.StorageInstanceAttachments = []internal.StorageInstanceUnitAttachment{
		{
			UnitUUID: otherUnitUUID,
			UUID:     attachmentUUID,
		},
	}

	s.state.EXPECT().GetStorageAttachInfoByUnitUUIDAndStorageUUID(
		gomock.Any(), unitUUID, storageUUID,
	).Return(
		attachInfo,
		nil,
	)

	err := s.service.AttachStorageToUnit(c.Context(), storageUUID, unitUUID)
	c.Check(err, tc.ErrorIs, applicationerrors.StorageInstanceUnexpectedAttachments)
}

// TestAttachStorageMachineOwnerMismatch ensures owning machine mismatch returns
// [applicationerrors.StorageInstanceAttachMachineOwnerMismatch].
func (s *attachStorageSuite) TestAttachStorageMachineOwnerMismatch(c *tc.C) {
	defer s.setupMocks(c).Finish()

	storageUUID := tc.Must(c, domainstorage.NewStorageInstanceUUID)
	unitUUID := tc.Must(c, coreunit.NewUUID)
	owningMachineUUID := tc.Must(c, coremachine.NewUUID)
	attachInfo := makeValidAttachInfo(c, unitUUID, storageUUID)
	attachInfo.StorageInstanceInfo.Filesystem.OwningMachineUUID = &owningMachineUUID

	s.state.EXPECT().GetStorageAttachInfoByUnitUUIDAndStorageUUID(
		gomock.Any(), unitUUID, storageUUID,
	).Return(
		attachInfo,
		nil,
	)

	err := s.service.AttachStorageToUnit(c.Context(), storageUUID, unitUUID)
	c.Check(err, tc.ErrorIs, applicationerrors.StorageInstanceAttachMachineOwnerMismatch)
}

// TestAttachStorageMachineOwnerDifferentUnitMachine ensures a unit machine
// mismatch returns [applicationerrors.StorageInstanceAttachMachineOwnerMismatch].
func (s *attachStorageSuite) TestAttachStorageMachineOwnerDifferentUnitMachine(c *tc.C) {
	defer s.setupMocks(c).Finish()

	storageUUID := tc.Must(c, domainstorage.NewStorageInstanceUUID)
	unitUUID := tc.Must(c, coreunit.NewUUID)
	unitMachineUUID := tc.Must(c, coremachine.NewUUID)
	owningMachineUUID := tc.Must(c, coremachine.NewUUID)
	attachInfo := makeValidAttachInfo(c, unitUUID, storageUUID)
	attachInfo.UnitNamedStorageInfo.MachineUUID = &unitMachineUUID
	attachInfo.StorageInstanceInfo.Filesystem.OwningMachineUUID = &owningMachineUUID

	s.state.EXPECT().GetStorageAttachInfoByUnitUUIDAndStorageUUID(
		gomock.Any(), unitUUID, storageUUID,
	).Return(
		attachInfo,
		nil,
	)

	err := s.service.AttachStorageToUnit(c.Context(), storageUUID, unitUUID)
	c.Check(err, tc.ErrorIs, applicationerrors.StorageInstanceAttachMachineOwnerMismatch)
}

// TestAttachStorageMachineOwnerWithUnsetUnitMachine ensures a storage instance
// owning machine with no unit machine returns
// [applicationerrors.StorageInstanceAttachMachineOwnerMismatch].
func (s *attachStorageSuite) TestAttachStorageMachineOwnerWithUnsetUnitMachine(c *tc.C) {
	defer s.setupMocks(c).Finish()

	storageUUID := tc.Must(c, domainstorage.NewStorageInstanceUUID)
	unitUUID := tc.Must(c, coreunit.NewUUID)
	owningMachineUUID := tc.Must(c, coremachine.NewUUID)
	attachInfo := makeValidAttachInfo(c, unitUUID, storageUUID)
	attachInfo.UnitNamedStorageInfo.MachineUUID = nil
	attachInfo.StorageInstanceInfo.Filesystem.OwningMachineUUID = &owningMachineUUID

	s.state.EXPECT().GetStorageAttachInfoByUnitUUIDAndStorageUUID(
		gomock.Any(), unitUUID, storageUUID,
	).Return(
		attachInfo,
		nil,
	)

	err := s.service.AttachStorageToUnit(c.Context(), storageUUID, unitUUID)
	c.Check(err, tc.ErrorIs, applicationerrors.StorageInstanceAttachMachineOwnerMismatch)
}

// TestAttachStorageStateStorageInstanceNotFound ensures state-level validation
// failures return [storageerrors.StorageInstanceNotFound].
func (s *attachStorageSuite) TestAttachStorageStateStorageInstanceNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.assertAttachStorageStateError(c, storageerrors.StorageInstanceNotFound)
}

// TestAttachStorageStateStorageInstanceNotAlive ensures state-level validation
// failures return [storageerrors.StorageInstanceNotAlive].
func (s *attachStorageSuite) TestAttachStorageStateStorageInstanceNotAlive(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.assertAttachStorageStateError(c, storageerrors.StorageInstanceNotAlive)
}

// TestAttachStorageStateUnitNotFound ensures state-level validation failures
// return [applicationerrors.UnitNotFound].
func (s *attachStorageSuite) TestAttachStorageStateUnitNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.assertAttachStorageStateError(c, applicationerrors.UnitNotFound)
}

// TestAttachStorageStateUnitNotAlive ensures state-level validation failures
// return [applicationerrors.UnitNotAlive].
func (s *attachStorageSuite) TestAttachStorageStateUnitNotAlive(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.assertAttachStorageStateError(c, applicationerrors.UnitNotAlive)
}

// TestAttachStorageStateAlreadyAttached ensures state-level validation failures
// return [applicationerrors.StorageInstanceAlreadyAttachedToUnit].
func (s *attachStorageSuite) TestAttachStorageStateAlreadyAttached(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.assertAttachStorageStateError(c, applicationerrors.StorageInstanceAlreadyAttachedToUnit)
}

// TestAttachStorageStateAttachmentCountExceeded ensures state-level validation
// failures return [applicationerrors.UnitAttachmentCountExceedsLimit].
func (s *attachStorageSuite) TestAttachStorageStateAttachmentCountExceeded(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.assertAttachStorageStateError(c, applicationerrors.UnitAttachmentCountExceedsLimit)
}

// TestAttachStorageStateCharmChanged ensures state-level validation failures
// return [applicationerrors.UnitCharmChanged].
func (s *attachStorageSuite) TestAttachStorageStateCharmChanged(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.assertAttachStorageStateError(c, applicationerrors.UnitCharmChanged)
}

// TestAttachStorageStateMachineChanged ensures state-level validation failures
// return [applicationerrors.UnitMachineChanged].
func (s *attachStorageSuite) TestAttachStorageStateMachineChanged(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.assertAttachStorageStateError(c, applicationerrors.UnitMachineChanged)
}

// TestAttachStorageStateUnexpectedAttachments ensures state-level validation
// failures return [applicationerrors.StorageInstanceUnexpectedAttachments].
func (s *attachStorageSuite) TestAttachStorageStateUnexpectedAttachments(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.assertAttachStorageStateError(c, applicationerrors.StorageInstanceUnexpectedAttachments)
}

// TestAttachStorageSuccess ensures a valid attach request is passed through to
// state with the generated arguments.
func (s *attachStorageSuite) TestAttachStorageSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	storageUUID := tc.Must(c, domainstorage.NewStorageInstanceUUID)
	unitUUID := tc.Must(c, coreunit.NewUUID)
	attachmentUUID := tc.Must(c, domainstorage.NewStorageAttachmentUUID)
	existingAttachmentUUID := tc.Must(c, domainstorage.NewStorageAttachmentUUID)
	otherUnitUUID := tc.Must(c, coreunit.NewUUID)
	filesystemAttachmentUUID := tc.Must(c, domainstorage.NewFilesystemAttachmentUUID)
	volumeUUID := tc.Must(c, domainstorage.NewVolumeUUID)
	volumeAttachmentUUID := tc.Must(c, domainstorage.NewVolumeAttachmentUUID)
	unitMachineUUID := tc.Must(c, coremachine.NewUUID)
	attachInfo := makeValidAttachInfo(c, unitUUID, storageUUID)
	attachInfo.StorageInstanceInfo.CharmName = nil
	attachInfo.StorageInstanceInfo.Volume = &internal.StorageInstanceVolumeInfo{
		UUID: volumeUUID,
		Size: 1024,
	}
	attachInfo.StorageInstanceAttachments = []internal.StorageInstanceUnitAttachment{
		{
			UnitUUID: otherUnitUUID,
			UUID:     existingAttachmentUUID,
		},
	}
	attachInfo.UnitNamedStorageInfo.MachineUUID = &unitMachineUUID
	providerID := "volume-attachment-provider-id"
	attachArg := internal.AttachStorageInstanceToUnitArg{
		CreateStorageInstanceAttachmentArg: internal.CreateStorageInstanceAttachmentArg{
			UUID: attachmentUUID,
			FilesystemAttachment: &internal.CreateUnitStorageFilesystemAttachmentArg{
				FilesystemUUID: attachInfo.StorageInstanceInfo.Filesystem.UUID,
				NetNodeUUID:    attachInfo.UnitNamedStorageInfo.NetNodeUUID,
				ProvisionScope: storageprovisioning.ProvisionScopeMachine,
				UUID:           filesystemAttachmentUUID,
			},
			StorageInstanceUUID: storageUUID,
			VolumeAttachment: &internal.CreateUnitStorageVolumeAttachmentArg{
				NetNodeUUID:    attachInfo.UnitNamedStorageInfo.NetNodeUUID,
				ProvisionScope: storageprovisioning.ProvisionScopeModel,
				VolumeUUID:     volumeUUID,
				UUID:           volumeAttachmentUUID,
				ProviderID:     &providerID,
			},
		},
		StorageInstanceAttachmentCheckArgs: internal.StorageInstanceAttachmentCheckArgs{
			ExpectedAttachments: []domainstorage.StorageAttachmentUUID{
				existingAttachmentUUID,
			},
			UUID: storageUUID,
		},
		StorageInstanceCharmNameSetArg: &internal.StorageInstanceCharmNameSetArg{
			CharmMetadataName: attachInfo.UnitNamedStorageInfo.CharmMetadataName,
			UUID:              storageUUID,
		},
		UnitStorageInstanceAttachmentCheckArgs: internal.UnitStorageInstanceAttachmentCheckArgs{
			CountLessThanEqual: attachInfo.UnitNamedStorageInfo.AlreadyAttachedCount,
			CharmUUID:          attachInfo.UnitNamedStorageInfo.CharmUUID,
			MachineUUID:        &unitMachineUUID,
		},
	}

	s.state.EXPECT().GetStorageAttachInfoByUnitUUIDAndStorageUUID(
		gomock.Any(), unitUUID, storageUUID,
	).Return(
		attachInfo,
		nil,
	)
	s.storageService.EXPECT().MakeAttachStorageInstanceToUnitArg(
		gomock.Any(),
		attachInfo,
	).Return(
		attachArg,
		nil,
	)
	s.state.EXPECT().AttachStorageToUnit(
		gomock.Any(), unitUUID, attachArg,
	).Return(
		nil,
	)

	err := s.service.AttachStorageToUnit(c.Context(), storageUUID, unitUUID)
	c.Assert(err, tc.ErrorIsNil)
}

// TestAttachStorageSuccessUnitMachineOnly ensures attachments succeed when the
// unit has a machine UUID but the storage instance has no owning machine.
func (s *attachStorageSuite) TestAttachStorageSuccessUnitMachineOnly(c *tc.C) {
	defer s.setupMocks(c).Finish()

	storageUUID := tc.Must(c, domainstorage.NewStorageInstanceUUID)
	unitUUID := tc.Must(c, coreunit.NewUUID)
	unitMachineUUID := tc.Must(c, coremachine.NewUUID)
	attachInfo := makeValidAttachInfo(c, unitUUID, storageUUID)
	attachInfo.UnitNamedStorageInfo.MachineUUID = &unitMachineUUID
	attachInfo.StorageInstanceInfo.Filesystem.OwningMachineUUID = nil

	attachArg := internal.AttachStorageInstanceToUnitArg{
		CreateStorageInstanceAttachmentArg: internal.CreateStorageInstanceAttachmentArg{
			StorageInstanceUUID: storageUUID,
		},
	}

	s.state.EXPECT().GetStorageAttachInfoByUnitUUIDAndStorageUUID(
		gomock.Any(), unitUUID, storageUUID,
	).Return(
		attachInfo,
		nil,
	)
	s.storageService.EXPECT().MakeAttachStorageInstanceToUnitArg(
		gomock.Any(),
		attachInfo,
	).Return(
		attachArg,
		nil,
	)
	s.state.EXPECT().AttachStorageToUnit(
		gomock.Any(), unitUUID, attachArg,
	).Return(
		nil,
	)

	err := s.service.AttachStorageToUnit(c.Context(), storageUUID, unitUUID)
	c.Assert(err, tc.ErrorIsNil)
}

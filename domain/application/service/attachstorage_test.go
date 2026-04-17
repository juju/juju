// Copyright 2026 Canonical Ltd.
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
	domainlife "github.com/juju/juju/domain/life"
	domainnetwork "github.com/juju/juju/domain/network"
	domainstorage "github.com/juju/juju/domain/storage"
	storageerrors "github.com/juju/juju/domain/storage/errors"
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
) domainstorage.StorageInstanceInfoForUnitAttach {
	charmName := "example"
	charmUUID := tc.Must(c, corecharm.NewID)
	netNodeUUID := tc.Must(c, domainnetwork.NewNetNodeUUID)
	filesystemUUID := tc.Must(c, domainstorage.NewFilesystemUUID)

	return domainstorage.StorageInstanceInfoForUnitAttach{
		StorageInstanceInfoForAttach: domainstorage.StorageInstanceInfoForAttach{
			StorageInstanceAttachInfo: domainstorage.StorageInstanceAttachInfo{
				UUID:      storageUUID,
				CharmName: &charmName,
				Filesystem: &domainstorage.StorageInstanceAttachFilesystemInfo{
					SizeMib: 1024,
					UUID:    filesystemUUID,
				},
				Kind:             domainstorage.StorageKindFilesystem,
				Life:             domainlife.Alive,
				RequestedSizeMIB: 1024,
				StorageName:      "data",
			},
		},
		UnitAttachNamedStorageInfo: domainstorage.UnitAttachNamedStorageInfo{
			CharmStorageDefinition: domainstorage.CharmStorageDefinition{
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
	}
}

// assertAttachStorageStateError ensures AttachStorageInstanceToUnit surfaces the error
// returned by state after validation succeeds.
func (s *attachStorageSuite) assertAttachStorageStateError(
	c *tc.C,
	stateErr error,
) {
	storageUUID := tc.Must(c, domainstorage.NewStorageInstanceUUID)
	unitUUID := tc.Must(c, coreunit.NewUUID)
	attachInfo := makeValidAttachInfo(c, unitUUID, storageUUID)
	attachArg := domainstorage.AttachStorageInstanceToUnitArg{
		CreateUnitStorageAttachmentArg: domainstorage.CreateUnitStorageAttachmentArg{
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
	s.state.EXPECT().AttachStorageInstanceToUnit(
		gomock.Any(), unitUUID, attachArg,
	).Return(
		stateErr,
	)

	err := s.service.AttachStorageInstanceToUnit(c.Context(), storageUUID, unitUUID)
	c.Check(err, tc.ErrorIs, stateErr)
}

// TestAttachStorageInvalidStorageUUID ensures invalid storage UUIDs are
// rejected with a [coreerrors.NotValid] error.
func (s *attachStorageSuite) TestAttachStorageInvalidStorageUUID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	storageUUID := domainstorage.StorageInstanceUUID("not-a-uuid")
	unitUUID := tc.Must(c, coreunit.NewUUID)

	err := s.service.AttachStorageInstanceToUnit(c.Context(), storageUUID, unitUUID)
	c.Check(err, tc.ErrorIs, coreerrors.NotValid)
}

// TestAttachStorageInvalidUnitUUID ensures invalid unit UUIDs are rejected
// with a [coreerrors.NotValid] error.
func (s *attachStorageSuite) TestAttachStorageInvalidUnitUUID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	storageUUID := tc.Must(c, domainstorage.NewStorageInstanceUUID)
	unitUUID := coreunit.UUID("not-a-uuid")

	err := s.service.AttachStorageInstanceToUnit(c.Context(), storageUUID, unitUUID)
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
		domainstorage.StorageInstanceInfoForUnitAttach{},
		storageerrors.StorageInstanceNotFound,
	)

	err := s.service.AttachStorageInstanceToUnit(c.Context(), storageUUID, unitUUID)
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
		domainstorage.StorageInstanceInfoForUnitAttach{},
		applicationerrors.UnitNotFound,
	)

	err := s.service.AttachStorageInstanceToUnit(c.Context(), storageUUID, unitUUID)
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
		domainstorage.StorageInstanceInfoForUnitAttach{},
		applicationerrors.StorageNameNotSupported,
	)

	err := s.service.AttachStorageInstanceToUnit(c.Context(), storageUUID, unitUUID)
	c.Check(err, tc.ErrorIs, applicationerrors.StorageNameNotSupported)
}

// TestAttachStorageStorageInstanceNotAlive ensures a non-alive storage instance
// returns [storageerrors.StorageInstanceNotAlive].
func (s *attachStorageSuite) TestAttachStorageStorageInstanceNotAlive(c *tc.C) {
	defer s.setupMocks(c).Finish()

	storageUUID := tc.Must(c, domainstorage.NewStorageInstanceUUID)
	unitUUID := tc.Must(c, coreunit.NewUUID)
	attachInfo := makeValidAttachInfo(c, unitUUID, storageUUID)
	attachInfo.StorageInstanceAttachInfo.Life = domainlife.Dying

	s.state.EXPECT().GetStorageAttachInfoByUnitUUIDAndStorageUUID(
		gomock.Any(), unitUUID, storageUUID,
	).Return(
		attachInfo,
		nil,
	)

	err := s.service.AttachStorageInstanceToUnit(c.Context(), storageUUID, unitUUID)
	c.Check(err, tc.ErrorIs, storageerrors.StorageInstanceNotAlive)
}

// TestAttachStorageUnitNotAlive ensures a non-alive unit returns
// [applicationerrors.UnitNotAlive].
func (s *attachStorageSuite) TestAttachStorageUnitNotAlive(c *tc.C) {
	defer s.setupMocks(c).Finish()

	storageUUID := tc.Must(c, domainstorage.NewStorageInstanceUUID)
	unitUUID := tc.Must(c, coreunit.NewUUID)
	attachInfo := makeValidAttachInfo(c, unitUUID, storageUUID)
	attachInfo.UnitAttachNamedStorageInfo.Life = domainlife.Dying

	s.state.EXPECT().GetStorageAttachInfoByUnitUUIDAndStorageUUID(
		gomock.Any(), unitUUID, storageUUID,
	).Return(
		attachInfo,
		nil,
	)

	err := s.service.AttachStorageInstanceToUnit(c.Context(), storageUUID, unitUUID)
	c.Check(err, tc.ErrorIs, applicationerrors.UnitNotAlive)
}

// TestAttachStorageCharmNameMismatch ensures a charm name mismatch returns
// [applicationerrors.StorageInstanceCharmNameMismatch].
func (s *attachStorageSuite) TestAttachStorageCharmNameMismatch(c *tc.C) {
	defer s.setupMocks(c).Finish()

	storageUUID := tc.Must(c, domainstorage.NewStorageInstanceUUID)
	unitUUID := tc.Must(c, coreunit.NewUUID)
	attachInfo := makeValidAttachInfo(c, unitUUID, storageUUID)
	attachInfo.StorageInstanceAttachInfo.CharmName = new("other")

	s.state.EXPECT().GetStorageAttachInfoByUnitUUIDAndStorageUUID(
		gomock.Any(), unitUUID, storageUUID,
	).Return(
		attachInfo,
		nil,
	)

	err := s.service.AttachStorageInstanceToUnit(c.Context(), storageUUID, unitUUID)
	c.Check(err, tc.ErrorIs, applicationerrors.StorageInstanceCharmNameMismatch)
}

// TestAttachStorageKindMismatch ensures a kind mismatch returns
// [applicationerrors.StorageInstanceKindNotValidForCharmStorageDefinition].
func (s *attachStorageSuite) TestAttachStorageKindMismatch(c *tc.C) {
	defer s.setupMocks(c).Finish()

	storageUUID := tc.Must(c, domainstorage.NewStorageInstanceUUID)
	unitUUID := tc.Must(c, coreunit.NewUUID)
	attachInfo := makeValidAttachInfo(c, unitUUID, storageUUID)
	attachInfo.UnitAttachNamedStorageInfo.CharmStorageDefinition.Type =
		applicationcharm.StorageBlock

	s.state.EXPECT().GetStorageAttachInfoByUnitUUIDAndStorageUUID(
		gomock.Any(), unitUUID, storageUUID,
	).Return(
		attachInfo,
		nil,
	)

	err := s.service.AttachStorageInstanceToUnit(c.Context(), storageUUID, unitUUID)
	c.Check(err, tc.ErrorIs, applicationerrors.StorageInstanceKindNotValidForCharmStorageDefinition)
}

// TestAttachStorageSizeBelowMinimum ensures a size below the minimum returns
// [applicationerrors.StorageInstanceSizeNotValidForCharmStorageDefinition].
func (s *attachStorageSuite) TestAttachStorageSizeBelowMinimum(c *tc.C) {
	defer s.setupMocks(c).Finish()

	storageUUID := tc.Must(c, domainstorage.NewStorageInstanceUUID)
	unitUUID := tc.Must(c, coreunit.NewUUID)
	attachInfo := makeValidAttachInfo(c, unitUUID, storageUUID)
	attachInfo.StorageInstanceAttachInfo.Filesystem.SizeMib = 1
	attachInfo.StorageInstanceAttachInfo.RequestedSizeMIB = 1
	attachInfo.UnitAttachNamedStorageInfo.CharmStorageDefinition.MinimumSize = 2

	s.state.EXPECT().GetStorageAttachInfoByUnitUUIDAndStorageUUID(
		gomock.Any(), unitUUID, storageUUID,
	).Return(
		attachInfo,
		nil,
	)

	err := s.service.AttachStorageInstanceToUnit(c.Context(), storageUUID, unitUUID)
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
	attachInfo.StorageInstanceAttachInfo.Filesystem.SizeMib = 0
	attachInfo.StorageInstanceAttachInfo.RequestedSizeMIB = 1
	attachInfo.UnitAttachNamedStorageInfo.CharmStorageDefinition.MinimumSize = 2

	s.state.EXPECT().GetStorageAttachInfoByUnitUUIDAndStorageUUID(
		gomock.Any(), unitUUID, storageUUID,
	).Return(
		attachInfo,
		nil,
	)

	err := s.service.AttachStorageInstanceToUnit(c.Context(), storageUUID, unitUUID)
	c.Check(err, tc.ErrorIs, applicationerrors.StorageInstanceSizeNotValidForCharmStorageDefinition)
}

// TestAttachStorageCountLimitExceeded ensures exceeding the charm max returns
// [applicationerrors.StorageCountLimitExceeded].
func (s *attachStorageSuite) TestAttachStorageCountLimitExceeded(c *tc.C) {
	defer s.setupMocks(c).Finish()

	storageUUID := tc.Must(c, domainstorage.NewStorageInstanceUUID)
	unitUUID := tc.Must(c, coreunit.NewUUID)
	attachInfo := makeValidAttachInfo(c, unitUUID, storageUUID)
	attachInfo.UnitAttachNamedStorageInfo.CharmStorageDefinition.CountMax = 0
	attachInfo.UnitAttachNamedStorageInfo.AlreadyAttachedCount = 0

	s.state.EXPECT().GetStorageAttachInfoByUnitUUIDAndStorageUUID(
		gomock.Any(), unitUUID, storageUUID,
	).Return(
		attachInfo,
		nil,
	)

	err := s.service.AttachStorageInstanceToUnit(c.Context(), storageUUID, unitUUID)
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
	attachInfo.StorageInstanceAttachments = []domainstorage.StorageInstanceUnitAttachmentID{
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

	err := s.service.AttachStorageInstanceToUnit(c.Context(), storageUUID, unitUUID)
	c.Check(err, tc.ErrorIs, applicationerrors.StorageInstanceAlreadyAttachedToUnit)
}

// TestAttachStorageUnexpectedAttachments ensures non-shared attachments return
// [applicationerrors.StorageInstanceAttachSharedAccessNotSupported].
func (s *attachStorageSuite) TestAttachStorageUnexpectedAttachments(c *tc.C) {
	defer s.setupMocks(c).Finish()

	storageUUID := tc.Must(c, domainstorage.NewStorageInstanceUUID)
	unitUUID := tc.Must(c, coreunit.NewUUID)
	otherUnitUUID := tc.Must(c, coreunit.NewUUID)
	attachmentUUID := tc.Must(c, domainstorage.NewStorageAttachmentUUID)
	attachInfo := makeValidAttachInfo(c, unitUUID, storageUUID)
	attachInfo.UnitAttachNamedStorageInfo.CharmStorageDefinition.Shared = false
	attachInfo.StorageInstanceAttachments = []domainstorage.StorageInstanceUnitAttachmentID{
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

	err := s.service.AttachStorageInstanceToUnit(c.Context(), storageUUID, unitUUID)
	c.Check(err, tc.ErrorIs, applicationerrors.StorageInstanceAttachSharedAccessNotSupported)
}

// TestAttachStorageMachineOwnerMismatch ensures owning machine mismatch returns
// [applicationerrors.StorageInstanceAttachMachineOwnerMismatch].
func (s *attachStorageSuite) TestAttachStorageMachineOwnerMismatch(c *tc.C) {
	defer s.setupMocks(c).Finish()

	storageUUID := tc.Must(c, domainstorage.NewStorageInstanceUUID)
	unitUUID := tc.Must(c, coreunit.NewUUID)
	owningMachineUUID := tc.Must(c, coremachine.NewUUID)
	attachInfo := makeValidAttachInfo(c, unitUUID, storageUUID)
	attachInfo.StorageInstanceAttachInfo.Filesystem.OwningMachineUUID = &owningMachineUUID

	s.state.EXPECT().GetStorageAttachInfoByUnitUUIDAndStorageUUID(
		gomock.Any(), unitUUID, storageUUID,
	).Return(
		attachInfo,
		nil,
	)

	err := s.service.AttachStorageInstanceToUnit(c.Context(), storageUUID, unitUUID)
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
	attachInfo.UnitAttachNamedStorageInfo.MachineUUID = &unitMachineUUID
	attachInfo.StorageInstanceAttachInfo.Filesystem.OwningMachineUUID = &owningMachineUUID

	s.state.EXPECT().GetStorageAttachInfoByUnitUUIDAndStorageUUID(
		gomock.Any(), unitUUID, storageUUID,
	).Return(
		attachInfo,
		nil,
	)

	err := s.service.AttachStorageInstanceToUnit(c.Context(), storageUUID, unitUUID)
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
	attachInfo.UnitAttachNamedStorageInfo.MachineUUID = nil
	attachInfo.StorageInstanceAttachInfo.Filesystem.OwningMachineUUID = &owningMachineUUID

	s.state.EXPECT().GetStorageAttachInfoByUnitUUIDAndStorageUUID(
		gomock.Any(), unitUUID, storageUUID,
	).Return(
		attachInfo,
		nil,
	)

	err := s.service.AttachStorageInstanceToUnit(c.Context(), storageUUID, unitUUID)
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

// TestAttachStorageStateAttachmentCountExceeded ensures that when state returns
// [applicationerrors.UnitAttachmentCountExceedsLimit], the service transforms it
// into a [applicationerrors.StorageCountLimitExceeded] with contextual details.
func (s *attachStorageSuite) TestAttachStorageStateAttachmentCountExceeded(c *tc.C) {
	defer s.setupMocks(c).Finish()

	storageUUID := tc.Must(c, domainstorage.NewStorageInstanceUUID)
	unitUUID := tc.Must(c, coreunit.NewUUID)
	attachInfo := makeValidAttachInfo(c, unitUUID, storageUUID)
	attachArg := domainstorage.AttachStorageInstanceToUnitArg{
		CreateUnitStorageAttachmentArg: domainstorage.CreateUnitStorageAttachmentArg{
			StorageInstanceUUID: storageUUID,
		},
	}

	s.state.EXPECT().GetStorageAttachInfoByUnitUUIDAndStorageUUID(
		gomock.Any(), unitUUID, storageUUID,
	).Return(attachInfo, nil)
	s.storageService.EXPECT().MakeAttachStorageInstanceToUnitArg(
		gomock.Any(), attachInfo,
	).Return(attachArg, nil)
	s.state.EXPECT().AttachStorageInstanceToUnit(
		gomock.Any(), unitUUID, attachArg,
	).Return(applicationerrors.UnitAttachmentCountExceedsLimit)

	err := s.service.AttachStorageInstanceToUnit(c.Context(), storageUUID, unitUUID)
	errVal, is := errors.AsType[applicationerrors.StorageCountLimitExceeded](err)
	c.Check(is, tc.IsTrue)
	c.Check(errVal, tc.DeepEquals, applicationerrors.StorageCountLimitExceeded{
		Maximum:     nil,
		Minimum:     0,
		Requested:   1,
		StorageName: "data",
	})
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
	attachInfo.StorageInstanceAttachInfo.CharmName = nil
	attachInfo.StorageInstanceAttachInfo.Volume =
		&domainstorage.StorageInstanceAttachVolumeInfo{
			UUID:    volumeUUID,
			SizeMiB: 1024,
		}
	attachInfo.StorageInstanceAttachments = []domainstorage.StorageInstanceUnitAttachmentID{
		{
			UnitUUID: otherUnitUUID,
			UUID:     existingAttachmentUUID,
		},
	}
	attachInfo.UnitAttachNamedStorageInfo.MachineUUID = &unitMachineUUID
	providerID := "volume-attachment-provider-id"
	attachArg := domainstorage.AttachStorageInstanceToUnitArg{
		CreateUnitStorageAttachmentArg: domainstorage.CreateUnitStorageAttachmentArg{
			UUID: attachmentUUID,
			FilesystemAttachment: &domainstorage.CreateUnitStorageFilesystemAttachmentArg{
				FilesystemUUID: attachInfo.StorageInstanceAttachInfo.Filesystem.UUID,
				NetNodeUUID:    attachInfo.UnitAttachNamedStorageInfo.NetNodeUUID,
				ProvisionScope: domainstorage.ProvisionScopeMachine,
				UUID:           filesystemAttachmentUUID,
			},
			StorageInstanceUUID: storageUUID,
			VolumeAttachment: &domainstorage.CreateUnitStorageVolumeAttachmentArg{
				NetNodeUUID:    attachInfo.UnitAttachNamedStorageInfo.NetNodeUUID,
				ProvisionScope: domainstorage.ProvisionScopeModel,
				VolumeUUID:     volumeUUID,
				UUID:           volumeAttachmentUUID,
				ProviderID:     &providerID,
			},
		},
		StorageInstanceAttachmentCheckArgs: domainstorage.StorageInstanceAttachmentCheckArgs{
			ExpectedAttachments: []domainstorage.StorageAttachmentUUID{
				existingAttachmentUUID,
			},
			UUID: storageUUID,
		},
		StorageInstanceCharmNameSetArg: &domainstorage.StorageInstanceCharmNameSetArg{
			CharmMetadataName: attachInfo.UnitAttachNamedStorageInfo.CharmMetadataName,
			UUID:              storageUUID,
		},
		UnitStorageInstanceAttachmentCheckArgs: domainstorage.UnitStorageInstanceAttachmentCheckArgs{
			CountLessThanEqual: attachInfo.UnitAttachNamedStorageInfo.AlreadyAttachedCount,
			CharmUUID:          attachInfo.UnitAttachNamedStorageInfo.CharmUUID,
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
	s.state.EXPECT().AttachStorageInstanceToUnit(
		gomock.Any(), unitUUID, attachArg,
	).Return(
		nil,
	)

	err := s.service.AttachStorageInstanceToUnit(c.Context(), storageUUID, unitUUID)
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
	attachInfo.UnitAttachNamedStorageInfo.MachineUUID = &unitMachineUUID
	attachInfo.StorageInstanceAttachInfo.Filesystem.OwningMachineUUID = nil

	attachArg := domainstorage.AttachStorageInstanceToUnitArg{
		CreateUnitStorageAttachmentArg: domainstorage.CreateUnitStorageAttachmentArg{
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
	s.state.EXPECT().AttachStorageInstanceToUnit(
		gomock.Any(), unitUUID, attachArg,
	).Return(
		nil,
	)

	err := s.service.AttachStorageInstanceToUnit(c.Context(), storageUUID, unitUUID)
	c.Assert(err, tc.ErrorIsNil)
}

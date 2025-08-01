// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	"testing"

	"github.com/juju/tc"
)

type storageSuite struct {
	baseStorageSuite
}

func TestStorageSuite(t *testing.T) {
	tc.Run(t, &storageSuite{})
}

func (s *storageSuite) TestStub(c *tc.C) {
	c.Skip(`This suite is missing tests for the following scenerios:
- ListStorageDetails but retrieving units returns an error (is this a useful test?)
- ListStorageDetails but retrieving the unit's storage attachements returns an error (is this a useful test?)
- TestStorageListEmpty
- TestStorageListFilesystem
- TestStorageListVolume
- TestStorageListError
- TestStorageListInstanceError
- TestStorageListFilesystemError
- TestShowStorageEmpty
- TestShowStorageInvalidTag
- TestShowStorage
- TestShowStorageInvalidId
- TestRemove
- TestDetach
- TestDetachSpecifiedNotFound
- TestDetachAttachmentNotFoundConcurrent
- TestDetachNoAttachmentsStorageNotFound
- TestAttach
- TestImportFilesystem
- TestImportFilesystemVolumeBacked
- TestImportFilesystemError
- TestImportFilesystemNotSupported
- TestImportFilesystemK8sProvider
- TestImportFilesystemVolumeBackedNotSupported
- TestImportValidationErrors
- TestListStorageAsAdminOnNotOwnedModel
- TestListStorageAsNonAdminOnNotOwnedModel
`)
}

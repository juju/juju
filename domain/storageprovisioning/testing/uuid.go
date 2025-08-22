// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"github.com/juju/tc"

	"github.com/juju/juju/domain/storageprovisioning"
)

func GenStorageAttachmentUUID(c *tc.C) storageprovisioning.StorageAttachmentUUID {
	uuid, err := storageprovisioning.NewStorageAttachmentUUID()
	c.Assert(err, tc.ErrorIsNil)
	return uuid
}

// GenFilesystemUUID generates a new [storageprovisioning.FilesystemUUID] for
// testing purposes.
func GenFilesystemUUID(c *tc.C) storageprovisioning.FilesystemUUID {
	uuid, err := storageprovisioning.NewFilesystemUUID()
	c.Assert(err, tc.ErrorIsNil)
	return uuid
}

// GenFilesystemAttachmentUUID generates a new
// [storageprovisioning.FilesystemAttachmentUUID] for testing purposes.
func GenFilesystemAttachmentUUID(c *tc.C) storageprovisioning.FilesystemAttachmentUUID {
	uuid, err := storageprovisioning.NewFilesystemAttachmentUUID()
	c.Assert(err, tc.ErrorIsNil)
	return uuid
}

// GenVolumeUUID generates a new [storageprovisioning.VolumeUUID] for
// testing purposes.
func GenVolumeUUID(c *tc.C) storageprovisioning.VolumeUUID {
	uuid, err := storageprovisioning.NewVolumeUUID()
	c.Assert(err, tc.ErrorIsNil)
	return uuid
}

// GenVolumeAttachmentUUID generates a new
// [storageprovisioning.VolumeAttachmentUUID] for testing purposes.
func GenVolumeAttachmentUUID(c *tc.C) storageprovisioning.VolumeAttachmentUUID {
	uuid, err := storageprovisioning.NewVolumeAttachmentUUID()
	c.Assert(err, tc.ErrorIsNil)
	return uuid
}

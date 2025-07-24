// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"github.com/juju/tc"

	storageprovisioning "github.com/juju/juju/domain/storageprovisioning"
)

// GenFilesystemUUID generates a new FilesystemUUID for testing purposes.
func GenFilesystemUUID(c *tc.C) storageprovisioning.FilesystemUUID {
	uuid, err := storageprovisioning.NewFileystemUUID()
	c.Assert(err, tc.IsNil)
	return uuid
}

// GenFilesystemAttachmentUUID generates a new FilesystemAttachmentUUID for
// testing purposes.
func GenFilesystemAttachmentUUID(c *tc.C) storageprovisioning.FilesystemAttachmentUUID {
	uuid, err := storageprovisioning.NewFilesystemAttachmentUUID()
	c.Assert(err, tc.IsNil)
	return uuid
}

// GenVolumeUUID generates a new VolumeUUID for testing purposes.
func GenVolumeUUID(c *tc.C) storageprovisioning.VolumeUUID {
	uuid, err := storageprovisioning.NewVolumeUUID()
	c.Assert(err, tc.IsNil)
	return uuid
}

// GenVolumeAttachmentUUID generates a new VolumeAttachmentUUID for testing
// purposes.
func GenVolumeAttachmentUUID(c *tc.C) storageprovisioning.VolumeAttachmentUUID {
	uuid, err := storageprovisioning.NewVolumeAttachmentUUID()
	c.Assert(err, tc.IsNil)
	return uuid
}

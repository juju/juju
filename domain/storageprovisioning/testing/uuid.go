// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"github.com/juju/tc"

	domainstorage "github.com/juju/juju/domain/storage"
	"github.com/juju/juju/domain/storageprovisioning"
)

// GenFilesystemUUID generates a new [storageprovisioning.FilesystemUUID] for
// testing purposes.
func GenFilesystemUUID(c *tc.C) storageprovisioning.FilesystemUUID {
	uuid, err := storageprovisioning.NewFilesystemUUID()
	c.Assert(err, tc.ErrorIsNil)
	return uuid
}

// GenVolumeAttachmentPlanUUID generates a new
// [storageprovisioning.VolumeAttachmentPlanUUID] for testing purposes.
func GenVolumeAttachmentPlanUUID(c *tc.C) domainstorage.VolumeAttachmentPlanUUID {
	uuid, err := domainstorage.NewVolumeAttachmentPlanUUID()
	c.Assert(err, tc.ErrorIsNil)
	return uuid
}

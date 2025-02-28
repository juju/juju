// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	corestorage "github.com/juju/juju/core/storage"
)

// GenStorageUUID can be used in testing for generating a storage uuid.
func GenStorageUUID(c *gc.C) corestorage.UUID {
	uuid, err := corestorage.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	return uuid
}

// GenFilesystemUUID can be used in testing for generating a filesystem uuid.
func GenFilesystemUUID(c *gc.C) corestorage.FilesystemUUID {
	uuid, err := corestorage.NewFilesystemUUID()
	c.Assert(err, jc.ErrorIsNil)
	return uuid
}

// GenFilesystemAttachmentUUID can be used in testing for generating a filesystem uuid.
func GenFilesystemAttachmentUUID(c *gc.C) corestorage.FilesystemAttachmentUUID {
	uuid, err := corestorage.NewFilesystemAttachmentUUID()
	c.Assert(err, jc.ErrorIsNil)
	return uuid
}

// GenVolumeUUID can be used in testing for generating a volume uuid.
func GenVolumeUUID(c *gc.C) corestorage.VolumeUUID {
	uuid, err := corestorage.NewVolumeUUID()
	c.Assert(err, jc.ErrorIsNil)
	return uuid
}

// GenVolumeAttachmentUUID can be used in testing for generating a volume uuid.
func GenVolumeAttachmentUUID(c *gc.C) corestorage.VolumeAttachmentUUID {
	uuid, err := corestorage.NewVolumeAttachmentUUID()
	c.Assert(err, jc.ErrorIsNil)
	return uuid
}

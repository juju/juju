// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"github.com/juju/tc"

	"github.com/juju/juju/domain/storageprovisioning"
)

// GenFilesystemUUID generates a new [storageprovisioning.FilesystemUUID] for
// testing purposes.
func GenFilesystemUUID(c *tc.C) domainstorage.FilesystemUUID {
	uuid, err := domainstorage.NewFilesystemUUID()
	c.Assert(err, tc.ErrorIsNil)
	return uuid
}

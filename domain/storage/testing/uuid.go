// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"github.com/juju/juju/domain/storage"
	"github.com/juju/tc"
)

// GenStorageInstanceUUID generates a new [storage.StorageInstanceUUID] for
// testing purposes.
func GenStorageInstanceUUID(c *tc.C) storage.StorageInstanceUUID {
	uuid, err := storage.NewStorageInstanceUUID()
	c.Assert(err, tc.ErrorIsNil)
	return uuid
}

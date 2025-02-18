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

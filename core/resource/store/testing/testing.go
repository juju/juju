// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/resource/store"
)

// GenFileResourceStoreID can be used in testing for generating a file
// resource store ID that is checked for subsequent errors using the test
// suit's go check instance.
func GenFileResourceStoreID(c *gc.C, uuid objectstore.UUID) store.ID {
	id, err := store.NewFileResourceID(uuid)
	c.Assert(err, jc.ErrorIsNil)
	return id
}

// GenContainerImageMetadataResourceID can be used in testing for generating a
// container image metadata resource store ID that is checked for subsequent
// errors using the test suit's go check instance.
func GenContainerImageMetadataResourceID(c *gc.C, storageKey string) store.ID {
	id, err := store.NewContainerImageMetadataResourceID(storageKey)
	c.Assert(err, jc.ErrorIsNil)
	return id
}

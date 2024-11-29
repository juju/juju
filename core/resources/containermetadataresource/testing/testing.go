// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/resources/containermetadataresource"
)

// GenContainerMetadataResourceUUID can be used in testing for generating a objectstore UUID
// that is checked for subsequent errors using the test suit's go check
// instance.
func GenContainerMetadataResourceUUID(c *gc.C) containermetadataresource.UUID {
	id, err := containermetadataresource.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	return id
}

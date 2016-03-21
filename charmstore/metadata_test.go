// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmstore_test

import (
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/charmstore"
)

type JujuMetadataSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&JujuMetadataSuite{})

func (s *JujuMetadataSuite) TestIsZeroNothingSet(c *gc.C) {
	var meta charmstore.JujuMetadata

	isZero := meta.IsZero()

	c.Check(isZero, jc.IsTrue)
}

func (s *JujuMetadataSuite) TestIsZeroHasUUID(c *gc.C) {
	uuidVal, err := utils.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	uuid := uuidVal.String()
	meta := charmstore.JujuMetadata{
		ModelUUID: uuid,
	}
	isZero := meta.IsZero()

	c.Check(isZero, jc.IsFalse)
}

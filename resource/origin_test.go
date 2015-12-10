// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resource_test

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/resource"
)

type OriginKindSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&OriginKindSuite{})

func (OriginKindSuite) TestValidateKnown(c *gc.C) {
	recognized := []resource.OriginKind{
		resource.OriginKindUpload,
	}
	for _, kind := range recognized {
		err := kind.Validate()

		c.Check(err, jc.ErrorIsNil)
	}
}

func (OriginKindSuite) TestValidateUnknown(c *gc.C) {
	kind := resource.OriginKind("<invalid>")
	err := kind.Validate()

	c.Check(errors.Cause(err), jc.Satisfies, errors.IsNotValid)
	c.Check(err, gc.ErrorMatches, `.*unknown origin.*`)
}

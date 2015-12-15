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

func (OriginKindSuite) TestParseOriginKindKnown(c *gc.C) {
	recognized := map[string]resource.OriginKind{
		"upload": resource.OriginKindUpload,
		"store":  resource.OriginKindStore,
	}
	for value, expected := range recognized {
		kind, err := resource.ParseOriginKind(value)

		c.Check(err, jc.ErrorIsNil)
		c.Check(kind, gc.Equals, expected)
	}
}

func (OriginKindSuite) TestParseOriginKindUnknown(c *gc.C) {
	_, err := resource.ParseOriginKind("<invalid>")

	c.Check(err, gc.ErrorMatches, `.*unknown origin "<invalid>".*`)
}

func (OriginKindSuite) TestValidateKnown(c *gc.C) {
	recognized := []resource.OriginKind{
		resource.OriginKindUpload,
		resource.OriginKindStore,
	}
	for _, kind := range recognized {
		err := kind.Validate()

		c.Check(err, jc.ErrorIsNil)
	}
}

func (OriginKindSuite) TestValidateUnknown(c *gc.C) {
	var kind resource.OriginKind
	err := kind.Validate()

	c.Check(errors.Cause(err), jc.Satisfies, errors.IsNotValid)
	c.Check(err, gc.ErrorMatches, `.*unknown origin.*`)
}

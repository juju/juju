// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resource_test

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/charm/resource"
)

type OriginSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&OriginSuite{})

func (OriginSuite) TestParseOriginKnown(c *gc.C) {
	recognized := map[string]resource.Origin{
		"upload": resource.OriginUpload,
		"store":  resource.OriginStore,
	}
	for value, expected := range recognized {
		origin, err := resource.ParseOrigin(value)

		c.Check(err, jc.ErrorIsNil)
		c.Check(origin, gc.Equals, expected)
	}
}

func (OriginSuite) TestParseOriginUnknown(c *gc.C) {
	_, err := resource.ParseOrigin("<invalid>")

	c.Check(err, gc.ErrorMatches, `.*unknown origin "<invalid>".*`)
}

func (OriginSuite) TestValidateKnown(c *gc.C) {
	recognized := []resource.Origin{
		resource.OriginUpload,
		resource.OriginStore,
	}
	for _, origin := range recognized {
		err := origin.Validate()

		c.Check(err, jc.ErrorIsNil)
	}
}

func (OriginSuite) TestValidateUnknown(c *gc.C) {
	var origin resource.Origin
	err := origin.Validate()

	c.Check(errors.Cause(err), jc.Satisfies, errors.IsNotValid)
	c.Check(err, gc.ErrorMatches, `.*unknown origin.*`)
}

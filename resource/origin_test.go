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

var (
	_ = gc.Suite(&OriginKindSuite{})
	_ = gc.Suite(&OriginSuite{})
)

type OriginKindSuite struct {
	testing.IsolationSuite
}

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

type OriginSuite struct {
	testing.IsolationSuite
}

func (OriginSuite) TestValidateOkay(c *gc.C) {
	recognized := map[resource.OriginKind]string{
		resource.OriginKindUpload: "a-user",
	}
	for kind, value := range recognized {
		o := resource.Origin{
			Kind:  kind,
			Value: value,
		}
		err := o.Validate()

		c.Check(err, jc.ErrorIsNil)
	}
}

func (OriginSuite) TestValidateZeroValue(c *gc.C) {
	var o resource.Origin
	err := o.Validate()

	c.Check(errors.Cause(err), jc.Satisfies, errors.IsNotValid)
}

func (OriginSuite) TestValidateBadKind(c *gc.C) {
	o := resource.Origin{
		Kind:  resource.OriginKind("<invalid>"),
		Value: "spam",
	}
	err := o.Validate()

	c.Check(errors.Cause(err), jc.Satisfies, errors.IsNotValid)
	c.Check(err, gc.ErrorMatches, `.*bad origin kind.*`)
}

func (OriginSuite) TestValidateUploadBadUsername(c *gc.C) {
	o := resource.Origin{
		Kind:  resource.OriginKindUpload,
		Value: "",
	}
	err := o.Validate()

	c.Check(errors.Cause(err), jc.Satisfies, errors.IsNotValid)
	c.Check(err, gc.ErrorMatches, `.*missing upload username.*`)
}

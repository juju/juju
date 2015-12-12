// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resource_test

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	charmresource "gopkg.in/juju/charm.v6-unstable/resource"

	"github.com/juju/juju/resource"
)

type InfoSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&InfoSuite{})

func (InfoSuite) TestValidateFull(c *gc.C) {
	info := resource.Info{
		Resource: newFullCharmResource(c, "spam"),
		Origin:   resource.OriginKindUpload,
	}

	err := info.Validate()

	c.Check(err, jc.ErrorIsNil)
}

func (InfoSuite) TestValidateZeroValue(c *gc.C) {
	var info resource.Info

	err := info.Validate()

	c.Check(errors.Cause(err), jc.Satisfies, errors.IsNotValid)
}

func (InfoSuite) TestValidateBadCharmResource(c *gc.C) {
	var cRes charmresource.Resource
	c.Assert(cRes.Validate(), gc.NotNil)

	info := resource.Info{
		Resource: cRes,
		Origin:   resource.OriginKindUpload,
	}
	err := info.Validate()

	c.Check(errors.Cause(err), jc.Satisfies, errors.IsNotValid)
	c.Check(err, gc.ErrorMatches, `.*bad charm resource info.*`)
}

func (InfoSuite) TestValidateBadOrigin(c *gc.C) {
	var origin resource.OriginKind
	c.Assert(origin.Validate(), gc.NotNil)

	info := resource.Info{
		Resource: newFullCharmResource(c, "spam"),
		Origin:   origin,
	}
	err := info.Validate()

	c.Check(errors.Cause(err), jc.Satisfies, errors.IsNotValid)
	c.Check(err, gc.ErrorMatches, `.*bad origin.*`)
}

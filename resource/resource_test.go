// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resource_test

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	charmresource "gopkg.in/juju/charm.v6-unstable/resource"

	"github.com/juju/juju/resource"
)

type ResourceSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&ResourceSuite{})

func (ResourceSuite) TestValidateUploadUsed(c *gc.C) {
	res := resource.Resource{
		Resource:  newFullCharmResource(c, "spam"),
		Username:  "a-user",
		Timestamp: time.Now(),
	}

	err := res.Validate()

	c.Check(err, jc.ErrorIsNil)
}

func (ResourceSuite) TestValidateUploadNotUsed(c *gc.C) {
	res := resource.Resource{
		Resource: newFullCharmResource(c, "spam"),
	}

	err := res.Validate()

	c.Check(err, jc.ErrorIsNil)
}

func (ResourceSuite) TestValidateZeroValue(c *gc.C) {
	var res resource.Resource

	err := res.Validate()

	c.Check(errors.Cause(err), jc.Satisfies, errors.IsNotValid)
	c.Check(err, gc.ErrorMatches, `.*bad info.*`)
}

func (ResourceSuite) TestValidateBadInfo(c *gc.C) {
	var charmRes charmresource.Resource
	c.Assert(charmRes.Validate(), gc.NotNil)

	res := resource.Resource{
		Resource: charmRes,
	}

	err := res.Validate()

	c.Check(errors.Cause(err), jc.Satisfies, errors.IsNotValid)
	c.Check(err, gc.ErrorMatches, `.*bad info.*`)
}

func (ResourceSuite) TestValidateMissingUsername(c *gc.C) {
	res := resource.Resource{
		Resource:  newFullCharmResource(c, "spam"),
		Username:  "",
		Timestamp: time.Now(),
	}

	err := res.Validate()

	c.Check(err, jc.ErrorIsNil)
}

func (ResourceSuite) TestValidateMissingTimestamp(c *gc.C) {
	res := resource.Resource{
		Resource:  newFullCharmResource(c, "spam"),
		Username:  "a-user",
		Timestamp: time.Time{},
	}

	err := res.Validate()

	c.Check(errors.Cause(err), jc.Satisfies, errors.IsNotValid)
	c.Check(err, gc.ErrorMatches, `.*missing timestamp.*`)
}

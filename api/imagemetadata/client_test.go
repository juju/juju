// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package imagemetadata_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/imagemetadata"
	coretesting "github.com/juju/juju/testing"
)

type imagemetadataSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&imagemetadataSuite{})

func (s *imagemetadataSuite) TestUpdateFromPublishedImages(c *gc.C) {
	called := false

	apiCaller := testing.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, result interface{},
		) error {
			called = true
			c.Check(objType, gc.Equals, "ImageMetadata")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "UpdateFromPublishedImages")
			return nil
		})

	client := imagemetadata.NewClient(apiCaller)
	err := client.UpdateFromPublishedImages()
	c.Check(err, jc.ErrorIsNil)
	c.Assert(called, jc.IsTrue)
}

func (s *imagemetadataSuite) TestUpdateFromPublishedImagesFacadeCallError(c *gc.C) {
	called := false
	msg := "facade failure"
	apiCaller := testing.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, result interface{},
		) error {
			called = true
			c.Check(objType, gc.Equals, "ImageMetadata")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "UpdateFromPublishedImages")
			return errors.New(msg)
		})
	client := imagemetadata.NewClient(apiCaller)
	err := client.UpdateFromPublishedImages()
	c.Assert(errors.Cause(err), gc.ErrorMatches, msg)
	c.Assert(called, jc.IsTrue)
}

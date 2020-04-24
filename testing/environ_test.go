// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing_test

import (
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/testing"
)

type fakeHomeSuite struct {
	testing.FakeJujuXDGDataHomeSuite
}

var _ = gc.Suite(&fakeHomeSuite{})

func (s *fakeHomeSuite) TestModelTagValid(c *gc.C) {
	asString := testing.ModelTag.String()
	tag, err := names.ParseModelTag(asString)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(tag, gc.Equals, testing.ModelTag)
}

func (s *fakeHomeSuite) TestModelUUIDValid(c *gc.C) {
	c.Assert(utils.IsValidUUIDString(testing.ModelTag.Id()), jc.IsTrue)
}

// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing_test

import (
	stdtesting "testing"

	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"github.com/juju/utils/v4"

	"github.com/juju/juju/internal/testing"
)

type fakeHomeSuite struct {
	testing.FakeJujuXDGDataHomeSuite
}

func TestFakeHomeSuite(t *stdtesting.T) { tc.Run(t, &fakeHomeSuite{}) }
func (s *fakeHomeSuite) TestModelTagValid(c *tc.C) {
	asString := testing.ModelTag.String()
	tag, err := names.ParseModelTag(asString)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(tag, tc.Equals, testing.ModelTag)
}

func (s *fakeHomeSuite) TestModelUUIDValid(c *tc.C) {
	c.Assert(utils.IsValidUUIDString(testing.ModelTag.Id()), tc.IsTrue)
}

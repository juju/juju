// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resource_test

import (
	stdtesting "testing"

	"github.com/juju/errors"
	"github.com/juju/tc"

	"github.com/juju/juju/internal/charm/resource"
	"github.com/juju/juju/internal/testhelpers"
)

type OriginSuite struct {
	testhelpers.IsolationSuite
}

func TestOriginSuite(t *stdtesting.T) {
	tc.Run(t, &OriginSuite{})
}

func (s *OriginSuite) TestParseOriginKnown(c *tc.C) {
	recognized := map[string]resource.Origin{
		"upload": resource.OriginUpload,
		"store":  resource.OriginStore,
	}
	for value, expected := range recognized {
		origin, err := resource.ParseOrigin(value)

		c.Check(err, tc.ErrorIsNil)
		c.Check(origin, tc.Equals, expected)
	}
}

func (s *OriginSuite) TestParseOriginUnknown(c *tc.C) {
	_, err := resource.ParseOrigin("<invalid>")

	c.Check(err, tc.ErrorMatches, `.*unknown origin "<invalid>".*`)
}

func (s *OriginSuite) TestValidateKnown(c *tc.C) {
	recognized := []resource.Origin{
		resource.OriginUpload,
		resource.OriginStore,
	}
	for _, origin := range recognized {
		err := origin.Validate()

		c.Check(err, tc.ErrorIsNil)
	}
}

func (s *OriginSuite) TestValidateUnknown(c *tc.C) {
	var origin resource.Origin
	err := origin.Validate()

	c.Check(errors.Cause(err), tc.Satisfies, errors.IsNotValid)
	c.Check(err, tc.ErrorMatches, `.*unknown origin.*`)
}

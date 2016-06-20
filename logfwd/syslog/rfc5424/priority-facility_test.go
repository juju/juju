// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package rfc5424_test

import (
	"fmt"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/logfwd/syslog/rfc5424"
)

type FacilitySuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&FacilitySuite{})

func (s *FacilitySuite) TestString(c *gc.C) {
}

func (s *FacilitySuite) TestValidateSupported(c *gc.C) {
	facilities := []rfc5424.Facility{
		rfc5424.FacilityKern,
		rfc5424.FacilityUser,
		rfc5424.FacilityMail,
		rfc5424.FacilityDaemon,
		rfc5424.FacilityAuth,
		rfc5424.FacilitySyslog,
		rfc5424.FacilityLPR,
		rfc5424.FacilityNews,
		rfc5424.FacilityUUCP,
		rfc5424.FacilityCron,
		rfc5424.FacilityAuthpriv,
		rfc5424.FacilityFTP,
		rfc5424.FacilityNTP,
		rfc5424.FacilityLocal0,
		rfc5424.FacilityLocal1,
		rfc5424.FacilityLocal2,
		rfc5424.FacilityLocal3,
		rfc5424.FacilityLocal4,
		rfc5424.FacilityLocal5,
		rfc5424.FacilityLocal6,
		rfc5424.FacilityLocal7,
	}
	for i, fac := range facilities {
		c.Logf("trying #%d: %s", i, fac)

		err := fac.Validate()

		c.Check(err, jc.ErrorIsNil)
	}
}

func (s *FacilitySuite) TestValidateHidden(c *gc.C) {
	facilities := []rfc5424.Facility{
		rfc5424.Facility(14),
		rfc5424.Facility(15),
		rfc5424.Facility(16),
	}
	for i, fac := range facilities {
		c.Logf("trying #%d: %s", i, fac)

		err := fac.Validate()

		c.Check(err, jc.ErrorIsNil)
	}
}

func (s *FacilitySuite) TestValidateZeroValue(c *gc.C) {
	var fac rfc5424.Facility

	err := fac.Validate()

	c.Check(err, jc.ErrorIsNil)
}

func (s *FacilitySuite) TestValidateUnsupported(c *gc.C) {
	for i, val := range []int{-1, 25, 1000} {
		c.Logf("trying #%d: %d", i, val)
		fac := rfc5424.Facility(val)

		err := fac.Validate()

		c.Check(err, gc.ErrorMatches, fmt.Sprintf(`facility %d not recognized`, val))
	}
}

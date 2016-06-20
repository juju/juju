// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package rfc5424_test

import (
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/logfwd/syslog/rfc5424"
)

type PrioritySuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&PrioritySuite{})

func (s *PrioritySuite) TestStringFull(c *gc.C) {
	pr := rfc5424.Priority{
		Severity: rfc5424.SeverityWarning,
		Facility: rfc5424.FacilityDaemon,
	}

	str := pr.String()

	c.Check(str, gc.Equals, "<28>") // 3<<3 + 4
}

func (s *PrioritySuite) TestStringZeroValue(c *gc.C) {
	var pr rfc5424.Priority

	str := pr.String()

	c.Check(str, gc.Equals, "<8>") // 1<<3 + 0
}

func (s *PrioritySuite) TestStringDefaultSeverity(c *gc.C) {
	pr := rfc5424.Priority{
		Facility: rfc5424.FacilityDaemon,
	}

	str := pr.String()

	c.Check(str, gc.Equals, "<24>") // 3<<3 + 0
}

func (s *PrioritySuite) TestStringDefaultFacility(c *gc.C) {
	pr := rfc5424.Priority{
		Severity: rfc5424.SeverityWarning,
	}

	str := pr.String()

	c.Check(str, gc.Equals, "<12>") // 1<<3 + 4
}

func (s *PrioritySuite) TestStringKernDebug(c *gc.C) {
	pr := rfc5424.Priority{
		Severity: rfc5424.SeverityDebug,
		Facility: rfc5424.FacilityKern,
	}

	str := pr.String()

	c.Check(str, gc.Equals, "<7>") // 0<<3 + 7
}

func (s *PrioritySuite) TestStringKernEmergency(c *gc.C) {
	pr := rfc5424.Priority{
		Severity: rfc5424.SeverityEmergency,
		Facility: rfc5424.FacilityKern,
	}

	str := pr.String()

	c.Check(str, gc.Equals, "<0>") // 0<<3 + 0
}

func (s *PrioritySuite) TestValidateOkay(c *gc.C) {
	pr := rfc5424.Priority{
		Severity: rfc5424.SeverityWarning,
		Facility: rfc5424.FacilityDaemon,
	}

	err := pr.Validate()

	c.Check(err, jc.ErrorIsNil)
}

func (s *PrioritySuite) TestValidateZeroValue(c *gc.C) {
	var pr rfc5424.Priority

	err := pr.Validate()

	c.Check(err, jc.ErrorIsNil)
}

func (s *PrioritySuite) TestValidateBadSeverity(c *gc.C) {
	pr := rfc5424.Priority{
		Severity: -1,
		Facility: rfc5424.FacilityDaemon,
	}

	err := pr.Validate()

	c.Check(err, gc.ErrorMatches, `severity -1 not recognized`)
}

func (s *PrioritySuite) TestValidateBadFacility(c *gc.C) {
	pr := rfc5424.Priority{
		Severity: rfc5424.SeverityWarning,
		Facility: -1,
	}

	err := pr.Validate()

	c.Check(err, gc.ErrorMatches, `facility -1 not recognized`)
}

// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package rfc5424_test

import (
	"fmt"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/standards/rfc5424"
)

type SeveritySuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&SeveritySuite{})

func (s *SeveritySuite) TestString(c *gc.C) {
}

func (s *SeveritySuite) TestValidateSupported(c *gc.C) {
	severities := []rfc5424.Severity{
		rfc5424.SeverityEmergency,
		rfc5424.SeverityAlert,
		rfc5424.SeverityCrit,
		rfc5424.SeverityError,
		rfc5424.SeverityWarning,
		rfc5424.SeverityNotice,
		rfc5424.SeverityInformational,
		rfc5424.SeverityDebug,
	}
	for i, sev := range severities {
		c.Logf("trying #%d: %s", i, sev)

		err := sev.Validate()

		c.Check(err, jc.ErrorIsNil)
	}
}

func (s *SeveritySuite) TestValidateZeroValue(c *gc.C) {
	var sev rfc5424.Severity

	err := sev.Validate()

	c.Check(err, jc.ErrorIsNil)
}

func (s *SeveritySuite) TestValidateUnsupported(c *gc.C) {
	for i, val := range []int{-1, 8, 1000} {
		c.Logf("trying #%d: %d", i, val)
		sev := rfc5424.Severity(val)

		err := sev.Validate()

		c.Check(err, gc.ErrorMatches, fmt.Sprintf(`severity %d not recognized`, val))
	}
}

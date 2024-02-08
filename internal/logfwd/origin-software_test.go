// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logfwd_test

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version/v2"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/internal/logfwd"
)

type SoftwareSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&SoftwareSuite{})

func (s *SoftwareSuite) TestValidateFull(c *gc.C) {
	sw := logfwd.Software{
		PrivateEnterpriseNumber: 28978,
		Name:                    "juju",
		Version:                 version.MustParse("2.0.1"),
	}

	err := sw.Validate()

	c.Check(err, jc.ErrorIsNil)
}

func (s *SoftwareSuite) TestValidateZeroValue(c *gc.C) {
	var sw logfwd.Software

	err := sw.Validate()

	c.Check(err, jc.ErrorIs, errors.NotValid)
}

func (s *SoftwareSuite) TestValidateEmptyPEN(c *gc.C) {
	sw := logfwd.Software{
		Name:    "juju",
		Version: version.MustParse("2.0.1"),
	}

	err := sw.Validate()

	c.Check(err, jc.ErrorIs, errors.NotValid)
	c.Check(err, gc.ErrorMatches, `missing PrivateEnterpriseNumber`)
}

func (s *SoftwareSuite) TestValidateNegativePEN(c *gc.C) {
	sw := logfwd.Software{
		PrivateEnterpriseNumber: -1,
		Name:                    "juju",
		Version:                 version.MustParse("2.0.1"),
	}

	err := sw.Validate()

	c.Check(err, jc.ErrorIs, errors.NotValid)
	c.Check(err, gc.ErrorMatches, `missing PrivateEnterpriseNumber`)
}

func (s *SoftwareSuite) TestValidateEmptyName(c *gc.C) {
	sw := logfwd.Software{
		PrivateEnterpriseNumber: 28978,
		Version:                 version.MustParse("2.0.1"),
	}

	err := sw.Validate()

	c.Check(err, jc.ErrorIs, errors.NotValid)
	c.Check(err, gc.ErrorMatches, `empty Name`)
}

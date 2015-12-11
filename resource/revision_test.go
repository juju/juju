// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resource_test

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/resource"
)

var (
	_ = gc.Suite(&RevisionTypeSuite{})
	_ = gc.Suite(&RevisionSuite{})
)

type RevisionTypeSuite struct {
	testing.IsolationSuite
}

func (RevisionTypeSuite) TestValidateKnown(c *gc.C) {
	recognized := []resource.RevisionType{
		resource.RevisionTypeNone,
		resource.RevisionTypeNumber,
		resource.RevisionTypeDate,
	}
	for _, rt := range recognized {
		err := rt.Validate()

		c.Check(err, jc.ErrorIsNil)
	}
}

func (RevisionTypeSuite) TestValidateUnknown(c *gc.C) {
	rt := resource.RevisionType("<invalid>")
	err := rt.Validate()

	c.Check(errors.Cause(err), jc.Satisfies, errors.IsNotValid)
	c.Check(err, gc.ErrorMatches, `.*unknown revision type.*`)
}

type RevisionSuite struct {
	testing.IsolationSuite
}

func (RevisionSuite) TestParseRevisionOkay(c *gc.C) {
	recognized := map[resource.RevisionType]string{
		resource.RevisionTypeNone:   "",
		resource.RevisionTypeNumber: "1",
		resource.RevisionTypeDate:   "2015-02-12",
	}
	for rt, value := range recognized {
		c.Logf("checking %q:%q", rt, value)
		rev, err := resource.ParseRevision(value)

		c.Check(err, jc.ErrorIsNil)
		c.Check(rev, jc.DeepEquals, resource.Revision{
			Type:  rt,
			Value: value,
		})
	}
}

func (RevisionSuite) TestParseRevisionUnrecognized(c *gc.C) {
	rev, err := resource.ParseRevision("spam")

	c.Check(err, jc.Satisfies, errors.IsNotValid)
	c.Check(rev, jc.DeepEquals, resource.Revision{
		Type:  resource.RevisionTypeUnknown,
		Value: "spam",
	})

}

func (RevisionSuite) TestValidateOkay(c *gc.C) {
	recognized := map[resource.RevisionType]string{
		resource.RevisionTypeNone:   "",
		resource.RevisionTypeNumber: "1",
		resource.RevisionTypeDate:   "2015-02-12",
	}
	for rt, value := range recognized {
		rev := resource.Revision{rt, value}
		err := rev.Validate()

		c.Check(err, jc.ErrorIsNil)
	}
}

func (RevisionSuite) TestValidateZeroValue(c *gc.C) {
	var rev resource.Revision
	err := rev.Validate()

	c.Check(errors.Cause(err), jc.Satisfies, errors.IsNotValid)
}

func (RevisionSuite) TestValidateBadRevisionType(c *gc.C) {
	rev := resource.Revision{Value: "<ignored>"}
	err := rev.Validate()

	c.Check(errors.Cause(err), jc.Satisfies, errors.IsNotValid)
	c.Check(err, gc.ErrorMatches, `.*bad revision type.*`)
}

func (RevisionSuite) TestValidateBadValue(c *gc.C) {
	rev := resource.Revision{
		Type:  resource.RevisionTypeDate,
		Value: "<not-a-date>",
	}
	err := rev.Validate()

	c.Check(errors.Cause(err), jc.Satisfies, errors.IsNotValid)
	c.Check(err, gc.ErrorMatches, `.*invalid value.*`)
}

func (RevisionSuite) checkRevision(c *gc.C, rt resource.RevisionType, value string, valid bool) {
	rev := resource.Revision{
		Type:  rt,
		Value: value,
	}
	err := rev.Validate()

	if valid {
		c.Check(err, jc.ErrorIsNil)
	} else {
		c.Check(errors.Cause(err), jc.Satisfies, errors.IsNotValid)
		c.Check(err, gc.ErrorMatches, `invalid value.*for revision type.*`)
	}
}

func (s RevisionSuite) TestValidateNumber(c *gc.C) {
	values := map[string]bool{
		"0":         true,
		"1":         true,
		"2":         true,
		"1234":      true,
		"987654321": true,
		"spam":      false,
		"-1":        false,
	}
	for value, ok := range values {
		c.Logf("checking %q (%v)", value, ok)
		rt := resource.RevisionTypeNumber
		s.checkRevision(c, rt, value, ok)
	}
}

func (s RevisionSuite) TestValidateDate(c *gc.C) {
	values := map[string]bool{
		"2012-01-01":    true,
		"2020-12-31":    true,
		"2016-02-29":    true,
		"spam":          false,
		"15-01-01":      false,
		"2012-01":       false,
		"2012":          false,
		"2012-01-01-01": false,
		"20012-01-01":   false,
		"2012-13-01":    false,
		"2012-01-32":    false,
		"2012-01-01xxx": false,
		"2012-01-01.1":  false,
		"2012-01-01-1":  false,
	}
	for value, ok := range values {
		c.Logf("checking %q (%v)", value, ok)
		rt := resource.RevisionTypeDate
		s.checkRevision(c, rt, value, ok)
	}
}

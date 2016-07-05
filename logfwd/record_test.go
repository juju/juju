// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logfwd_test

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/logfwd"
)

type BaseRecordSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&BaseRecordSuite{})

func (s *BaseRecordSuite) TestValidateValid(c *gc.C) {
	rec := validBaseRecord

	err := rec.Validate()

	c.Check(err, jc.ErrorIsNil)
}

func (s *BaseRecordSuite) TestValidateZero(c *gc.C) {
	var rec logfwd.BaseRecord

	err := rec.Validate()

	c.Check(err, jc.Satisfies, errors.IsNotValid)
}

func (s *BaseRecordSuite) TestValidateBadOrigin(c *gc.C) {
	rec := validBaseRecord
	rec.Origin.Name = "..."

	err := rec.Validate()

	c.Check(err, jc.Satisfies, errors.IsNotValid)
	c.Check(err, gc.ErrorMatches, `invalid Origin: invalid Name "...": bad user name`)
}

func (s *BaseRecordSuite) TestValidateEmptyTimestamp(c *gc.C) {
	rec := validBaseRecord
	rec.Timestamp = time.Time{}

	err := rec.Validate()

	c.Check(err, jc.Satisfies, errors.IsNotValid)
	c.Check(err, gc.ErrorMatches, `empty Timestamp`)
}

var validBaseRecord = logfwd.BaseRecord{
	ID:        10,
	Origin:    validOrigin,
	Timestamp: time.Now(),
	Message:   "uh-oh",
}

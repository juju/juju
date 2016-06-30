// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logfwd_test

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/logfwd"
)

type AuditSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&AuditSuite{})

func (s *AuditSuite) TestIsZeroTrue(c *gc.C) {
	var audit logfwd.Audit

	isZero := audit.IsZero()

	c.Check(isZero, jc.IsTrue)
}

func (s *AuditSuite) TestIsZeroFalse(c *gc.C) {
	audit := logfwd.Audit{
		Operation: "spam",
	}

	isZero := audit.IsZero()

	c.Check(isZero, jc.IsFalse)
}

func (s *AuditSuite) TestValidateFull(c *gc.C) {
	audit := logfwd.Audit{
		Operation: "spam",
		Args: map[string]string{
			"x": "y",
		},
	}

	err := audit.Validate()

	c.Check(err, jc.ErrorIsNil)
}

func (s *AuditSuite) TestValidateZeroValue(c *gc.C) {
	var audit logfwd.Audit

	err := audit.Validate()

	c.Check(err, gc.NotNil)
}

func (s *AuditSuite) TestValidateEmptyOperation(c *gc.C) {
	audit := logfwd.Audit{
		Args: map[string]string{
			"x": "y",
		},
	}

	err := audit.Validate()

	c.Check(err, jc.Satisfies, errors.IsNotValid)
	c.Check(err, gc.ErrorMatches, `empty Operation`)
}

func (s *AuditSuite) TestValidateNilArgs(c *gc.C) {
	audit := logfwd.Audit{
		Operation: "spam",
	}

	err := audit.Validate()

	c.Check(err, jc.ErrorIsNil)
}

func (s *AuditSuite) TestValidateEmptyArgs(c *gc.C) {
	audit := logfwd.Audit{
		Operation: "spam",
		Args:      map[string]string{},
	}

	err := audit.Validate()

	c.Check(err, jc.ErrorIsNil)
}

func (s *AuditSuite) TestValidateEmptyArgName(c *gc.C) {
	audit := logfwd.Audit{
		Operation: "spam",
		Args: map[string]string{
			"x": "y",
			"":  "...",
			"w": "z",
		},
	}

	err := audit.Validate()

	c.Check(err, jc.Satisfies, errors.IsNotValid)
	c.Check(err, gc.ErrorMatches, `empty arg name not allowed`)
}

func (s *AuditSuite) TestValidateEmptyArgValue(c *gc.C) {
	audit := logfwd.Audit{
		Operation: "spam",
		Args: map[string]string{
			"x": "",
		},
	}

	err := audit.Validate()

	c.Check(err, jc.ErrorIsNil)
}

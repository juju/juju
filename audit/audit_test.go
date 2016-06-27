// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package audit_test

import (
	"net"
	"time"

	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	"github.com/juju/version"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/audit"
)

type auditSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&auditSuite{})

func (s *auditSuite) TestValidate_EmptyModelUUIDErrors(c *gc.C) {
	invalidEntry := validEntry()
	invalidEntry.ModelUUID = ""

	validationErr := invalidEntry.Validate()
	c.Check(validationErr, jc.Satisfies, errors.IsNotValid)
	c.Check(validationErr, gc.ErrorMatches, "ModelUUID not assigned")
}

func (s *auditSuite) TestValidate_InvalidModelUUIDErrors(c *gc.C) {
	invalidEntry := validEntry()
	invalidEntry.ModelUUID = "."

	validationErr := invalidEntry.Validate()
	c.Check(validationErr, jc.Satisfies, errors.IsNotValid)
	c.Check(validationErr, gc.ErrorMatches, "ModelUUID not valid")
}

func (s *auditSuite) TestValidate_EmptyTimestampErrors(c *gc.C) {
	invalidEntry := validEntry()
	invalidEntry.Timestamp = time.Time{}

	validationErr := invalidEntry.Validate()
	c.Check(validationErr, jc.Satisfies, errors.IsNotValid)
	c.Check(validationErr, gc.ErrorMatches, "Timestamp not assigned")
}

func (s *auditSuite) TestValidate_NonUTCTimestampInvalid(c *gc.C) {
	invalidEntry := validEntry()
	invalidEntry.Timestamp = invalidEntry.Timestamp.In(time.FixedZone("x", 3600))

	validationErr := invalidEntry.Validate()
	c.Check(validationErr, jc.Satisfies, errors.IsNotValid)
	c.Check(validationErr, gc.ErrorMatches, "must be set to UTC: Timestamp not valid")
}

func (s *auditSuite) TestValidate_NilOriginIPErrors(c *gc.C) {
	invalidEntry := validEntry()
	invalidEntry.OriginIP = nil

	validationErr := invalidEntry.Validate()
	c.Check(validationErr, jc.Satisfies, errors.IsNotValid)
	c.Check(validationErr, gc.ErrorMatches, "OriginIP not assigned")
}

func (s *auditSuite) TestValidate_ZeroOriginIPErrors(c *gc.C) {
	invalidEntry := validEntry()
	invalidEntry.OriginIP = net.IPv4zero

	validationErr := invalidEntry.Validate()
	c.Check(validationErr, jc.Satisfies, errors.IsNotValid)
	c.Check(validationErr, gc.ErrorMatches, "OriginIP not assigned")
}

func (s *auditSuite) TestValidate_EmptyOriginTypeErrors(c *gc.C) {
	invalidEntry := validEntry()
	invalidEntry.OriginType = ""

	validationErr := invalidEntry.Validate()
	c.Check(validationErr, jc.Satisfies, errors.IsNotValid)
	c.Check(validationErr, gc.ErrorMatches, "OriginType not assigned")
}

func (s *auditSuite) TestValidate_EmptyOriginNameErrors(c *gc.C) {
	invalidEntry := validEntry()
	invalidEntry.OriginName = ""

	validationErr := invalidEntry.Validate()
	c.Check(validationErr, jc.Satisfies, errors.IsNotValid)
	c.Check(validationErr, gc.ErrorMatches, "OriginName not assigned")
}

func (s *auditSuite) TestValidate_EmptyOperationErrors(c *gc.C) {
	invalidEntry := validEntry()
	invalidEntry.Operation = ""

	validationErr := invalidEntry.Validate()
	c.Check(validationErr, jc.Satisfies, errors.IsNotValid)
	c.Check(validationErr, gc.ErrorMatches, "Operation not assigned")
}

func (s *auditSuite) TestValidate_EmptyJujuServerVersionErrors(c *gc.C) {
	invalidEntry := validEntry()
	invalidEntry.JujuServerVersion = version.Zero

	validationErr := invalidEntry.Validate()
	c.Check(validationErr, jc.Satisfies, errors.IsNotValid)
	c.Check(validationErr, gc.ErrorMatches, "JujuServerVersion not assigned")
}

func validEntry() audit.AuditEntry {
	return audit.AuditEntry{
		JujuServerVersion: version.MustParse("1.0.0"),
		ModelUUID:         utils.MustNewUUID().String(),
		Timestamp:         time.Now().UTC(),
		OriginIP:          net.IPv4(8, 8, 8, 8),
		OriginType:        ".",
		OriginName:        ".",
		Operation:         ".",
	}
}

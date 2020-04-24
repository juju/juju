// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package payload_test

import (
	"github.com/juju/charm/v7"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/payload"
)

var _ = gc.Suite(&payloadSuite{})

type payloadSuite struct {
	testing.IsolationSuite
}

func (s *payloadSuite) newPayload(name, pType string) payload.Payload {
	return payload.Payload{
		PayloadClass: charm.PayloadClass{
			Name: name,
			Type: pType,
		},
		ID:     "id" + name,
		Status: payload.StateRunning,
		Labels: []string{"a-tag"},
		Unit:   "a-application/0",
	}
}

func (s *payloadSuite) TestFullID(c *gc.C) {
	payload := s.newPayload("spam", "docker")
	id := payload.FullID()

	c.Check(id, gc.Equals, "spam/idspam")
}

func (s *payloadSuite) TestFullIDMissingID(c *gc.C) {
	payload := s.newPayload("spam", "docker")
	payload.ID = ""
	id := payload.FullID()

	c.Check(id, gc.Equals, "spam")
}

func (s *payloadSuite) TestValidateOkay(c *gc.C) {
	payload := s.newPayload("spam", "docker")
	err := payload.Validate()

	c.Check(err, jc.ErrorIsNil)
}

func (s *payloadSuite) TestValidateMissingName(c *gc.C) {
	payload := s.newPayload("spam", "docker")
	payload.Name = ""
	err := payload.Validate()

	c.Check(err, gc.ErrorMatches, `payload class missing name`)
}

func (s *payloadSuite) TestValidateMissingType(c *gc.C) {
	payload := s.newPayload("spam", "docker")
	payload.Type = ""
	err := payload.Validate()

	c.Check(err, gc.ErrorMatches, `payload class missing type`)
}

func (s *payloadSuite) TestValidateMissingID(c *gc.C) {
	payload := s.newPayload("spam", "docker")
	payload.ID = ""
	err := payload.Validate()

	c.Check(err, gc.ErrorMatches, `missing ID .*`)
}

func (s *payloadSuite) TestValidateMissingStatus(c *gc.C) {
	payload := s.newPayload("spam", "docker")
	payload.Status = ""
	err := payload.Validate()

	c.Check(err, gc.ErrorMatches, `status .* not supported; expected one of .*`)
}

func (s *payloadSuite) TestValidateUnknownStatus(c *gc.C) {
	payload := s.newPayload("spam", "docker")
	payload.Status = "some-unknown-value"
	err := payload.Validate()

	c.Check(err, gc.ErrorMatches, `status .* not supported; expected one of .*`)
}

func (s *payloadSuite) TestValidateMissingUnit(c *gc.C) {
	payload := s.newPayload("spam", "docker")
	payload.Unit = ""
	err := payload.Validate()

	c.Check(err, gc.ErrorMatches, `missing Unit .*`)
}

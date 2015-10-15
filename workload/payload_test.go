// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package workload_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v5"

	"github.com/juju/juju/workload"
)

var _ = gc.Suite(&payloadSuite{})

type payloadSuite struct{}

func (s *payloadSuite) newPayload(name, pType string) workload.Payload {
	return workload.Payload{
		PayloadClass: charm.PayloadClass{
			Name: name,
			Type: pType,
		},
		ID:      "id" + name,
		Status:  workload.StateRunning,
		Tags:    []string{"a-tag"},
		Unit:    "unit-a-service-0",
		Machine: "1",
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

	c.Check(err, gc.ErrorMatches, `state .* not valid`)
}

func (s *payloadSuite) TestValidateUnknownStatus(c *gc.C) {
	payload := s.newPayload("spam", "docker")
	payload.Status = "some-unknown-value"
	err := payload.Validate()

	c.Check(err, gc.ErrorMatches, `state .* not valid`)
}

func (s *payloadSuite) TestValidateMissingUnit(c *gc.C) {
	payload := s.newPayload("spam", "docker")
	payload.Unit = ""
	err := payload.Validate()

	c.Check(err, gc.ErrorMatches, `missing Unit .*`)
}

func (s *payloadSuite) TestValidateMissingMachine(c *gc.C) {
	payload := s.newPayload("spam", "docker")
	payload.Machine = ""
	err := payload.Validate()

	c.Check(err, gc.ErrorMatches, `missing Machine .*`)
}

func (s *payloadSuite) TestAsWorkload(c *gc.C) {
	payload := workload.Payload{
		PayloadClass: charm.PayloadClass{
			Name: "spam",
			Type: "docker",
		},
		ID:      "idspam",
		Status:  workload.StateRunning,
		Tags:    []string{"a-tag"},
		Unit:    "unit-a-service-0",
		Machine: "1",
	}
	converted := payload.AsWorkload()

	c.Check(converted, jc.DeepEquals, workload.Info{
		Workload: charm.Workload{
			Name: "spam",
			Type: "docker",
		},
		Status: workload.Status{
			State: workload.StateRunning,
		},
		Tags: []string{"a-tag"},
		Details: workload.Details{
			ID: "idspam",
		},
	})
}

// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package workload_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v5"

	"github.com/juju/juju/testing"
	"github.com/juju/juju/workload"
)

var _ = gc.Suite(&payloadSuite{})
var _ = gc.Suite(&infoSuite{})

type payloadSuite struct {
	testing.BaseSuite
}

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

type infoSuite struct {
	testing.BaseSuite
}

func (s *infoSuite) newInfo(name, workloadType string) *workload.Info {
	return &workload.Info{
		Workload: charm.Workload{
			Name: name,
			Type: workloadType,
		},
	}
}

func (s *infoSuite) TestIDFull(c *gc.C) {
	info := s.newInfo("a-workload", "docker")
	info.Details.ID = "my-workload"
	id := info.ID()

	c.Check(id, gc.Equals, "a-workload/my-workload")
}

func (s *infoSuite) TestIDMissingDetailsID(c *gc.C) {
	info := s.newInfo("a-workload", "docker")
	id := info.ID()

	c.Check(id, gc.Equals, "a-workload")
}

func (s *infoSuite) TestIDNameOnly(c *gc.C) {
	info := s.newInfo("a-workload", "docker")
	id := info.ID()

	c.Check(id, gc.Equals, "a-workload")
}

func (s *infoSuite) TestValidateOkay(c *gc.C) {
	info := s.newInfo("a workload", "docker")
	info.Status.State = workload.StateRunning
	info.Details.ID = "my-workload"
	info.Details.Status.State = "running"
	err := info.Validate()

	c.Check(err, jc.ErrorIsNil)
}

func (s *infoSuite) TestValidateBadMetadata(c *gc.C) {
	info := s.newInfo("a workload", "")
	err := info.Validate()

	c.Check(err, jc.Satisfies, errors.IsNotValid)
	c.Check(err, gc.ErrorMatches, ".*type: name is required")
}

func (s *infoSuite) TestValidateBadStatus(c *gc.C) {
	info := s.newInfo("a workload", "docker")
	err := info.Validate()

	c.Check(err, jc.Satisfies, errors.IsNotValid)
}

func (s *infoSuite) TestValidateBadDetails(c *gc.C) {
	info := s.newInfo("a workload", "docker")
	info.Status.State = workload.StateRunning
	info.Details.ID = "my-workload"
	err := info.Validate()

	c.Check(err, jc.Satisfies, errors.IsNotValid)
	c.Check(err, gc.ErrorMatches, ".*State cannot be empty.*")
}

func (s *infoSuite) TestTrackedTrue(c *gc.C) {
	info := s.newInfo("a workload", "docker")
	info.Details.ID = "abc123"
	info.Details.Status.State = "running"
	isTracked := info.IsTracked()

	c.Check(isTracked, jc.IsTrue)
}

func (s *infoSuite) TestIsTrackedFalse(c *gc.C) {
	info := s.newInfo("a workload", "docker")
	isTracked := info.IsTracked()

	c.Check(isTracked, jc.IsFalse)
}

func (s *infoSuite) TestAsPayload(c *gc.C) {
	info := s.newInfo("a workload", "docker")
	info.Details.ID = "my-workload"
	info.Status.State = workload.StateRunning
	payload := info.AsPayload()

	c.Check(payload, jc.DeepEquals, workload.Payload{
		PayloadClass: charm.PayloadClass{
			Name: "a workload",
			Type: "docker",
		},
		ID:      "my-workload",
		Status:  "running",
		Unit:    "",
		Machine: "",
	})
}

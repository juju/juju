// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package workload_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v5"

	"github.com/juju/juju/workload"
)

var _ = gc.Suite(&infoSuite{})

type infoSuite struct{}

func (s *infoSuite) newInfo(name, workloadType string) *workload.Info {
	return &workload.Info{
		PayloadClass: charm.PayloadClass{
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
	c.Check(err, gc.ErrorMatches, "payload class missing type")
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

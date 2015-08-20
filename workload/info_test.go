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

type infoSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&infoSuite{})

func (s *infoSuite) newInfo(name, procType string) *workload.Info {
	return &workload.Info{
		Workload: charm.Workload{
			Name: name,
			Type: procType,
		},
	}
}

func (s *infoSuite) TestIDFull(c *gc.C) {
	info := s.newInfo("a-proc", "docker")
	info.Details.ID = "my-proc"
	id := info.ID()

	c.Check(id, gc.Equals, "a-proc/my-proc")
}

func (s *infoSuite) TestIDMissingDetailsID(c *gc.C) {
	info := s.newInfo("a-proc", "docker")
	id := info.ID()

	c.Check(id, gc.Equals, "a-proc")
}

func (s *infoSuite) TestIDNameOnly(c *gc.C) {
	info := s.newInfo("a-proc", "docker")
	id := info.ID()

	c.Check(id, gc.Equals, "a-proc")
}

func (s *infoSuite) TestParseIDFull(c *gc.C) {
	name, id := workload.ParseID("a-proc/my-proc")

	c.Check(name, gc.Equals, "a-proc")
	c.Check(id, gc.Equals, "my-proc")
}

func (s *infoSuite) TestParseIDNameOnly(c *gc.C) {
	name, id := workload.ParseID("a-proc")

	c.Check(name, gc.Equals, "a-proc")
	c.Check(id, gc.Equals, "")
}

func (s *infoSuite) TestParseIDExtras(c *gc.C) {
	name, id := workload.ParseID("somecharm/0/a-proc/my-proc")

	c.Check(name, gc.Equals, "somecharm")
	c.Check(id, gc.Equals, "0/a-proc/my-proc")
}

func (s *infoSuite) TestValidateOkay(c *gc.C) {
	info := s.newInfo("a proc", "docker")
	info.Status.State = workload.StateRunning
	info.Details.ID = "my-proc"
	info.Details.Status.State = "running"
	err := info.Validate()

	c.Check(err, jc.ErrorIsNil)
}

func (s *infoSuite) TestValidateBadMetadata(c *gc.C) {
	info := s.newInfo("a proc", "")
	err := info.Validate()

	c.Check(err, jc.Satisfies, errors.IsNotValid)
	c.Check(err, gc.ErrorMatches, ".*type: name is required")
}

func (s *infoSuite) TestValidateBadStatus(c *gc.C) {
	info := s.newInfo("a proc", "docker")
	err := info.Validate()

	c.Check(err, jc.Satisfies, errors.IsNotValid)
}

func (s *infoSuite) TestValidateBadDetails(c *gc.C) {
	info := s.newInfo("a proc", "docker")
	info.Status.State = workload.StateRunning
	info.Details.ID = "my-proc"
	err := info.Validate()

	c.Check(err, jc.Satisfies, errors.IsNotValid)
	c.Check(err, gc.ErrorMatches, ".*State cannot be empty.*")
}

func (s *infoSuite) TestTrackedTrue(c *gc.C) {
	info := s.newInfo("a proc", "docker")
	info.Details.ID = "abc123"
	info.Details.Status.State = "running"
	isTracked := info.IsTracked()

	c.Check(isTracked, jc.IsTrue)
}

func (s *infoSuite) TestIsTrackedFalse(c *gc.C) {
	info := s.newInfo("a proc", "docker")
	isTracked := info.IsTracked()

	c.Check(isTracked, jc.IsFalse)
}

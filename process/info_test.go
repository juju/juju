// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package process_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v5"

	"github.com/juju/juju/process"
	"github.com/juju/juju/testing"
)

type infoSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&infoSuite{})

func (s *infoSuite) newInfo(name, procType string) *process.Info {
	return &process.Info{
		Process: charm.Process{
			Name: name,
			Type: procType,
		},
	}
}

func (s *infoSuite) TestIDFull(c *gc.C) {
	info := s.newInfo("a-proc", "docker")
	info.CharmID = "somecharm"
	info.UnitID = "somecharm/0"
	info.Details.ID = "my-proc"
	id := info.ID()

	c.Check(id, gc.Equals, "a-proc/my-proc")
}

func (s *infoSuite) TestIDMissingDetailsID(c *gc.C) {
	info := s.newInfo("a-proc", "docker")
	info.CharmID = "somecharm"
	info.UnitID = "somecharm/0"
	id := info.ID()

	c.Check(id, gc.Equals, "a-proc")
}

func (s *infoSuite) TestIDNameOnly(c *gc.C) {
	info := s.newInfo("a-proc", "docker")
	id := info.ID()

	c.Check(id, gc.Equals, "a-proc")
}

func (s *infoSuite) TestFullIDFull(c *gc.C) {
	info := s.newInfo("a-proc", "docker")
	info.CharmID = "somecharmX" // doesn't match UnitID...
	info.UnitID = "somecharm/0"
	info.Details.ID = "my-proc"
	id := info.FullID()

	c.Check(id, gc.Equals, "somecharm/0/a-proc/my-proc")
}

func (s *infoSuite) TestFullIDNameOnly(c *gc.C) {
	info := s.newInfo("a-proc", "docker")
	id := info.FullID()

	c.Check(id, gc.Equals, "a-proc")
}

func (s *infoSuite) TestFullIDMissingCharmID(c *gc.C) {
	info := s.newInfo("a-proc", "docker")
	info.UnitID = "somecharm/0"
	info.Details.ID = "my-proc"
	id := info.FullID()

	c.Check(id, gc.Equals, "somecharm/0/a-proc/my-proc")
}

func (s *infoSuite) TestFullIDMissingUnitID(c *gc.C) {
	info := s.newInfo("a-proc", "docker")
	info.CharmID = "somecharm"
	info.Details.ID = "my-proc"
	id := info.FullID()

	c.Check(id, gc.Equals, "somecharm/a-proc")
}

func (s *infoSuite) TestFullIDMissingDetailsID(c *gc.C) {
	info := s.newInfo("a-proc", "docker")
	info.CharmID = "somecharm"
	info.UnitID = "somecharm/0"
	id := info.FullID()

	c.Check(id, gc.Equals, "somecharm/0/a-proc")
}

func (s *infoSuite) TestParseIDFull(c *gc.C) {
	name, id := process.ParseID("a-proc/my-proc")

	c.Check(name, gc.Equals, "a-proc")
	c.Check(id, gc.Equals, "my-proc")
}

func (s *infoSuite) TestParseIDNameOnly(c *gc.C) {
	name, id := process.ParseID("a-proc")

	c.Check(name, gc.Equals, "a-proc")
	c.Check(id, gc.Equals, "")
}

func (s *infoSuite) TestParseIDExtras(c *gc.C) {
	name, id := process.ParseID("somecharm/0/a-proc/my-proc")

	c.Check(name, gc.Equals, "somecharm")
	c.Check(id, gc.Equals, "0/a-proc/my-proc")
}

func (s *infoSuite) TestValidateOkay(c *gc.C) {
	info := s.newInfo("a proc", "docker")
	info.CharmID = "somecharm"
	info.UnitID = "somecharm/0"
	err := info.Validate()

	c.Check(err, jc.ErrorIsNil)
}

func (s *infoSuite) TestValidateBadMetadata(c *gc.C) {
	info := s.newInfo("a proc", "")
	err := info.Validate()

	c.Check(err, gc.ErrorMatches, ".*type: name is required")
}

func (s *infoSuite) TestValidateBadDetails(c *gc.C) {
	info := s.newInfo("a proc", "docker")
	info.Details.ID = "my-proc"
	err := info.Validate()

	c.Check(err, gc.ErrorMatches, ".*Label cannot be empty.*")
}

func (s *infoSuite) TestValidateMissingCharmID(c *gc.C) {
	info := s.newInfo("a proc", "docker")
	err := info.Validate()

	c.Check(err, gc.ErrorMatches, "missing CharmID")
}

func (s *infoSuite) TestValidateMissingUnitID(c *gc.C) {
	info := s.newInfo("a proc", "docker")
	info.CharmID = "somecharm"
	err := info.Validate()
	c.Check(err, jc.ErrorIsNil)

	info.Details.ID = "my-proc"
	info.Details.Status.Label = "running"
	err = info.Validate()
	c.Check(err, gc.ErrorMatches, "missing UnitID")
}

func (s *infoSuite) TestIsRegisteredTrue(c *gc.C) {
	info := s.newInfo("a proc", "docker")
	info.Details.ID = "abc123"
	info.Details.Status.Label = "running"
	isRegistered := info.IsRegistered()

	c.Check(isRegistered, jc.IsTrue)
}

func (s *infoSuite) TestIsRegisteredFalse(c *gc.C) {
	info := s.newInfo("a proc", "docker")
	isRegistered := info.IsRegistered()

	c.Check(isRegistered, jc.IsFalse)
}

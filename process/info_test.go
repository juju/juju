// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package process_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/process"
	"github.com/juju/juju/testing"
)

type infoSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&infoSuite{})

func (s *infoSuite) TestValidateOkay(c *gc.C) {
	info := process.NewInfo("a proc", "docker")
	err := info.Validate()

	c.Check(err, jc.ErrorIsNil)
}

func (s *infoSuite) TestValidateBadMetadata(c *gc.C) {
	info := process.NewInfo("a proc", "")
	err := info.Validate()

	c.Check(err, gc.ErrorMatches, ".*type: name is required")
}

func (s *infoSuite) TestValidateBadStatus(c *gc.C) {
	info := process.NewInfo("a proc", "docker")
	info.Status = process.Status(-1)
	err := info.Validate()

	c.Check(err, gc.ErrorMatches, "bad status .*")
}

func (s *infoSuite) TestIsRegisteredTrue(c *gc.C) {
	info := process.NewInfo("a proc", "docker")
	info.Status = process.StatusActive
	isRegistered := info.IsRegistered()
	c.Check(isRegistered, jc.IsTrue)

	info = process.NewInfo("a proc", "docker")
	info.Details.ID = "abc123"
	info.Details.Status = "running"
	isRegistered = info.IsRegistered()
	c.Check(isRegistered, jc.IsTrue)
}

func (s *infoSuite) TestIsRegisteredFalse(c *gc.C) {
	info := process.NewInfo("a proc", "docker")
	isRegistered := info.IsRegistered()

	c.Check(isRegistered, jc.IsFalse)
}

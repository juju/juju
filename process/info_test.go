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

func (s *infoSuite) newInfo(name, procType string) *process.ProcessInfo {
	info := &process.ProcessInfo{
		Process: charm.Process{
			Name: name,
			Type: procType,
		},
	}
	return info
}

func (s *infoSuite) TestValidateOkay(c *gc.C) {
	info := s.newInfo("a proc", "docker")
	err := info.Validate()

	c.Check(err, jc.ErrorIsNil)
}

func (s *infoSuite) TestValidateBadMetadata(c *gc.C) {
	info := s.newInfo("a proc", "")
	err := info.Validate()

	c.Check(err, gc.ErrorMatches, ".*type: name is required")
}

func (s *infoSuite) TestValidateBadStatus(c *gc.C) {
	info := s.newInfo("a proc", "docker")
	info.Status = process.Status(-1)
	err := info.Validate()

	c.Check(err, gc.ErrorMatches, "bad status .*")
}

// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package utils_test

import (
	"fmt"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/utils"
)

type IsUbuntuSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&IsUbuntuSuite{})

func (s *IsUbuntuSuite) patchLsbRelease(c *gc.C, name string) {
	content := fmt.Sprintf("#!/bin/bash --norc\necho %s", name)
	patchExecutable(s, c.MkDir(), "lsb_release", content)
}

func (s *IsUbuntuSuite) TestIsUbuntu(c *gc.C) {
	s.patchLsbRelease(c, "Ubuntu")
	c.Assert(utils.IsUbuntu(), jc.IsTrue)
}

func (s *IsUbuntuSuite) TestIsNotUbuntu(c *gc.C) {
	s.patchLsbRelease(c, "Windows NT")
	c.Assert(utils.IsUbuntu(), jc.IsFalse)
}

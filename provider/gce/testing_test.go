// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce

import (
	gitjujutesting "github.com/juju/testing"
	//jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/provider/gce/google"
)

type BaseSuite struct {
	gitjujutesting.IsolationSuite
}

var _ = gc.Suite(&BaseSuite{})

func (s *BaseSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.PatchValue(&connect, func(*google.Connection, google.Auth) error {
		return nil
	})
}

// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action_test

import (
	"testing"

	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/action"
	jujutesting "github.com/juju/juju/juju/testing"
	coretesting "github.com/juju/juju/testing"
)

func TestAll(t *testing.T) {
	coretesting.MgoTestPackage(t)
}

type baseSuite struct {
	jujutesting.JujuConnSuite
	client *action.Client
}

func (s *baseSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.client = action.NewClient(s.APIState)
}

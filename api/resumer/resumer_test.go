// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resumer_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/resumer"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
)

type ResumerSuite struct {
	testing.JujuConnSuite

	st      *api.State
	resumer *resumer.API
}

var _ = gc.Suite(&ResumerSuite{})

func (s *ResumerSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)

	apiState, _ := s.OpenAPIAsNewMachine(c, state.JobManageEnviron)
	// Create the machiner API facade.
	s.resumer = apiState.Resumer()
	c.Assert(s.resumer, gc.NotNil)
}

func (s *ResumerSuite) TestResumeTransactions(c *gc.C) {
	err := s.resumer.ResumeTransactions()
	c.Assert(err, jc.ErrorIsNil)
}

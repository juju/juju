// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resumer_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/resumer"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	jujutesting "github.com/juju/juju/juju/testing"
)

type ResumerSuite struct {
	jujutesting.JujuConnSuite

	resumer    *resumer.ResumerAPI
	resources  *common.Resources
	authoriser apiservertesting.FakeAuthorizer
}

var _ = gc.Suite(&ResumerSuite{})

func (s *ResumerSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)

	s.resources = common.NewResources()
	s.AddCleanup(func(_ *gc.C) { s.resources.StopAll() })
	s.authoriser = apiservertesting.FakeAuthorizer{
		EnvironManager: true,
	}
	var err error
	s.resumer, err = resumer.NewResumerAPI(s.State, s.resources, s.authoriser)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ResumerSuite) TestNewResumerAPIRequiresEnvironManager(c *gc.C) {
	anAuthoriser := s.authoriser
	anAuthoriser.EnvironManager = false
	resumer, err := resumer.NewResumerAPI(s.State, s.resources, anAuthoriser)
	c.Assert(resumer, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *ResumerSuite) TestResumeTransactions(c *gc.C) {
	err := s.resumer.ResumeTransactions()
	c.Assert(err, jc.ErrorIsNil)
}

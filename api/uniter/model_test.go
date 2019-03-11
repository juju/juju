// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/model"
	"github.com/juju/juju/state"
)

type modelSuite struct {
	uniterSuite
	apiModel   *model.Model
	stateModel *state.Model
}

var _ = gc.Suite(&modelSuite{})

func (s *modelSuite) SetUpTest(c *gc.C) {
	s.uniterSuite.SetUpTest(c)
	var err error
	s.apiModel, err = s.uniter.Model()
	c.Assert(err, jc.ErrorIsNil)
	s.stateModel, err = s.State.Model()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *modelSuite) TearDownTest(c *gc.C) {
	s.uniterSuite.TearDownTest(c)
}

func (s *modelSuite) TestUUID(c *gc.C) {
	c.Assert(s.apiModel.UUID, gc.Equals, s.stateModel.UUID())
}

func (s *modelSuite) TestName(c *gc.C) {
	c.Assert(s.apiModel.Name, gc.Equals, s.stateModel.Name())
}

func (s *modelSuite) TestType(c *gc.C) {
	c.Assert(s.apiModel.ModelType.String(), gc.Equals, string(s.stateModel.Type()))
}

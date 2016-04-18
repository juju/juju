// Copyright 2012-2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/state"
	"github.com/juju/juju/status"
	"github.com/juju/juju/testing/factory"
)

type ModelStatusSuite struct {
	ConnSuite
	st    *state.State
	model *state.Model
}

var _ = gc.Suite(&ModelStatusSuite{})

func (s *ModelStatusSuite) SetUpTest(c *gc.C) {
	s.ConnSuite.SetUpTest(c)
	s.st = s.Factory.MakeModel(c, nil)
	m, err := s.st.Model()
	c.Assert(err, jc.ErrorIsNil)
	s.model = m
}

func (s *ModelStatusSuite) TearDownTest(c *gc.C) {
	if s.st != nil {
		err := s.st.Close()
		c.Assert(err, jc.ErrorIsNil)
		s.st = nil
	}
	s.ConnSuite.TearDownTest(c)
}

func (s *ModelStatusSuite) TestInitialStatus(c *gc.C) {
	s.checkInitialStatus(c)
}

func (s *ModelStatusSuite) checkInitialStatus(c *gc.C) {
	statusInfo, err := s.model.Status()
	c.Check(err, jc.ErrorIsNil)
	c.Check(statusInfo.Status, gc.Equals, status.StatusAvailable)
	c.Check(statusInfo.Message, gc.Equals, "")
	c.Check(statusInfo.Data, gc.HasLen, 0)
	c.Check(statusInfo.Since, gc.NotNil)
}

func (s *ModelStatusSuite) TestSetUnknownStatus(c *gc.C) {
	err := s.model.SetStatus(status.Status("vliegkat"), "orville", nil)
	c.Assert(err, gc.ErrorMatches, `cannot set invalid status "vliegkat"`)

	s.checkInitialStatus(c)
}

func (s *ModelStatusSuite) TestSetOverwritesData(c *gc.C) {
	err := s.model.SetStatus(status.StatusAvailable, "blah", map[string]interface{}{
		"pew.pew": "zap",
	})
	c.Check(err, jc.ErrorIsNil)

	s.checkGetSetStatus(c)
}

func (s *ModelStatusSuite) TestGetSetStatusDying(c *gc.C) {
	// Add a machine to the model to ensure it is non-empty
	// when we destroy; this prevents the model from advancing
	// directly to Dead.
	factory.NewFactory(s.st).MakeMachine(c, nil)

	err := s.model.Destroy()
	c.Assert(err, jc.ErrorIsNil)

	s.checkGetSetStatus(c)
}

func (s *ModelStatusSuite) TestGetSetStatusDead(c *gc.C) {
	err := s.model.Destroy()
	c.Assert(err, jc.ErrorIsNil)

	// NOTE: it would be more technically correct to reject status updates
	// while Dead, but it's easier and clearer, not to mention more efficient,
	// to just depend on status doc existence.
	s.checkGetSetStatus(c)
}

func (s *ModelStatusSuite) TestGetSetStatusGone(c *gc.C) {
	err := s.model.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	err = s.st.RemoveAllModelDocs()
	c.Assert(err, jc.ErrorIsNil)

	err = s.model.SetStatus(status.StatusAvailable, "not really", nil)
	c.Check(err, gc.ErrorMatches, `cannot set status: model not found`)

	_, err = s.model.Status()
	c.Check(err, gc.ErrorMatches, `cannot get status: model not found`)
}

func (s *ModelStatusSuite) checkGetSetStatus(c *gc.C) {
	err := s.model.SetStatus(status.StatusAvailable, "blah", map[string]interface{}{
		"$foo.bar.baz": map[string]interface{}{
			"pew.pew": "zap",
		},
	})
	c.Check(err, jc.ErrorIsNil)

	model, err := s.State.GetModel(s.model.ModelTag())
	c.Assert(err, jc.ErrorIsNil)

	statusInfo, err := model.Status()
	c.Check(err, jc.ErrorIsNil)
	c.Check(statusInfo.Status, gc.Equals, status.StatusAvailable)
	c.Check(statusInfo.Message, gc.Equals, "blah")
	c.Check(statusInfo.Data, jc.DeepEquals, map[string]interface{}{
		"$foo.bar.baz": map[string]interface{}{
			"pew.pew": "zap",
		},
	})
	c.Check(statusInfo.Since, gc.NotNil)
}

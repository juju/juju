package state_test

import (
	"github.com/juju/errors"
	"github.com/juju/juju/core/model"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

type generationSuite struct {
	ConnSuite
}

var _ = gc.Suite(&generationSuite{})

func (s *generationSuite) TestNextGenerationNotFound(c *gc.C) {
	_, err := s.Model.NextGeneration()
	c.Assert(errors.IsNotFound(err), jc.IsTrue)
}

func (s *generationSuite) TestNextGenerationSuccess(c *gc.C) {
	c.Assert(s.Model.AddGeneration(), jc.ErrorIsNil)

	gen, err := s.Model.NextGeneration()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(gen, gc.NotNil)

	// A newly created generation is immediately the active one.
	c.Check(gen.Active(), jc.IsTrue)

	v, err := s.Model.ActiveGeneration()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(v, gc.Equals, model.GenerationNext)

	c.Check(gen.ModelUUID(), gc.Equals, s.Model.UUID())
	c.Check(gen.Id(), gc.Not(gc.Equals), "")
}

func (s *generationSuite) TestNextGenerationExistsError(c *gc.C) {
	c.Assert(s.Model.AddGeneration(), jc.ErrorIsNil)

	_, err := s.Model.NextGeneration()
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(s.Model.AddGeneration(), gc.ErrorMatches, "model has a next generation that is not completed")
}

func (s *generationSuite) TestActiveGenerationSwitchSuccess(c *gc.C) {
	v, err := s.Model.ActiveGeneration()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(v, gc.Equals, model.GenerationCurrent)

	c.Assert(s.Model.AddGeneration(), jc.ErrorIsNil)

	v, err = s.Model.ActiveGeneration()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(v, gc.Equals, model.GenerationNext)

	c.Assert(s.Model.SwitchGeneration(model.GenerationCurrent), jc.ErrorIsNil)

	v, err = s.Model.ActiveGeneration()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(v, gc.Equals, model.GenerationCurrent)
}

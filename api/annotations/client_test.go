// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package annotations_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/annotations"
	"github.com/juju/juju/apiserver/params"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
)

type annotationsSuite struct {
	jujutesting.JujuConnSuite
	client *annotations.Client
}

var _ = gc.Suite(&annotationsSuite{})

func (s *annotationsSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.client = annotations.NewClient(s.APIState)
	c.Assert(s.client, gc.NotNil)
}

func (s *annotationsSuite) TearDownTest(c *gc.C) {
	s.JujuConnSuite.TearDownTest(c)
	s.client.ClientFacade.Close()
}

func (s *annotationsSuite) TestSetEntitiesAnnotation(c *gc.C) {
	var called bool
	annts := map[string]string{"annotation": "test"}
	entities := params.Entities{
		[]params.Entity{
			{"charmA"},
			{"serviceB"},
		},
	}
	annotations.PatchFacadeCall(s, s.client, func(request string, a, response interface{}) error {
		called = true
		c.Assert(request, gc.Equals, "SetEntitiesAnnotations")
		args, ok := a.(params.SetEntitiesAnnotations)
		c.Assert(ok, jc.IsTrue)
		c.Assert(args.Annotations, gc.DeepEquals, annts)
		c.Assert(args.Collection, gc.DeepEquals, entities)
		return nil
	})
	err := s.client.SetEntitiesAnnotations(entities, annts)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(called, jc.IsTrue)
}

func (s *annotationsSuite) TestGetEntitiesAnnotations(c *gc.C) {
	var called bool
	testEntity := params.Entity{"charm"}

	annotations.PatchFacadeCall(s, s.client, func(request string, a, response interface{}) error {
		called = true
		c.Assert(request, gc.Equals, "GetEntitiesAnnotations")
		args, ok := a.(params.Entities)
		c.Assert(ok, jc.IsTrue)
		c.Assert(args.Entities, gc.HasLen, 1)
		c.Assert(args.Entities[0], gc.DeepEquals, testEntity)

		result := response.(*params.GetEntitiesAnnotationsResults)
		facadeAnnts := map[string]string{
			"annotations": "test",
		}
		entitiesAnnts := params.GetEntitiesAnnotationsResult{
			Entity:      params.Entity{"charm"},
			Annotations: facadeAnnts,
		}
		result.Results = []params.GetEntitiesAnnotationsResult{entitiesAnnts}
		return nil
	})
	annts, err := s.client.GetEntitiesAnnotations(params.Entities{[]params.Entity{testEntity}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(called, jc.IsTrue)
	c.Assert(annts.Results, gc.HasLen, 1)
}

func (s *annotationsSuite) TestAnnotationFacadeCall(c *gc.C) {
	charm := s.Factory.MakeCharm(c, &factory.CharmParams{Name: "wordpress"})

	annts := map[string]string{"annotation": "test"}
	entities := params.Entities{
		[]params.Entity{
			{charm.Tag().String()},
		},
	}
	err := s.client.SetEntitiesAnnotations(entities, annts)
	c.Assert(err, jc.ErrorIsNil)

	found, err := s.client.GetEntitiesAnnotations(entities)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found.Results, gc.HasLen, 1)
}

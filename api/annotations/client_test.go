// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package annotations_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/annotations"
	basetesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/apiserver/params"
	jujutesting "github.com/juju/juju/juju/testing"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
)

type annotationsMockSuite struct {
	coretesting.BaseSuite
	annotationsClient *annotations.Client
}

var _ = gc.Suite(&annotationsMockSuite{})

func (s *annotationsMockSuite) TestSetEntitiesAnnotation(c *gc.C) {
	var called bool
	annts := map[string]string{"annotation": "test"}
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, a, result interface{}) error {
		called = true
		c.Check(objType, gc.Equals, "Annotations")
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "Set")
		args, ok := a.(params.AnnotationsSet)
		c.Assert(ok, jc.IsTrue)
		c.Assert(args.Annotations, gc.DeepEquals, annts)
		expected := params.Entities{
			[]params.Entity{
				{"charmA"},
				{"serviceB"},
			},
		}
		c.Assert(args.Collection, gc.DeepEquals, expected)
		return nil
	})
	annotationsClient := annotations.NewClient(apiCaller)
	err := annotationsClient.Set([]string{"charmA", "serviceB"}, annts)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(called, jc.IsTrue)
}

func (s *annotationsMockSuite) TestGetEntitiesAnnotations(c *gc.C) {
	var called bool
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, a, response interface{}) error {
		called = true
		c.Check(objType, gc.Equals, "Annotations")
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "Get")
		args, ok := a.(params.Entities)
		c.Assert(ok, jc.IsTrue)
		c.Assert(args.Entities, gc.HasLen, 1)
		c.Assert(args.Entities[0], gc.DeepEquals, params.Entity{"charm"})

		result := response.(*params.AnnotationsGetResults)
		facadeAnnts := map[string]string{
			"annotations": "test",
		}
		entitiesAnnts := params.AnnotationsGetResult{
			Entity:      params.Entity{"charm"},
			Annotations: facadeAnnts,
		}
		result.Results = []params.AnnotationsGetResult{entitiesAnnts}
		return nil
	})
	annotationsClient := annotations.NewClient(apiCaller)
	found, err := annotationsClient.Get([]string{"charm"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(called, jc.IsTrue)
	c.Assert(found, gc.HasLen, 1)
}

type annotationsSuite struct {
	jujutesting.JujuConnSuite
	annotationsClient *annotations.Client
}

var _ = gc.Suite(&annotationsSuite{})

func (s *annotationsSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.annotationsClient = annotations.NewClient(s.APIState)
	c.Assert(s.annotationsClient, gc.NotNil)
}

func (s *annotationsSuite) TearDownTest(c *gc.C) {
	s.JujuConnSuite.TearDownTest(c)
	s.annotationsClient.ClientFacade.Close()
}

func (s *annotationsSuite) TestAnnotationFacadeCall(c *gc.C) {
	charm := s.Factory.MakeCharm(c, &factory.CharmParams{Name: "wordpress"})

	annts := map[string]string{"annotation": "test"}
	err := s.annotationsClient.Set([]string{charm.Tag().String()}, annts)
	c.Assert(err, jc.ErrorIsNil)

	found, err := s.annotationsClient.Get([]string{charm.Tag().String()})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found, gc.HasLen, 1)
}

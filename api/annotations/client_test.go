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
	annts := map[string]string{"annotation1": "test"}
	annts2 := map[string]string{"annotation2": "test"}
	setParams := map[string]map[string]string{
		"charmA":   annts,
		"serviceB": annts2,
	}
	apiCaller := basetesting.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, result interface{},
		) error {
			called = true
			c.Check(objType, gc.Equals, "Annotations")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "Set")

			args, ok := a.(params.AnnotationsSet)
			c.Assert(ok, jc.IsTrue)

			for _, aParam := range args.Annotations {
				// Since sometimes arrays returned on some
				// architectures vary the order within params.AnnotationsSet,
				// simply assert that each entity has its own annotations.
				// Bug 1409141
				c.Assert(aParam.Annotations, gc.DeepEquals, setParams[aParam.EntityTag])
			}
			return nil
		})
	annotationsClient := annotations.NewClient(apiCaller)
	callErrs, err := annotationsClient.Set(setParams)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(callErrs, gc.HasLen, 0)
	c.Assert(called, jc.IsTrue)
}

func (s *annotationsMockSuite) TestGetEntitiesAnnotations(c *gc.C) {
	var called bool
	apiCaller := basetesting.APICallerFunc(
		func(
			objType string,
			version int,
			id, request string,
			a, response interface{}) error {
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
				EntityTag:   "charm",
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
	s.annotationsClient.ClientFacade.Close()
	s.JujuConnSuite.TearDownTest(c)
}

func (s *annotationsSuite) TestAnnotationFacadeCall(c *gc.C) {
	charm := s.Factory.MakeCharm(c, &factory.CharmParams{Name: "wordpress"})

	annts := map[string]string{"annotation": "test"}
	callErrs, err := s.annotationsClient.Set(
		map[string]map[string]string{
			charm.Tag().String(): annts,
		})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(callErrs, gc.HasLen, 0)

	charmTag := charm.Tag().String()
	found, err := s.annotationsClient.Get([]string{charmTag})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found, gc.HasLen, 1)

	firstFound := found[0]
	c.Assert(firstFound.EntityTag, gc.Equals, charmTag)
	c.Assert(firstFound.Annotations, gc.DeepEquals, annts)
	c.Assert(firstFound.Error.Error, gc.IsNil)
}

func (s *annotationsSuite) TestSetCallGettingErrors(c *gc.C) {
	charm := s.Factory.MakeCharm(c, &factory.CharmParams{Name: "wordpress"})
	charmTag := charm.Tag().String()

	annts := map[string]string{"invalid.key": "test"}
	callErrs, err := s.annotationsClient.Set(
		map[string]map[string]string{
			charmTag: annts,
		})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(callErrs, gc.HasLen, 1)
	c.Assert(callErrs[0].Error.Error(), gc.Matches, `.*: invalid key "invalid.key"`)

	found, err := s.annotationsClient.Get([]string{charmTag})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found, gc.HasLen, 1)

	firstFound := found[0]
	c.Assert(firstFound.EntityTag, gc.Equals, charmTag)
	c.Assert(firstFound.Annotations, gc.HasLen, 0)
	c.Assert(firstFound.Error.Error, gc.IsNil)
}

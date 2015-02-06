// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package annotations_test

import (
	"fmt"

	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/annotations"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing/factory"
)

type annotationSuite struct {
	// TODO(anastasiamac) mock to remove JujuConnSuite
	jujutesting.JujuConnSuite

	annotationsApi *annotations.API
	authorizer     apiservertesting.FakeAuthorizer
}

var _ = gc.Suite(&annotationSuite{})

func (s *annotationSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag: s.AdminUserTag(c),
	}
	var err error
	s.annotationsApi, err = annotations.NewAPI(s.State, nil, s.authorizer)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *annotationSuite) TestEnvironmentAnnotations(c *gc.C) {
	env, err := s.State.Environment()
	c.Assert(err, jc.ErrorIsNil)
	s.testSetGetEntitiesAnnotations(c, env.Tag())
}

func (s *annotationSuite) TestMachineAnnotations(c *gc.C) {
	machine := s.Factory.MakeMachine(c, &factory.MachineParams{
		Jobs: []state.MachineJob{state.JobHostUnits},
	})
	s.testSetGetEntitiesAnnotations(c, machine.Tag())

	// on machine removal
	err := machine.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = machine.Remove()
	c.Assert(err, jc.ErrorIsNil)
	s.assertAnnotationsRemoval(c, machine.Tag())
}

func (s *annotationSuite) TestCharmAnnotations(c *gc.C) {
	charm := s.Factory.MakeCharm(c, &factory.CharmParams{Name: "wordpress"})
	s.testSetGetEntitiesAnnotations(c, charm.Tag())
}

func (s *annotationSuite) TestServiceAnnotations(c *gc.C) {
	charm := s.Factory.MakeCharm(c, &factory.CharmParams{Name: "wordpress"})
	wordpress := s.Factory.MakeService(c, &factory.ServiceParams{
		Charm: charm,
	})
	s.testSetGetEntitiesAnnotations(c, wordpress.Tag())

	// on service removal
	err := wordpress.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	s.assertAnnotationsRemoval(c, wordpress.Tag())
}

func (s *annotationSuite) assertAnnotationsRemoval(c *gc.C, tag names.Tag) {
	entity := tag.String()
	entities := params.Entities{[]params.Entity{{entity}}}
	ann := s.annotationsApi.Get(entities)
	c.Assert(ann.Results, gc.HasLen, 1)

	aResult := ann.Results[0]
	c.Assert(aResult.EntityTag, gc.DeepEquals, entity)
	c.Assert(aResult.Annotations, gc.HasLen, 0)
}

func (s *annotationSuite) TestInvalidEntityAnnotations(c *gc.C) {
	entity := "charm-invalid"
	entities := params.Entities{[]params.Entity{{entity}}}
	annotations := map[string]string{"mykey": "myvalue"}

	setResult := s.annotationsApi.Set(
		params.AnnotationsSet{Annotations: constructSetParameters([]string{entity}, annotations)})
	c.Assert(setResult.OneError().Error(), gc.Matches, ".*permission denied.*")

	got := s.annotationsApi.Get(entities)
	c.Assert(got.Results, gc.HasLen, 1)

	aResult := got.Results[0]
	c.Assert(aResult.EntityTag, gc.DeepEquals, entity)
	c.Assert(aResult.Error.Error.Error(), gc.Matches, ".*permission denied.*")
}

func (s *annotationSuite) TestUnitAnnotations(c *gc.C) {
	machine := s.Factory.MakeMachine(c, &factory.MachineParams{
		Jobs: []state.MachineJob{state.JobHostUnits},
	})
	charm := s.Factory.MakeCharm(c, &factory.CharmParams{Name: "wordpress"})
	wordpress := s.Factory.MakeService(c, &factory.ServiceParams{
		Charm: charm,
	})
	unit := s.Factory.MakeUnit(c, &factory.UnitParams{
		Service: wordpress,
		Machine: machine,
	})
	s.testSetGetEntitiesAnnotations(c, unit.Tag())

	// on unit removal
	err := unit.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = unit.Remove()
	c.Assert(err, jc.ErrorIsNil)
	s.assertAnnotationsRemoval(c, wordpress.Tag())
}

func (s *annotationSuite) makeRelation(c *gc.C) (*state.Service, *state.Relation) {
	s1 := s.Factory.MakeService(c, &factory.ServiceParams{
		Name: "service1",
		Charm: s.Factory.MakeCharm(c, &factory.CharmParams{
			Name: "wordpress",
		}),
	})
	e1, err := s1.Endpoint("db")
	c.Assert(err, jc.ErrorIsNil)

	s2 := s.Factory.MakeService(c, &factory.ServiceParams{
		Name: "service2",
		Charm: s.Factory.MakeCharm(c, &factory.CharmParams{
			Name: "mysql",
		}),
	})
	e2, err := s2.Endpoint("server")
	c.Assert(err, jc.ErrorIsNil)

	relation := s.Factory.MakeRelation(c, &factory.RelationParams{
		Endpoints: []state.Endpoint{e1, e2},
	})
	c.Assert(relation, gc.NotNil)
	return s1, relation
}

// Cannot annotate relations...
func (s *annotationSuite) TestRelationAnnotations(c *gc.C) {
	_, relation := s.makeRelation(c)

	tag := relation.Tag().String()
	entity := params.Entity{tag}
	entities := params.Entities{[]params.Entity{entity}}
	annotations := map[string]string{"mykey": "myvalue"}

	setResult := s.annotationsApi.Set(
		params.AnnotationsSet{Annotations: constructSetParameters([]string{tag}, annotations)})
	c.Assert(setResult.OneError().Error(), gc.Matches, ".*does not support annotations.*")

	got := s.annotationsApi.Get(entities)
	c.Assert(got.Results, gc.HasLen, 1)

	aResult := got.Results[0]
	c.Assert(aResult.EntityTag, gc.DeepEquals, tag)
	c.Assert(aResult.Error.Error.Error(), gc.Matches, ".*does not support annotations.*")
}

func constructSetParameters(
	entities []string,
	annotations map[string]string) []params.EntityAnnotations {
	result := []params.EntityAnnotations{}
	for _, entity := range entities {
		one := params.EntityAnnotations{
			EntityTag:   entity,
			Annotations: annotations,
		}
		result = append(result, one)
	}
	return result
}

func (s *annotationSuite) TestMultipleEntitiesAnnotations(c *gc.C) {
	s1, relation := s.makeRelation(c)

	rTag := relation.Tag()
	rEntity := rTag.String()
	sTag := s1.Tag()
	sEntity := sTag.String()

	entities := []string{
		sEntity, //service: expect success in set/get
		rEntity, //relation:expect failure in set/get - cannot annotate relations
	}
	annotations := map[string]string{"mykey": "myvalue"}

	setResult := s.annotationsApi.Set(
		params.AnnotationsSet{Annotations: constructSetParameters(entities, annotations)})
	c.Assert(setResult.Results, gc.HasLen, 1)

	oneError := setResult.Results[0].Error.Error()
	// Only attempt at annotate relation should have erred
	c.Assert(oneError, gc.Matches, fmt.Sprintf(".*%q.*", rTag))
	c.Assert(oneError, gc.Matches, ".*does not support annotations.*")

	got := s.annotationsApi.Get(params.Entities{[]params.Entity{
		{rEntity},
		{sEntity}}})
	c.Assert(got.Results, gc.HasLen, 2)

	var rGet, sGet bool
	for _, aResult := range got.Results {
		if aResult.EntityTag == rTag.String() {
			rGet = true
			c.Assert(aResult.Error.Error.Error(), gc.Matches, ".*does not support annotations.*")
		} else {
			sGet = true
			c.Assert(aResult.EntityTag, gc.DeepEquals, sEntity)
			c.Assert(aResult.Annotations, gc.DeepEquals, annotations)
		}
	}
	// Both entities should have processed
	c.Assert(sGet, jc.IsTrue)
	c.Assert(rGet, jc.IsTrue)
}

func (s *annotationSuite) testSetGetEntitiesAnnotations(c *gc.C, tag names.Tag) {
	entity := tag.String()
	entities := []string{entity}
	for i, t := range clientAnnotationsTests {
		c.Logf("test %d. %s. entity %s", i, t.about, tag.Id())
		s.setupEntity(c, entities, t.initial)
		s.assertSetEntityAnnotations(c, entities, t.input, t.err)
		if t.err != "" {
			continue
		}
		aResult := s.assertGetEntityAnnotations(c, params.Entities{[]params.Entity{{entity}}}, entity, t.expected)
		s.cleanupEntityAnnotations(c, entities, aResult)
	}
}

func (s *annotationSuite) setupEntity(
	c *gc.C,
	entities []string,
	initialAnnotations map[string]string) {
	if initialAnnotations != nil {
		initialResult := s.annotationsApi.Set(
			params.AnnotationsSet{
				Annotations: constructSetParameters(entities, initialAnnotations)})
		c.Assert(initialResult.Combine(), jc.ErrorIsNil)
	}
}

func (s *annotationSuite) assertSetEntityAnnotations(c *gc.C,
	entities []string,
	annotations map[string]string,
	expectedError string) {
	setResult := s.annotationsApi.Set(
		params.AnnotationsSet{Annotations: constructSetParameters(entities, annotations)})
	if expectedError != "" {
		c.Assert(setResult.OneError().Error(), gc.Matches, expectedError)
	} else {
		c.Assert(setResult.Combine(), jc.ErrorIsNil)
	}
}

func (s *annotationSuite) assertGetEntityAnnotations(c *gc.C,
	entities params.Entities,
	entity string,
	expected map[string]string) params.AnnotationsGetResult {
	got := s.annotationsApi.Get(entities)
	c.Assert(got.Results, gc.HasLen, 1)

	aResult := got.Results[0]
	c.Assert(aResult.EntityTag, gc.DeepEquals, entity)
	c.Assert(aResult.Annotations, gc.DeepEquals, expected)
	return aResult
}

func (s *annotationSuite) cleanupEntityAnnotations(c *gc.C,
	entities []string,
	aResult params.AnnotationsGetResult) {
	cleanup := make(map[string]string)
	for key := range aResult.Annotations {
		cleanup[key] = ""
	}
	cleanupResult := s.annotationsApi.Set(
		params.AnnotationsSet{Annotations: constructSetParameters(entities, cleanup)})
	c.Assert(cleanupResult.Combine(), jc.ErrorIsNil)
}

var clientAnnotationsTests = []struct {
	about    string
	initial  map[string]string
	input    map[string]string
	expected map[string]string
	err      string
}{
	{
		about:    "test setting an annotation",
		input:    map[string]string{"mykey": "myvalue"},
		expected: map[string]string{"mykey": "myvalue"},
	},
	{
		about:    "test setting multiple annotations",
		input:    map[string]string{"key1": "value1", "key2": "value2"},
		expected: map[string]string{"key1": "value1", "key2": "value2"},
	},
	{
		about:    "test overriding annotations",
		initial:  map[string]string{"mykey": "myvalue"},
		input:    map[string]string{"mykey": "another-value"},
		expected: map[string]string{"mykey": "another-value"},
	},
	{
		about: "test setting an invalid annotation",
		input: map[string]string{"invalid.key": "myvalue"},
		err:   `.*: invalid key "invalid.key"`,
	},
}

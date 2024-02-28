// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package annotations_test

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/facade/facadetest"
	"github.com/juju/juju/apiserver/facades/client/annotations"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/rpc/params"
)

type annotationSuite struct {
	testing.ApiServerSuite

	annotationsAPI *annotations.API
	authorizer     apiservertesting.FakeAuthorizer

	store objectstore.ObjectStore
}

var _ = gc.Suite(&annotationSuite{})

func (s *annotationSuite) SetUpTest(c *gc.C) {
	s.ApiServerSuite.SetUpTest(c)

	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag: testing.AdminUser,
	}
	var err error
	s.annotationsAPI, err = annotations.NewAPI(facadetest.ModelContext{
		State_:          s.ControllerModel(c).State(),
		ServiceFactory_: s.DefaultModelServiceFactory(c),
		Auth_:           s.authorizer,
	})
	c.Assert(err, jc.ErrorIsNil)

	s.store = testing.NewObjectStore(c, s.ControllerModelUUID())
}

func (s *annotationSuite) TestModelAnnotations(c *gc.C) {
	s.testSetGetEntitiesAnnotations(c, s.ControllerModel(c).ModelTag())
}

func (s *annotationSuite) TestMachineAnnotations(c *gc.C) {
	// TODO (cderici): replace ensureMachine when Machines are added.
	s.ensureMachine(c, "0", "123")
	s.testSetGetEntitiesAnnotations(c, names.NewMachineTag("0"))
}

func (s *annotationSuite) TestCharmAnnotations(c *gc.C) {
	// TODO (cderici): replace ensureCharm when Charm are added.
	s.ensureCharm(c, "local:wordpress-1", "234")
	s.testSetGetEntitiesAnnotations(c, names.NewCharmTag("local:wordpress-1"))
}

func (s *annotationSuite) TestApplicationAnnotations(c *gc.C) {
	// TODO (cderici): replace ensureApplication when Applications are added.
	s.ensureApplication(c, "wordpress", "3")
	s.testSetGetEntitiesAnnotations(c, names.NewApplicationTag("wordpress"))
}

func (s *annotationSuite) TestInvalidEntityAnnotations(c *gc.C) {
	entity := "charm-invalid"
	entities := params.Entities{[]params.Entity{{entity}}}
	annotations := map[string]string{"mykey": "myvalue"}

	setResult := s.annotationsAPI.Set(
		context.Background(),
		params.AnnotationsSet{Annotations: constructSetParameters([]string{entity}, annotations)})
	c.Assert(setResult.OneError().Error(), gc.Matches, ".*unable to find UUID for ID.*")

	got := s.annotationsAPI.Get(context.Background(), entities)
	c.Assert(got.Results, gc.HasLen, 1)

	aResult := got.Results[0]
	c.Assert(aResult.EntityTag, gc.DeepEquals, entity)
	c.Assert(aResult.Error.Error.Error(), gc.Matches, ".*unable to find UUID for ID.*")
}

func (s *annotationSuite) TestUnitAnnotations(c *gc.C) {
	// TODO (cderici): replace ensureUnit when Units are added.
	s.ensureUnit(c, "wordpress/3", "12")
	s.testSetGetEntitiesAnnotations(c, names.NewUnitTag("wordpress/3"))
}

// Cannot annotate relations...
func (s *annotationSuite) TestRelationAnnotations(c *gc.C) {

	tag := names.NewRelationTag("app:rel").String()
	entity := params.Entity{tag}
	entities := params.Entities{[]params.Entity{entity}}
	annotations := map[string]string{"mykey": "myvalue"}

	setResult := s.annotationsAPI.Set(
		context.Background(),
		params.AnnotationsSet{Annotations: constructSetParameters([]string{tag}, annotations)})
	c.Assert(setResult.OneError().Error(), gc.Matches, ".*unknown kind.*")

	got := s.annotationsAPI.Get(context.Background(), entities)
	c.Assert(got.Results, gc.HasLen, 1)

	aResult := got.Results[0]
	c.Assert(aResult.EntityTag, gc.DeepEquals, tag)
	c.Assert(aResult.Error.Error.Error(), gc.Matches, ".*unknown kind.*")
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
	s.ensureApplication(c, "app", "123")
	rTag := names.NewRelationTag("app:rel")
	rEntity := rTag.String()
	sTag := names.NewApplicationTag("app")
	sEntity := sTag.String()

	entities := []string{
		sEntity, //application: expect success in set/get
		rEntity, //relation:expect failure in set/get - cannot annotate relations
	}
	annotations := map[string]string{"mykey": "myvalue"}

	setResult := s.annotationsAPI.Set(
		context.Background(),
		params.AnnotationsSet{Annotations: constructSetParameters(entities, annotations)})
	c.Assert(setResult.Results, gc.HasLen, 1)

	oneError := setResult.Results[0].Error.Error()
	// Only attempt at annotate relation should have erred
	c.Assert(oneError, gc.Matches, fmt.Sprintf(".*%q.*", rTag))
	c.Assert(oneError, gc.Matches, ".*unknown kind.*")

	got := s.annotationsAPI.Get(context.Background(), params.Entities{[]params.Entity{
		{rEntity},
		{sEntity}}})
	c.Assert(got.Results, gc.HasLen, 2)

	var rGet, sGet bool
	for _, aResult := range got.Results {
		if aResult.EntityTag == rTag.String() {
			rGet = true
			c.Assert(aResult.Error.Error.Error(), gc.Matches, ".*unknown kind.*")
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
		initialResult := s.annotationsAPI.Set(
			context.Background(),
			params.AnnotationsSet{
				Annotations: constructSetParameters(entities, initialAnnotations)})
		c.Assert(initialResult.Combine(), jc.ErrorIsNil)
	}
}

func (s *annotationSuite) assertSetEntityAnnotations(c *gc.C,
	entities []string,
	annotations map[string]string,
	expectedError string) {
	setResult := s.annotationsAPI.Set(
		context.Background(),
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
	got := s.annotationsAPI.Get(context.Background(), entities)
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
	cleanupResult := s.annotationsAPI.Set(
		context.Background(),
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

// TODO (cderici): The section below that adds entities into the DB for testing purposes by raw SQL
// should be replaced with actual makers from actual services corresponding to entities whenever
// those entities are implemented.

// ensureApplication manually inserts a row into the application table.
func (s *annotationSuite) ensureApplication(c *gc.C, name, uuid string) {
	err := s.ModelTxnRunner(c, s.ControllerModelUUID()).StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
		INSERT INTO application (uuid, name, life_id)
		VALUES (?, ?, "0")`, uuid, name)
		return err
	})
	c.Assert(err, jc.ErrorIsNil)
}

// ensureNetNode inserts a row into the net_node table, mostly used as a foreign key for entries in
// other tables (e.g. machine)
func (s *annotationSuite) ensureNetNode(c *gc.C, uuid string) {
	err := s.ModelTxnRunner(c, s.ControllerModelUUID()).StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
		INSERT INTO net_node (uuid)
		VALUES (?)`, uuid)
		return err
	})
	c.Assert(err, jc.ErrorIsNil)
}

// ensureUnit manually inserts a row into the unit table.
func (s *annotationSuite) ensureUnit(c *gc.C, unit_id, uuid string) {
	s.ensureApplication(c, "myapp", "123")
	s.ensureNetNode(c, "321")

	err := s.ModelTxnRunner(c, s.ControllerModelUUID()).StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
		INSERT INTO unit (uuid, unit_id, application_uuid, net_node_uuid, life_id)
		VALUES (?, ?, ?, ?, ?)`, uuid, unit_id, "123", "321", "0")
		return err
	})
	c.Assert(err, jc.ErrorIsNil)
}

// ensureMachine manually inserts a row into the machine table.
func (s *annotationSuite) ensureMachine(c *gc.C, id, uuid string) {
	s.ensureNetNode(c, "node2")
	err := s.ModelTxnRunner(c, s.ControllerModelUUID()).StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
		INSERT INTO machine (uuid, net_node_uuid, machine_id, life_id)
		VALUES (?, "node2", ?, "0")`, uuid, id)
		return err
	})
	c.Assert(err, jc.ErrorIsNil)
}

// ensureCharm manually inserts a row into the charm table.
func (s *annotationSuite) ensureCharm(c *gc.C, url, uuid string) {
	err := s.ModelTxnRunner(c, s.ControllerModelUUID()).StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
		INSERT INTO charm (uuid, url)
		VALUES (?, ?)`, uuid, url)
		return err
	})
	c.Assert(err, jc.ErrorIsNil)
}

// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/state"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/testing"
)

type AnnotationsSuite struct {
	ConnSuite
	// any entity that implements
	// state.GlobalEntity will do
	testEntity *state.Machine
}

var _ = gc.Suite(&AnnotationsSuite{})

func (s *AnnotationsSuite) SetUpTest(c *gc.C) {
	s.ConnSuite.SetUpTest(c)

	var err error
	s.testEntity, err = s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *AnnotationsSuite) TestSetAnnotationsInvalidKey(c *gc.C) {
	key := "tes.tkey"
	expected := "typo"
	err := s.setAnnotationResult(c, key, expected)
	c.Assert(errors.Cause(err), gc.ErrorMatches, ".*invalid key.*")
}

func (s *AnnotationsSuite) TestSetAnnotationsCreate(c *gc.C) {
	s.createTestAnnotation(c)
}

func (s *AnnotationsSuite) createTestAnnotation(c *gc.C) string {
	key := "testkey"
	expected := "typo"
	s.assertSetAnnotation(c, key, expected)
	assertAnnotation(c, s.Model, s.testEntity, key, expected)
	return key
}

func (s *AnnotationsSuite) setAnnotationResult(c *gc.C, key, value string) error {
	annts := map[string]string{key: value}
	return s.Model.SetAnnotations(s.testEntity, annts)
}

func (s *AnnotationsSuite) assertSetAnnotation(c *gc.C, key, value string) {
	err := s.setAnnotationResult(c, key, value)
	c.Assert(err, jc.ErrorIsNil)
}

func assertAnnotation(c *gc.C, model *state.Model, entity state.GlobalEntity, key, expected string) {
	value, err := model.Annotation(entity, key)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(value, gc.DeepEquals, expected)
}

func (s *AnnotationsSuite) TestSetAnnotationsUpdate(c *gc.C) {
	key := s.createTestAnnotation(c)
	updated := "fixed"

	s.assertSetAnnotation(c, key, updated)
	assertAnnotation(c, s.Model, s.testEntity, key, updated)
}

func (s *AnnotationsSuite) TestSetAnnotationsRemove(c *gc.C) {
	key := s.createTestAnnotation(c)
	updated := ""
	s.assertSetAnnotation(c, key, updated)
	assertAnnotation(c, s.Model, s.testEntity, key, updated)

	annts, err := s.Model.Annotations(s.testEntity)
	c.Assert(err, jc.ErrorIsNil)

	// we are expecting not to find this key...
	for akey := range annts {
		c.Assert(akey == key, jc.IsFalse)
	}
}

func (s *AnnotationsSuite) TestSetAnnotationsDestroyedEntity(c *gc.C) {
	key := s.createTestAnnotation(c)

	err := s.testEntity.ForceDestroy(dontWait)
	c.Assert(err, jc.ErrorIsNil)
	err = s.testEntity.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = s.testEntity.Remove()
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.Machine(s.testEntity.Id())
	c.Assert(errors.Cause(err), gc.ErrorMatches, ".*not found.*")

	annts, err := s.Model.Annotations(s.testEntity)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(annts, gc.DeepEquals, map[string]string{})

	annts[key] = "oops"
	err = s.Model.SetAnnotations(s.testEntity, annts)
	c.Assert(errors.Cause(err), gc.ErrorMatches, ".*no longer exists.*")
	c.Assert(err, gc.ErrorMatches, ".*cannot update annotations.*")
}

func (s *AnnotationsSuite) TestSetAnnotationsNonExistentEntity(c *gc.C) {
	annts := map[string]string{"key": "oops"}
	err := s.Model.SetAnnotations(state.MockGlobalEntity{}, annts)

	c.Assert(errors.Cause(err), gc.ErrorMatches, ".*no longer exists.*")
	c.Assert(err, gc.ErrorMatches, ".*cannot update annotations.*")
}

func (s *AnnotationsSuite) TestSetAnnotationsConcurrently(c *gc.C) {
	key := "conkey"
	first := "alpha"
	last := "omega"

	setAnnotations := func() {
		s.assertSetAnnotation(c, key, first)
		assertAnnotation(c, s.Model, s.testEntity, key, first)
	}
	defer state.SetBeforeHooks(c, s.State, setAnnotations).Check()
	s.assertSetAnnotation(c, key, last)
	assertAnnotation(c, s.Model, s.testEntity, key, last)
}

type AnnotationsModelSuite struct {
	ConnSuite
}

var _ = gc.Suite(&AnnotationsModelSuite{})

func (s *AnnotationsModelSuite) SetUpTest(c *gc.C) {
	s.ConnSuite.SetUpTest(c)
	s.ConnSuite.PatchValue(&state.TagToCollectionAndId, func(st *state.State, tag names.Tag) (string, interface{}, error) {
		return "", nil, errors.Errorf("this error should not be reached with current implementation %v", tag)
	})
}

func (s *AnnotationsModelSuite) TestSetAnnotationsDestroyedModel(c *gc.C) {
	model, st := s.createTestModel(c)
	defer st.Close()

	key := "key"
	expected := "oops"
	annts := map[string]string{key: expected}
	err := model.SetAnnotations(model, annts)
	c.Assert(err, jc.ErrorIsNil)
	assertAnnotation(c, model, model, key, expected)

	err = model.Destroy(state.DestroyModelParams{})
	c.Assert(err, jc.ErrorIsNil)
	err = st.RemoveDyingModel()
	c.Assert(err, jc.ErrorIsNil)
	err = st.Close()
	c.Assert(err, jc.ErrorIsNil)

	expected = "fail"
	annts[key] = expected
	err = s.Model.SetAnnotations(model, annts)
	c.Assert(errors.Cause(err), gc.ErrorMatches, "model.* no longer exists")
	c.Assert(err, gc.ErrorMatches, ".*cannot update annotations.*")
}

func (s *AnnotationsModelSuite) createTestModel(c *gc.C) (*state.Model, *state.State) {
	uuid, err := utils.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	cfg := testing.CustomModelConfig(c, testing.Attrs{
		"name": "testing",
		"uuid": uuid.String(),
	})
	owner := names.NewUserTag("test@remote")
	model, st, err := s.Controller.NewModel(state.ModelArgs{
		Type:        state.ModelTypeIAAS,
		CloudName:   "dummy",
		CloudRegion: "dummy-region",
		Config:      cfg, Owner: owner,
		StorageProviderRegistry: storage.StaticProviderRegistry{},
	})
	c.Assert(err, jc.ErrorIsNil)
	return model, st
}

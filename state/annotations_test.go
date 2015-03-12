// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"github.com/juju/errors"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/state"
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
	assertAnnotation(c, s.State, s.testEntity, key, expected)
	return key
}

func (s *AnnotationsSuite) setAnnotationResult(c *gc.C, key, value string) error {
	annts := map[string]string{key: value}
	return s.State.SetAnnotations(s.testEntity, annts)
}

func (s *AnnotationsSuite) assertSetAnnotation(c *gc.C, key, value string) {
	err := s.setAnnotationResult(c, key, value)
	c.Assert(err, jc.ErrorIsNil)
}

func assertAnnotation(c *gc.C, st *state.State, entity state.GlobalEntity, key, expected string) {
	value, err := st.Annotation(entity, key)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(value, gc.DeepEquals, expected)
}

func (s *AnnotationsSuite) TestSetAnnotationsUpdate(c *gc.C) {
	key := s.createTestAnnotation(c)
	updated := "fixed"

	s.assertSetAnnotation(c, key, updated)
	assertAnnotation(c, s.State, s.testEntity, key, updated)
}

func (s *AnnotationsSuite) TestSetAnnotationsRemove(c *gc.C) {
	key := s.createTestAnnotation(c)
	updated := ""
	s.assertSetAnnotation(c, key, updated)
	assertAnnotation(c, s.State, s.testEntity, key, updated)

	annts, err := s.State.Annotations(s.testEntity)
	c.Assert(err, jc.ErrorIsNil)

	// we are expecting not to find this key...
	for akey := range annts {
		c.Assert(akey == key, jc.IsFalse)
	}
}

func (s *AnnotationsSuite) TestSetAnnotationsDestroyedEntity(c *gc.C) {
	key := s.createTestAnnotation(c)

	err := s.testEntity.ForceDestroy()
	c.Assert(err, jc.ErrorIsNil)
	err = s.testEntity.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = s.testEntity.Remove()
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.Machine(s.testEntity.Id())
	c.Assert(errors.Cause(err), gc.ErrorMatches, ".*not found.*")

	annts, err := s.State.Annotations(s.testEntity)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(annts, gc.DeepEquals, map[string]string{})

	annts[key] = "oops"
	err = s.State.SetAnnotations(s.testEntity, annts)
	c.Assert(errors.Cause(err), gc.ErrorMatches, ".*no longer exists.*")
	c.Assert(err, gc.ErrorMatches, ".*cannot update annotations.*")
}

func (s *AnnotationsSuite) TestSetAnnotationsNonExistentEntity(c *gc.C) {
	annts := map[string]string{"key": "oops"}
	err := s.State.SetAnnotations(state.MockGlobalEntity{}, annts)

	c.Assert(errors.Cause(err), gc.ErrorMatches, ".*no longer exists.*")
	c.Assert(err, gc.ErrorMatches, ".*cannot update annotations.*")
}

func (s *AnnotationsSuite) TestSetAnnotationsConcurrently(c *gc.C) {
	key := "conkey"
	first := "alpha"
	last := "omega"

	setAnnotations := func() {
		s.assertSetAnnotation(c, key, first)
		assertAnnotation(c, s.State, s.testEntity, key, first)
	}
	defer state.SetBeforeHooks(c, s.State, setAnnotations).Check()
	s.assertSetAnnotation(c, key, last)
	assertAnnotation(c, s.State, s.testEntity, key, last)
}

type AnnotationsEnvSuite struct {
	ConnSuite
}

var _ = gc.Suite(&AnnotationsEnvSuite{})

func (s *AnnotationsEnvSuite) SetUpTest(c *gc.C) {
	s.ConnSuite.SetUpTest(c)
	s.ConnSuite.PatchValue(&state.TagToCollectionAndId, func(st *state.State, tag names.Tag) (string, interface{}, error) {
		return "", nil, errors.Errorf("this error should not be reached with current implementation %v", tag)
	})
}

func (s *AnnotationsEnvSuite) TestSetAnnotationsDestroyedEnvironment(c *gc.C) {
	env, st := s.createTestEnv(c)
	defer st.Close()

	key := "key"
	expected := "oops"
	annts := map[string]string{key: expected}
	err := st.SetAnnotations(env, annts)
	c.Assert(err, jc.ErrorIsNil)
	assertAnnotation(c, st, env, key, expected)

	err = env.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	err = st.Close()
	c.Assert(err, jc.ErrorIsNil)
	err = state.RemoveEnvironment(s.State, st.EnvironUUID())
	c.Assert(err, jc.ErrorIsNil)

	expected = "fail"
	annts[key] = expected
	err = s.State.SetAnnotations(env, annts)
	c.Assert(errors.Cause(err), gc.ErrorMatches, ".*environment not found.*")
	c.Assert(err, gc.ErrorMatches, ".*cannot update annotations.*")
}

func (s *AnnotationsEnvSuite) createTestEnv(c *gc.C) (*state.Environment, *state.State) {
	uuid, err := utils.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	cfg := testing.CustomEnvironConfig(c, testing.Attrs{
		"name": "testing",
		"uuid": uuid.String(),
	})
	owner := names.NewUserTag("test@remote")
	env, st, err := s.State.NewEnvironment(cfg, owner)
	c.Assert(err, jc.ErrorIsNil)
	return env, st
}

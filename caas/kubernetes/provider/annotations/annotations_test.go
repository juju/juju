// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package annotations_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	jujuannotations "github.com/juju/juju/caas/kubernetes/provider/annotations"
	"github.com/juju/juju/testing"
)

type annotationsSuite struct {
	testing.BaseSuite

	annotations jujuannotations.Annotation
}

var _ = gc.Suite(&annotationsSuite{})

func (s *annotationsSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.annotations = jujuannotations.New(nil)
}

func (s *annotationsSuite) TestExistAndAdd(c *gc.C) {
	key := "annotation-1-key"
	value := "annotation-1-val"
	c.Assert(s.annotations.Exist(key, value), jc.IsFalse)

	s.annotations.Add(key, value)
	c.Assert(s.annotations.Exist(key, value), jc.IsTrue)

	s.annotations.Add(key, "a new value")
	c.Assert(s.annotations.Exist(key, value), jc.IsFalse)
	c.Assert(s.annotations.Exist(key, "a new value"), jc.IsTrue)
}

func (s *annotationsSuite) TestExistAllExistAnyMergeToMap(c *gc.C) {
	annotation1 := map[string]string{
		"annotation-1-key": "annotation-1-val",
	}
	annotation2 := map[string]string{
		"annotation-2-key": "annotation-2-val",
	}
	annotation3 := map[string]string{
		"annotation-3-key": "annotation-3-val",
	}
	mergeMap := func(addPrefix bool, maps ...map[string]string) map[string]string {
		out := make(map[string]string)
		for _, m := range maps {
			for k, v := range m {
				newKey := k
				if addPrefix {
					newKey = "juju.io/" + k
				}
				out[newKey] = v
			}
		}
		return out
	}

	// empty
	c.Assert(s.annotations.ExistAll(mergeMap(false,
		annotation1, annotation2, annotation3,
	)), jc.IsFalse)
	c.Assert(s.annotations.ExistAny(annotation1), jc.IsFalse)
	c.Assert(s.annotations.ExistAny(annotation2), jc.IsFalse)
	c.Assert(s.annotations.ExistAny(annotation3), jc.IsFalse)
	c.Assert(s.annotations.ToMap(), gc.DeepEquals, make(map[string]string))

	// merge 1, has 1.
	s.annotations.Merge(jujuannotations.New(annotation1))
	c.Assert(s.annotations.ExistAll(mergeMap(false,
		annotation1, annotation2, annotation3,
	)), jc.IsFalse)
	c.Assert(s.annotations.ExistAny(annotation1), jc.IsTrue)
	c.Assert(s.annotations.ExistAny(annotation2), jc.IsFalse)
	c.Assert(s.annotations.ExistAny(annotation3), jc.IsFalse)
	c.Assert(s.annotations.ToMap(), jc.DeepEquals, mergeMap(true, annotation1))

	// merge 2, has 1, 2.
	s.annotations.Merge(jujuannotations.New(annotation2))
	c.Assert(s.annotations.ExistAll(mergeMap(false,
		annotation1, annotation2, annotation3,
	)), jc.IsFalse)
	c.Assert(s.annotations.ExistAny(annotation1), jc.IsTrue)
	c.Assert(s.annotations.ExistAny(annotation2), jc.IsTrue)
	c.Assert(s.annotations.ExistAny(annotation3), jc.IsFalse)
	c.Assert(s.annotations.ToMap(), jc.DeepEquals, mergeMap(true, annotation1, annotation2))

	// merge 3, has 1, 2, 3.
	s.annotations.Merge(jujuannotations.New(annotation3))
	c.Assert(s.annotations.ExistAll(mergeMap(false,
		annotation1, annotation2, annotation3,
	)), jc.IsTrue)
	c.Assert(s.annotations.ExistAny(annotation1), jc.IsTrue)
	c.Assert(s.annotations.ExistAny(annotation2), jc.IsTrue)
	c.Assert(s.annotations.ExistAny(annotation3), jc.IsTrue)
	c.Assert(s.annotations.ToMap(), jc.DeepEquals, mergeMap(true, annotation1, annotation2, annotation3))
}

func (s *annotationsSuite) TestCheckKeysNonEmpty(c *gc.C) {
	c.Assert(s.annotations.CheckKeysNonEmpty("key1"), jc.Satisfies, errors.IsNotFound)

	s.annotations.Add("key1", "")
	c.Assert(s.annotations.CheckKeysNonEmpty("key1"), jc.Satisfies, errors.IsNotValid)

	s.annotations.Add("key2", "val2")
	c.Assert(s.annotations.CheckKeysNonEmpty("key2"), jc.ErrorIsNil)
	c.Assert(s.annotations.CheckKeysNonEmpty("key1", "key2"), jc.Satisfies, errors.IsNotValid)
}

// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package annotations

import (
	"github.com/juju/names/v6"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/internal/testing"
)

type annotationsSuite struct {
	testing.BaseSuite

	annotations Annotation
}

var _ = gc.Suite(&annotationsSuite{})

func (s *annotationsSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.annotations = New(nil)
}

func (s *annotationsSuite) TestExistAndAdd(c *gc.C) {
	key := "annotation-1-key"
	value := "annotation-1-val"
	c.Assert(s.annotations.Has(key, value), jc.IsFalse)

	s.annotations.Add(key, value)
	c.Assert(s.annotations.Has(key, value), jc.IsTrue)

	s.annotations.Add(key, "a new value")
	c.Assert(s.annotations.Has(key, value), jc.IsFalse)
	c.Assert(s.annotations.Has(key, "a new value"), jc.IsTrue)
}

func (s *annotationsSuite) TestRemove(c *gc.C) {
	key := "annotation-1-key"
	value := "annotation-1-val"
	c.Assert(s.annotations.Has(key, value), jc.IsFalse)

	s.annotations.Add(key, value)
	c.Assert(s.annotations.Has(key, value), jc.IsTrue)

	s.annotations.Remove(key)
	c.Assert(s.annotations.Has(key, value), jc.IsFalse)
}

func (s *annotationsSuite) TestCopy(c *gc.C) {
	annotation1 := map[string]string{
		"annotation-1-key": "annotation-1-val",
	}
	s.annotations.Merge(New(annotation1))
	c.Assert(s.annotations.ToMap(), jc.DeepEquals, annotation1)

	newAnnotation1 := s.annotations.Copy()
	newAnnotation2 := s.annotations

	newAnnotation1.Add("a-new-key", "a-new-value")
	c.Assert(
		newAnnotation1.ToMap(), jc.DeepEquals,
		map[string]string{
			"annotation-1-key": "annotation-1-val",
			"a-new-key":        "a-new-value",
		},
	)
	// no change in original one because it was Copy.
	c.Assert(s.annotations.ToMap(), jc.DeepEquals, annotation1)

	newAnnotation2.Add("aaaa", "bbbbb")
	c.Assert(newAnnotation2.ToMap(), jc.DeepEquals, map[string]string{
		"annotation-1-key": "annotation-1-val",
		"aaaa":             "bbbbb",
	})
	// changed in original one because it was NOT Copy.
	c.Assert(s.annotations.ToMap(), jc.DeepEquals, map[string]string{
		"annotation-1-key": "annotation-1-val",
		"aaaa":             "bbbbb",
	})
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
	mergeMap := func(maps ...map[string]string) map[string]string {
		out := make(map[string]string)
		for _, m := range maps {
			for k, v := range m {
				out[k] = v
			}
		}
		return out
	}

	// empty
	c.Assert(s.annotations.HasAll(mergeMap(annotation1, annotation2, annotation3)), jc.IsFalse)
	c.Assert(s.annotations.HasAny(annotation1), jc.IsFalse)
	c.Assert(s.annotations.HasAny(annotation2), jc.IsFalse)
	c.Assert(s.annotations.HasAny(annotation3), jc.IsFalse)
	c.Assert(s.annotations.ToMap(), jc.DeepEquals, make(map[string]string))

	// merge 1, has 1.
	s.annotations.Merge(New(annotation1))
	c.Assert(s.annotations.HasAll(mergeMap(annotation1, annotation2, annotation3)), jc.IsFalse)
	c.Assert(s.annotations.HasAny(annotation1), jc.IsTrue)
	c.Assert(s.annotations.HasAny(annotation2), jc.IsFalse)
	c.Assert(s.annotations.HasAny(annotation3), jc.IsFalse)
	c.Assert(s.annotations.ToMap(), jc.DeepEquals, mergeMap(annotation1))

	// merge 2, has 1, 2.
	s.annotations.Merge(New(annotation2))
	c.Assert(s.annotations.HasAll(mergeMap(annotation1, annotation2, annotation3)), jc.IsFalse)
	c.Assert(s.annotations.HasAny(annotation1), jc.IsTrue)
	c.Assert(s.annotations.HasAny(annotation2), jc.IsTrue)
	c.Assert(s.annotations.HasAny(annotation3), jc.IsFalse)
	c.Assert(s.annotations.ToMap(), jc.DeepEquals, mergeMap(annotation1, annotation2))

	// merge 3, has 1, 2, 3.
	s.annotations.Merge(New(annotation3))
	c.Assert(s.annotations.HasAll(mergeMap(annotation1, annotation2, annotation3)), jc.IsTrue)
	c.Assert(s.annotations.HasAny(annotation1), jc.IsTrue)
	c.Assert(s.annotations.HasAny(annotation2), jc.IsTrue)
	c.Assert(s.annotations.HasAny(annotation3), jc.IsTrue)
	c.Assert(s.annotations.ToMap(), jc.DeepEquals, mergeMap(annotation1, annotation2, annotation3))
}

func (s *annotationsSuite) TestCheckKeysNonEmpty(c *gc.C) {
	c.Assert(s.annotations.CheckKeysNonEmpty("key1"), jc.ErrorIs, coreerrors.NotFound)

	s.annotations.Add("key1", "")
	c.Assert(s.annotations.CheckKeysNonEmpty("key1"), jc.ErrorIs, coreerrors.NotValid)

	s.annotations.Add("key2", "val2")
	c.Assert(s.annotations.CheckKeysNonEmpty("key2"), jc.ErrorIsNil)
	c.Assert(s.annotations.CheckKeysNonEmpty("key1", "key2"), jc.ErrorIs, coreerrors.NotValid)
}

func (s *annotationsSuite) TestConvertTagToID(c *gc.C) {
	// ConvertTagToID happy path
	id, err := ConvertTagToID(names.NewUnitTag("unit/0"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(id, jc.DeepEquals, ID{Kind: KindUnit, Name: "unit/0"})

	// ConvertTagToID unknown kind
	_, err = ConvertTagToID(names.NewEnvironTag("env/0"))
	c.Assert(err.Error(), gc.Equals, "unknown kind \"environment\"")
}

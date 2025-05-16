// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package annotations

import (
	stdtesting "testing"

	"github.com/juju/names/v6"
	"github.com/juju/tc"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/internal/testing"
)

type annotationsSuite struct {
	testing.BaseSuite

	annotations Annotation
}

func TestAnnotationsSuite(t *stdtesting.T) { tc.Run(t, &annotationsSuite{}) }
func (s *annotationsSuite) SetUpTest(c *tc.C) {
	s.BaseSuite.SetUpTest(c)

	s.annotations = New(nil)
}

func (s *annotationsSuite) TestExistAndAdd(c *tc.C) {
	key := "annotation-1-key"
	value := "annotation-1-val"
	c.Assert(s.annotations.Has(key, value), tc.IsFalse)

	s.annotations.Add(key, value)
	c.Assert(s.annotations.Has(key, value), tc.IsTrue)

	s.annotations.Add(key, "a new value")
	c.Assert(s.annotations.Has(key, value), tc.IsFalse)
	c.Assert(s.annotations.Has(key, "a new value"), tc.IsTrue)
}

func (s *annotationsSuite) TestRemove(c *tc.C) {
	key := "annotation-1-key"
	value := "annotation-1-val"
	c.Assert(s.annotations.Has(key, value), tc.IsFalse)

	s.annotations.Add(key, value)
	c.Assert(s.annotations.Has(key, value), tc.IsTrue)

	s.annotations.Remove(key)
	c.Assert(s.annotations.Has(key, value), tc.IsFalse)
}

func (s *annotationsSuite) TestCopy(c *tc.C) {
	annotation1 := map[string]string{
		"annotation-1-key": "annotation-1-val",
	}
	s.annotations.Merge(New(annotation1))
	c.Assert(s.annotations.ToMap(), tc.DeepEquals, annotation1)

	newAnnotation1 := s.annotations.Copy()
	newAnnotation2 := s.annotations

	newAnnotation1.Add("a-new-key", "a-new-value")
	c.Assert(
		newAnnotation1.ToMap(), tc.DeepEquals,
		map[string]string{
			"annotation-1-key": "annotation-1-val",
			"a-new-key":        "a-new-value",
		},
	)
	// no change in original one because it was Copy.
	c.Assert(s.annotations.ToMap(), tc.DeepEquals, annotation1)

	newAnnotation2.Add("aaaa", "bbbbb")
	c.Assert(newAnnotation2.ToMap(), tc.DeepEquals, map[string]string{
		"annotation-1-key": "annotation-1-val",
		"aaaa":             "bbbbb",
	})
	// changed in original one because it was NOT Copy.
	c.Assert(s.annotations.ToMap(), tc.DeepEquals, map[string]string{
		"annotation-1-key": "annotation-1-val",
		"aaaa":             "bbbbb",
	})
}

func (s *annotationsSuite) TestExistAllExistAnyMergeToMap(c *tc.C) {
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
	c.Assert(s.annotations.HasAll(mergeMap(annotation1, annotation2, annotation3)), tc.IsFalse)
	c.Assert(s.annotations.HasAny(annotation1), tc.IsFalse)
	c.Assert(s.annotations.HasAny(annotation2), tc.IsFalse)
	c.Assert(s.annotations.HasAny(annotation3), tc.IsFalse)
	c.Assert(s.annotations.ToMap(), tc.DeepEquals, make(map[string]string))

	// merge 1, has 1.
	s.annotations.Merge(New(annotation1))
	c.Assert(s.annotations.HasAll(mergeMap(annotation1, annotation2, annotation3)), tc.IsFalse)
	c.Assert(s.annotations.HasAny(annotation1), tc.IsTrue)
	c.Assert(s.annotations.HasAny(annotation2), tc.IsFalse)
	c.Assert(s.annotations.HasAny(annotation3), tc.IsFalse)
	c.Assert(s.annotations.ToMap(), tc.DeepEquals, mergeMap(annotation1))

	// merge 2, has 1, 2.
	s.annotations.Merge(New(annotation2))
	c.Assert(s.annotations.HasAll(mergeMap(annotation1, annotation2, annotation3)), tc.IsFalse)
	c.Assert(s.annotations.HasAny(annotation1), tc.IsTrue)
	c.Assert(s.annotations.HasAny(annotation2), tc.IsTrue)
	c.Assert(s.annotations.HasAny(annotation3), tc.IsFalse)
	c.Assert(s.annotations.ToMap(), tc.DeepEquals, mergeMap(annotation1, annotation2))

	// merge 3, has 1, 2, 3.
	s.annotations.Merge(New(annotation3))
	c.Assert(s.annotations.HasAll(mergeMap(annotation1, annotation2, annotation3)), tc.IsTrue)
	c.Assert(s.annotations.HasAny(annotation1), tc.IsTrue)
	c.Assert(s.annotations.HasAny(annotation2), tc.IsTrue)
	c.Assert(s.annotations.HasAny(annotation3), tc.IsTrue)
	c.Assert(s.annotations.ToMap(), tc.DeepEquals, mergeMap(annotation1, annotation2, annotation3))
}

func (s *annotationsSuite) TestCheckKeysNonEmpty(c *tc.C) {
	c.Assert(s.annotations.CheckKeysNonEmpty("key1"), tc.ErrorIs, coreerrors.NotFound)

	s.annotations.Add("key1", "")
	c.Assert(s.annotations.CheckKeysNonEmpty("key1"), tc.ErrorIs, coreerrors.NotValid)

	s.annotations.Add("key2", "val2")
	c.Assert(s.annotations.CheckKeysNonEmpty("key2"), tc.ErrorIsNil)
	c.Assert(s.annotations.CheckKeysNonEmpty("key1", "key2"), tc.ErrorIs, coreerrors.NotValid)
}

func (s *annotationsSuite) TestConvertTagToID(c *tc.C) {
	// ConvertTagToID happy path
	id, err := ConvertTagToID(names.NewUnitTag("unit/0"))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(id, tc.DeepEquals, ID{Kind: KindUnit, Name: "unit/0"})

	// ConvertTagToID unknown kind
	_, err = ConvertTagToID(names.NewEnvironTag("env/0"))
	c.Assert(err.Error(), tc.Equals, "unknown kind \"environment\"")
}

// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package description

import (
	"time"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/yaml.v2"
)

type ResourceSuite struct {
	SliceSerializationSuite
}

var _ = gc.Suite(&ResourceSuite{})

func (s *ResourceSuite) SetUpTest(c *gc.C) {
	s.SerializationSuite.SetUpTest(c)
	s.importName = "resources"
	s.sliceName = "resources"
	s.importFunc = func(m map[string]interface{}) (interface{}, error) {
		return importResources(m)
	}
	s.testFields = func(m map[string]interface{}) {
		m["resources"] = []interface{}{}
	}
}

func minimalResourceMap() map[interface{}]interface{} {
	return map[interface{}]interface{}{
		"application-revision": 3,
		"charmstore-revision":  4,
		"name":                 "bdist",
		"revisions": []interface{}{
			map[interface{}]interface{}{
				"revision":    3,
				"timestamp":   "2016-10-18T02:03:04Z",
				"description": "description",
				"fingerprint": "aaaaaaaa",
				"origin":      "store",
				"path":        "file.tar.gz",
				"size":        111,
				"type":        "file",
				"username":    "user",
			},
			map[interface{}]interface{}{
				"revision":    4,
				"description": "description",
				"fingerprint": "bbbbbbbb",
				"origin":      "store",
				"path":        "file.tar.gz",
				"size":        222,
				"type":        "file",
			},
		},
	}
}

func minimalResource() *resource {
	r := newResource(ResourceArgs{
		Name:               "bdist",
		Revision:           3,
		CharmStoreRevision: 4,
	})
	r.AddRevision(ResourceRevisionArgs{
		Revision:       3,
		Type:           "file",
		Path:           "file.tar.gz",
		Description:    "description",
		Origin:         "store",
		FingerprintHex: "aaaaaaaa",
		Size:           111,
		Timestamp:      time.Date(2016, 10, 18, 2, 3, 4, 0, time.UTC),
		Username:       "user",
	})
	r.AddRevision(ResourceRevisionArgs{
		Revision:       4,
		Type:           "file",
		Path:           "file.tar.gz",
		Description:    "description",
		Origin:         "store",
		FingerprintHex: "bbbbbbbb",
		Size:           222,
	})
	return r
}

func (s *ResourceSuite) TestNew(c *gc.C) {
	r := minimalResource()
	c.Check(r.Name(), gc.Equals, "bdist")
	c.Check(r.Revision(), gc.Equals, 3)
	c.Check(r.CharmStoreRevision(), gc.Equals, 4)

	rs := r.Revisions()
	c.Assert(rs, gc.HasLen, 2)

	rev3 := rs[3]
	c.Check(rev3.Revision(), gc.Equals, 3)
	c.Check(rev3.Type(), gc.Equals, "file")
	c.Check(rev3.Path(), gc.Equals, "file.tar.gz")
	c.Check(rev3.Description(), gc.Equals, "description")
	c.Check(rev3.Origin(), gc.Equals, "store")
	c.Check(rev3.FingerprintHex(), gc.Equals, "aaaaaaaa")
	c.Check(rev3.Size(), gc.Equals, int64(111))
	c.Check(rev3.Timestamp(), gc.Equals, time.Date(2016, 10, 18, 2, 3, 4, 0, time.UTC))
	c.Check(rev3.Username(), gc.Equals, "user")

	rev4 := rs[4]
	c.Check(rev4.Revision(), gc.Equals, 4)
	c.Check(rev4.Type(), gc.Equals, "file")
	c.Check(rev4.Path(), gc.Equals, "file.tar.gz")
	c.Check(rev4.Description(), gc.Equals, "description")
	c.Check(rev4.Origin(), gc.Equals, "store")
	c.Check(rev4.FingerprintHex(), gc.Equals, "bbbbbbbb")
	c.Check(rev4.Size(), gc.Equals, int64(222))
	c.Check(rev4.Timestamp(), gc.Equals, time.Time{})
	c.Check(rev4.Username(), gc.Equals, "")
}

func (s *ResourceSuite) TestMinimalValid(c *gc.C) {
	r := minimalResource()
	c.Assert(r.Validate(), jc.ErrorIsNil)
}

func (s *ResourceSuite) TestMinimalMatches(c *gc.C) {
	bytes, err := yaml.Marshal(minimalResource())
	c.Assert(err, jc.ErrorIsNil)

	var source map[interface{}]interface{}
	err = yaml.Unmarshal(bytes, &source)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(source, jc.DeepEquals, minimalResourceMap())
}

func (s *ResourceSuite) TestValidateMissingRev(c *gc.C) {
	r := newResource(ResourceArgs{
		Name:               "bdist",
		Revision:           3,
		CharmStoreRevision: 3,
	})
	c.Assert(r.Validate(), gc.ErrorMatches, `missing application revision \(3\)`)
}

func (s *ResourceSuite) TestValidateMissingCharmStoreRev(c *gc.C) {
	r := newResource(ResourceArgs{
		Name:               "bdist",
		Revision:           3,
		CharmStoreRevision: 4,
	})
	r.AddRevision(ResourceRevisionArgs{
		Revision:       3,
		Type:           "file",
		Path:           "file.tar.gz",
		Description:    "description",
		Origin:         "store",
		FingerprintHex: "aaaaaaaa",
		Size:           111,
		Timestamp:      time.Date(2016, 10, 18, 2, 3, 4, 0, time.UTC),
		Username:       "user",
	})
	c.Assert(r.Validate(), gc.ErrorMatches, `missing charmstore revision \(4\)`)
}

func (s *ResourceSuite) TestRevisionMerging(c *gc.C) {
	now := time.Now().UTC()

	args0 := ResourceRevisionArgs{
		Revision: 3,
	}
	args1 := ResourceRevisionArgs{
		Revision:       3,
		Description:    "description",
		Type:           "type",
		Path:           "path",
		FingerprintHex: "print",
		Size:           123,
		Timestamp:      now,
		Username:       "user",
	}

	check := func(a, b ResourceRevisionArgs) {
		r := newResource(ResourceArgs{
			Name:               "bdist",
			Revision:           3,
			CharmStoreRevision: 3,
		})
		_, err := r.AddRevision(a)
		c.Assert(err, jc.ErrorIsNil)
		_, err = r.AddRevision(b)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(r.Validate(), jc.ErrorIsNil)

		revs := r.Revisions()
		c.Assert(revs, gc.HasLen, 1)
		rev, found := revs[3]
		c.Assert(found, jc.IsTrue)
		c.Check(rev.Revision(), gc.Equals, 3)
		c.Check(rev.Type(), gc.Equals, "type")
		c.Check(rev.Description(), gc.Equals, "description")
		c.Check(rev.Path(), gc.Equals, "path")
		c.Check(rev.FingerprintHex(), gc.Equals, "print")
		c.Check(rev.Size(), gc.Equals, int64(123))
		c.Check(rev.Timestamp(), gc.Equals, now)
		c.Check(rev.Username(), gc.Equals, "user")
	}

	// The merged revision should be the same regarding of ordering.
	check(args0, args1)
	check(args1, args0)
}

func (s *ResourceSuite) TestMergedRevisionsMismatch(c *gc.C) {
	now := time.Now()
	args0 := ResourceRevisionArgs{
		Revision:       3,
		Description:    "description",
		Type:           "type",
		Path:           "path",
		FingerprintHex: "print",
		Size:           123,
		Timestamp:      now,
		Username:       "user",
	}
	r := newResource(ResourceArgs{Name: "bdist"})
	r.AddRevision(args0)

	shouldFail := func(args ResourceRevisionArgs, field string) {
		_, err := r.AddRevision(args)
		c.Check(err, gc.ErrorMatches, field+" mismatch for revision 3")
	}

	args := args0
	args.Description = "other"
	shouldFail(args, "description")

	args = args0
	args.Type = "other"
	shouldFail(args, "type")

	args = args0
	args.Path = "other"
	shouldFail(args, "path")

	args = args0
	args.FingerprintHex = "other"
	shouldFail(args, "fingerprint")

	args = args0
	args.Size = 999
	shouldFail(args, "size")

	args = args0
	args.Timestamp = now.Add(time.Hour)
	shouldFail(args, "timestamp")

	args = args0
	args.Username = "other"
	shouldFail(args, "username")
}

func (s *ResourceSuite) TestDuplicateRevisions(c *gc.C) {
	// This shouldn't really be possible but test it anyway.
	r := newResource(ResourceArgs{
		Name:               "bdist",
		Revision:           3,
		CharmStoreRevision: 3,
	})
	r.Revisions_ = []*resourceRevision{
		&resourceRevision{Revision_: 3},
		&resourceRevision{Revision_: 3},
	}
	c.Assert(r.Validate(), gc.ErrorMatches, `revision 3 appears more than once`)
}

func (s *ResourceSuite) TestRoundTrip(c *gc.C) {
	rIn := minimalResource()
	rOut := s.exportImport(c, rIn)
	c.Assert(rOut, jc.DeepEquals, rIn)
}

func (s *ResourceSuite) exportImport(c *gc.C, resourceIn *resource) *resource {
	resourcesIn := &resources{
		Version:    1,
		Resources_: []*resource{resourceIn},
	}
	bytes, err := yaml.Marshal(resourcesIn)
	c.Assert(err, jc.ErrorIsNil)

	var source map[string]interface{}
	err = yaml.Unmarshal(bytes, &source)
	c.Assert(err, jc.ErrorIsNil)

	resourcesOut, err := importResources(source)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(resourcesOut, gc.HasLen, 1)
	return resourcesOut[0]
}

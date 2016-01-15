// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resource_test

import (
	"strings"
	"time"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	charmresource "gopkg.in/juju/charm.v6-unstable/resource"

	"github.com/juju/juju/resource"
)

type SerializedSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&SerializedSuite{})

func (s *SerializedSuite) TestSerializeUploadFull(c *gc.C) {
	content := "some data\n..."
	fp, err := charmresource.GenerateFingerprint(strings.NewReader(content))
	c.Assert(err, jc.ErrorIsNil)
	now := time.Now().UTC()

	serialized := resource.Serialize(resource.Resource{
		Resource: charmresource.Resource{
			Meta: charmresource.Meta{
				Name:    "spam",
				Type:    charmresource.TypeFile,
				Path:    "spam.tgz",
				Comment: "you need this!",
			},
			Origin:      charmresource.OriginUpload,
			Revision:    0,
			Fingerprint: fp,
			Size:        int64(len(content)),
		},
		Username:  "a-user",
		Timestamp: now,
	})

	c.Check(serialized, jc.DeepEquals, resource.Serialized{
		Name:    "spam",
		Type:    "file",
		Path:    "spam.tgz",
		Comment: "you need this!",

		Origin:      "upload",
		Revision:    0,
		Fingerprint: fp.Bytes(),
		Size:        int64(len(content)),

		Username:  "a-user",
		Timestamp: now,
	})
}

func (s *SerializedSuite) TestSerializeUploadBasic(c *gc.C) {
	content := "some data\n..."
	fp, err := charmresource.GenerateFingerprint(strings.NewReader(content))
	c.Assert(err, jc.ErrorIsNil)
	now := time.Now().UTC()

	serialized := resource.Serialize(resource.Resource{
		Resource: charmresource.Resource{
			Meta: charmresource.Meta{
				Name: "spam",
				Type: charmresource.TypeFile,
				Path: "spam.tgz",
			},
			Origin:      charmresource.OriginUpload,
			Fingerprint: fp,
			Size:        int64(len(content)),
		},
		Username:  "a-user",
		Timestamp: now,
	})

	c.Check(serialized, jc.DeepEquals, resource.Serialized{
		Name: "spam",
		Type: "file",
		Path: "spam.tgz",

		Origin:      "upload",
		Fingerprint: fp.Bytes(),
		Size:        int64(len(content)),

		Username:  "a-user",
		Timestamp: now,
	})
}

func (s *SerializedSuite) TestSerializeCharmstoreFull(c *gc.C) {
	content := "some data\n..."
	fp, err := charmresource.GenerateFingerprint(strings.NewReader(content))
	c.Assert(err, jc.ErrorIsNil)
	now := time.Now().UTC()

	serialized := resource.Serialize(resource.Resource{
		Resource: charmresource.Resource{
			Meta: charmresource.Meta{
				Name:    "spam",
				Type:    charmresource.TypeFile,
				Path:    "spam.tgz",
				Comment: "you need this!",
			},
			Origin:      charmresource.OriginStore,
			Revision:    5,
			Fingerprint: fp,
			Size:        int64(len(content)),
		},
		Username:  "a-user",
		Timestamp: now,
	})

	c.Check(serialized, jc.DeepEquals, resource.Serialized{
		Name:    "spam",
		Type:    "file",
		Path:    "spam.tgz",
		Comment: "you need this!",

		Origin:      "store",
		Revision:    5,
		Fingerprint: fp.Bytes(),
		Size:        int64(len(content)),

		Username:  "a-user",
		Timestamp: now,
	})
}

func (s *SerializedSuite) TestSerializePlaceholder(c *gc.C) {
	serialized := resource.Serialize(resource.Resource{
		Resource: charmresource.Resource{
			Meta: charmresource.Meta{
				Name: "spam",
				Type: charmresource.TypeFile,
				Path: "spam.tgz",
			},
			Origin: charmresource.OriginUpload,
		},
	})

	c.Check(serialized, jc.DeepEquals, resource.Serialized{
		Name: "spam",
		Type: "file",
		Path: "spam.tgz",

		Origin: "upload",
	})
}

func (s *SerializedSuite) TestDeserializeUploadFull(c *gc.C) {
	content := "some data\n..."
	fp, err := charmresource.GenerateFingerprint(strings.NewReader(content))
	c.Assert(err, jc.ErrorIsNil)
	now := time.Now().UTC()

	serialized := resource.Serialized{
		Name:    "spam",
		Type:    "file",
		Path:    "spam.tgz",
		Comment: "you need this!",

		Origin:      "upload",
		Revision:    0,
		Fingerprint: fp.Bytes(),
		Size:        int64(len(content)),

		Username:  "a-user",
		Timestamp: now,
	}
	res, err := serialized.Deserialize()
	c.Assert(err, jc.ErrorIsNil)

	c.Check(res, jc.DeepEquals, resource.Resource{
		Resource: charmresource.Resource{
			Meta: charmresource.Meta{
				Name:    "spam",
				Type:    charmresource.TypeFile,
				Path:    "spam.tgz",
				Comment: "you need this!",
			},
			Origin:      charmresource.OriginUpload,
			Revision:    0,
			Fingerprint: fp,
			Size:        int64(len(content)),
		},
		Username:  "a-user",
		Timestamp: now,
	})
}

func (s *SerializedSuite) TestDeserializeUploadBasic(c *gc.C) {
	content := "some data\n..."
	fp, err := charmresource.GenerateFingerprint(strings.NewReader(content))
	c.Assert(err, jc.ErrorIsNil)
	now := time.Now().UTC()

	serialized := resource.Serialized{
		Name: "spam",
		Type: "file",
		Path: "spam.tgz",

		Origin:      "upload",
		Fingerprint: fp.Bytes(),
		Size:        int64(len(content)),

		Username:  "a-user",
		Timestamp: now,
	}
	res, err := serialized.Deserialize()
	c.Assert(err, jc.ErrorIsNil)

	c.Check(res, jc.DeepEquals, resource.Resource{
		Resource: charmresource.Resource{
			Meta: charmresource.Meta{
				Name: "spam",
				Type: charmresource.TypeFile,
				Path: "spam.tgz",
			},
			Origin:      charmresource.OriginUpload,
			Fingerprint: fp,
			Size:        int64(len(content)),
		},
		Username:  "a-user",
		Timestamp: now,
	})
}

func (s *SerializedSuite) TestDeserializeCharmstoreFull(c *gc.C) {
	content := "some data\n..."
	fp, err := charmresource.GenerateFingerprint(strings.NewReader(content))
	c.Assert(err, jc.ErrorIsNil)
	now := time.Now().UTC()

	serialized := resource.Serialized{
		Name:    "spam",
		Type:    "file",
		Path:    "spam.tgz",
		Comment: "you need this!",

		Origin:      "store",
		Revision:    5,
		Fingerprint: fp.Bytes(),
		Size:        int64(len(content)),

		Username:  "a-user",
		Timestamp: now,
	}
	res, err := serialized.Deserialize()
	c.Assert(err, jc.ErrorIsNil)

	c.Check(res, jc.DeepEquals, resource.Resource{
		Resource: charmresource.Resource{
			Meta: charmresource.Meta{
				Name:    "spam",
				Type:    charmresource.TypeFile,
				Path:    "spam.tgz",
				Comment: "you need this!",
			},
			Origin:      charmresource.OriginStore,
			Revision:    5,
			Fingerprint: fp,
			Size:        int64(len(content)),
		},
		Username:  "a-user",
		Timestamp: now,
	})
}

func (s *SerializedSuite) TestDeserializePlaceholder(c *gc.C) {
	serialized := resource.Serialized{
		Name: "spam",
		Type: "file",
		Path: "spam.tgz",

		Origin: "upload",
	}
	res, err := serialized.Deserialize()
	c.Assert(err, jc.ErrorIsNil)

	c.Check(res, jc.DeepEquals, resource.Resource{
		Resource: charmresource.Resource{
			Meta: charmresource.Meta{
				Name: "spam",
				Type: charmresource.TypeFile,
				Path: "spam.tgz",
			},
			Origin: charmresource.OriginUpload,
		},
	})
}

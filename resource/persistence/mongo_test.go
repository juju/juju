// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package persistence

import (
	"strings"
	"time"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	charmresource "gopkg.in/juju/charm.v6-unstable/resource"

	"github.com/juju/juju/resource"
)

type MongoSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&MongoSuite{})

func (s *MongoSuite) TestResource2DocUploadFull(c *gc.C) {
	content := "some data\n..."
	fp, err := charmresource.GenerateFingerprint(strings.NewReader(content))
	c.Assert(err, jc.ErrorIsNil)
	now := time.Now().UTC()

	serviceID := "a-service"
	id := resourceID("spam", serviceID)
	res := resource.Resource{
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
	}
	doc := resource2doc(id, resource.ModelResource{
		ID:        res.Name,
		ServiceID: serviceID,
		Resource:  res,
	})

	c.Check(doc, jc.DeepEquals, &resourceDoc{
		DocID:     id,
		ServiceID: serviceID,

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

func (s *MongoSuite) TestResource2DocUploadBasic(c *gc.C) {
	content := "some data\n..."
	fp, err := charmresource.GenerateFingerprint(strings.NewReader(content))
	c.Assert(err, jc.ErrorIsNil)
	now := time.Now().UTC()

	serviceID := "a-service"
	id := resourceID("spam", serviceID)
	res := resource.Resource{
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
	}
	doc := resource2doc(id, resource.ModelResource{
		ID:        res.Name,
		ServiceID: serviceID,
		Resource:  res,
	})

	c.Check(doc, jc.DeepEquals, &resourceDoc{
		DocID:     id,
		ServiceID: serviceID,

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

func (s *MongoSuite) TestResource2DocUploadPending(c *gc.C) {
	content := "some data\n..."
	fp, err := charmresource.GenerateFingerprint(strings.NewReader(content))
	c.Assert(err, jc.ErrorIsNil)
	now := time.Now().UTC()

	serviceID := "a-service"
	id := resourceID("spam", serviceID)
	res := resource.Resource{
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
	}
	doc := resource2doc(id, resource.ModelResource{
		ID:        res.Name,
		PendingID: "some-unique-ID-001",
		ServiceID: serviceID,
		Resource:  res,
	})

	c.Check(doc, jc.DeepEquals, &resourceDoc{
		DocID:     id,
		ServiceID: serviceID,
		PendingID: "some-unique-ID-001",

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

func (s *MongoSuite) TestDoc2ResourceUploadFull(c *gc.C) {
	serviceID := "a-service"
	id := resourceID("spam", serviceID)
	content := "some data\n..."
	fp, err := charmresource.GenerateFingerprint(strings.NewReader(content))
	c.Assert(err, jc.ErrorIsNil)
	now := time.Now().UTC()

	res, err := doc2resource(resourceDoc{
		DocID:     id,
		ServiceID: serviceID,

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

func (s *MongoSuite) TestDoc2ResourceUploadBasic(c *gc.C) {
	serviceID := "a-service"
	id := resourceID("spam", serviceID)
	content := "some data\n..."
	fp, err := charmresource.GenerateFingerprint(strings.NewReader(content))
	c.Assert(err, jc.ErrorIsNil)
	now := time.Now().UTC()

	res, err := doc2resource(resourceDoc{
		DocID:     id,
		ServiceID: serviceID,

		Name: "spam",
		Type: "file",
		Path: "spam.tgz",

		Origin:      "upload",
		Fingerprint: fp.Bytes(),
		Size:        int64(len(content)),

		Username:  "a-user",
		Timestamp: now,
	})
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

func (s *MongoSuite) TestResource2DocCharmstoreFull(c *gc.C) {
	content := "some data\n..."
	fp, err := charmresource.GenerateFingerprint(strings.NewReader(content))
	c.Assert(err, jc.ErrorIsNil)
	now := time.Now().UTC()

	serviceID := "a-service"
	id := resourceID("spam", serviceID)
	res := resource.Resource{
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
	}
	doc := resource2doc(id, resource.ModelResource{
		ID:        res.Name,
		ServiceID: serviceID,
		Resource:  res,
	})

	c.Check(doc, jc.DeepEquals, &resourceDoc{
		DocID:     id,
		ServiceID: serviceID,

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

func (s *MongoSuite) TestDoc2ResourceCharmstoreFull(c *gc.C) {
	serviceID := "a-service"
	id := resourceID("spam", serviceID)
	content := "some data\n..."
	fp, err := charmresource.GenerateFingerprint(strings.NewReader(content))
	c.Assert(err, jc.ErrorIsNil)
	now := time.Now().UTC()

	res, err := doc2resource(resourceDoc{
		DocID:     id,
		ServiceID: serviceID,

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

func (s *MongoSuite) TestDoc2ResourcePlaceholder(c *gc.C) {
	serviceID := "a-service"
	id := resourceID("spam", serviceID)
	res, err := doc2resource(resourceDoc{
		DocID:     id,
		ServiceID: serviceID,

		Name: "spam",
		Type: "file",
		Path: "spam.tgz",

		Origin: "upload",
	})
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

func (s *MongoSuite) TestResource2DocLocalPlaceholder(c *gc.C) {
	serviceID := "a-service"
	id := resourceID("spam", serviceID)
	res := resource.Resource{
		Resource: charmresource.Resource{
			Meta: charmresource.Meta{
				Name: "spam",
				Type: charmresource.TypeFile,
				Path: "spam.tgz",
			},
			Origin: charmresource.OriginUpload,
		},
	}
	doc := resource2doc(id, resource.ModelResource{
		ID:        res.Name,
		ServiceID: serviceID,
		Resource:  res,
	})

	c.Check(doc, jc.DeepEquals, &resourceDoc{
		DocID:     id,
		ServiceID: serviceID,

		Name: "spam",
		Type: "file",
		Path: "spam.tgz",

		Origin: "upload",
	})
}

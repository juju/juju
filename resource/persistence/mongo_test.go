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
	docID := serviceResourceID("spam")
	res := resource.Resource{
		Resource: charmresource.Resource{
			Meta: charmresource.Meta{
				Name:        "spam",
				Type:        charmresource.TypeFile,
				Path:        "spam.tgz",
				Description: "you need this!",
			},
			Origin:      charmresource.OriginUpload,
			Revision:    0,
			Fingerprint: fp,
			Size:        int64(len(content)),
		},
		ID:        serviceID + "/spam",
		PendingID: "some-unique-ID",
		ServiceID: serviceID,
		Username:  "a-user",
		Timestamp: now,
		Outdated:  true,
	}
	doc := resource2doc(docID, storedResource{
		Resource:    res,
		storagePath: "service-a-service/resources/spam",
	})

	c.Check(doc, jc.DeepEquals, &resourceDoc{
		DocID:     docID,
		ID:        res.ID,
		PendingID: "some-unique-ID",
		ServiceID: serviceID,

		Name:        "spam",
		Type:        "file",
		Path:        "spam.tgz",
		Description: "you need this!",

		Origin:      "upload",
		Revision:    0,
		Fingerprint: fp.Bytes(),
		Size:        int64(len(content)),

		Username:  "a-user",
		Timestamp: now,

		Outdated: true,

		StoragePath: "service-a-service/resources/spam",
	})
}

func (s *MongoSuite) TestResource2DocUploadBasic(c *gc.C) {
	content := "some data\n..."
	fp, err := charmresource.GenerateFingerprint(strings.NewReader(content))
	c.Assert(err, jc.ErrorIsNil)
	now := time.Now().UTC()

	serviceID := "a-service"
	docID := serviceResourceID("spam")
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
		ID:        serviceID + "/spam",
		ServiceID: serviceID,
		Username:  "a-user",
		Timestamp: now,
	}
	doc := resource2doc(docID, storedResource{
		Resource:    res,
		storagePath: "service-a-service/resources/spam",
	})

	c.Check(doc, jc.DeepEquals, &resourceDoc{
		DocID:     docID,
		ID:        res.ID,
		ServiceID: serviceID,

		Name: "spam",
		Type: "file",
		Path: "spam.tgz",

		Origin:      "upload",
		Fingerprint: fp.Bytes(),
		Size:        int64(len(content)),

		Username:  "a-user",
		Timestamp: now,

		StoragePath: "service-a-service/resources/spam",
	})
}

func (s *MongoSuite) TestResource2DocUploadPending(c *gc.C) {
	content := "some data\n..."
	fp, err := charmresource.GenerateFingerprint(strings.NewReader(content))
	c.Assert(err, jc.ErrorIsNil)
	now := time.Now().UTC()

	serviceID := "a-service"
	docID := pendingResourceID("spam", "some-unique-ID-001")
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
		ID:        serviceID + "/spam",
		PendingID: "some-unique-ID-001",
		ServiceID: serviceID,
		Username:  "a-user",
		Timestamp: now,
	}
	doc := resource2doc(docID, storedResource{
		Resource:    res,
		storagePath: "service-a-service/resources/spam",
	})

	c.Check(doc, jc.DeepEquals, &resourceDoc{
		DocID:     docID,
		ID:        res.ID,
		PendingID: "some-unique-ID-001",
		ServiceID: serviceID,

		Name: "spam",
		Type: "file",
		Path: "spam.tgz",

		Origin:      "upload",
		Fingerprint: fp.Bytes(),
		Size:        int64(len(content)),

		Username:  "a-user",
		Timestamp: now,

		StoragePath: "service-a-service/resources/spam",
	})
}

func (s *MongoSuite) TestDoc2Resource(c *gc.C) {
	serviceID := "a-service"
	docID := pendingResourceID("spam", "some-unique-ID-001")
	content := "some data\n..."
	fp, err := charmresource.GenerateFingerprint(strings.NewReader(content))
	c.Assert(err, jc.ErrorIsNil)
	now := time.Now().UTC()

	res, err := doc2resource(resourceDoc{
		DocID:     docID,
		ID:        "a-service/spam",
		PendingID: "some-unique-ID-001",
		ServiceID: serviceID,

		Name: "spam",
		Type: "file",
		Path: "spam.tgz",

		Origin:      "upload",
		Fingerprint: fp.Bytes(),
		Size:        int64(len(content)),

		Username:  "a-user",
		Timestamp: now,

		StoragePath: "service-a-service/resources/spam-some-unique-ID-001",
	})
	c.Assert(err, jc.ErrorIsNil)

	c.Check(res, jc.DeepEquals, storedResource{
		Resource: resource.Resource{
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
			ID:        "a-service/spam",
			PendingID: "some-unique-ID-001",
			ServiceID: serviceID,
			Username:  "a-user",
			Timestamp: now,
		},
		storagePath: "service-a-service/resources/spam-some-unique-ID-001",
	})
}

func (s *MongoSuite) TestDoc2BasicResourceUploadFull(c *gc.C) {
	serviceID := "a-service"
	docID := pendingResourceID("spam", "some-unique-ID-001")
	content := "some data\n..."
	fp, err := charmresource.GenerateFingerprint(strings.NewReader(content))
	c.Assert(err, jc.ErrorIsNil)
	now := time.Now().UTC()

	res, err := doc2basicResource(resourceDoc{
		DocID:     docID,
		ID:        "a-service/spam",
		ServiceID: serviceID,
		PendingID: "some-unique-ID-001",

		Name:        "spam",
		Type:        "file",
		Path:        "spam.tgz",
		Description: "you need this!",

		Origin:      "upload",
		Revision:    0,
		Fingerprint: fp.Bytes(),
		Size:        int64(len(content)),

		Username:  "a-user",
		Timestamp: now,

		Outdated: true,

		StoragePath: "service-a-service/resources/spam",
	})
	c.Assert(err, jc.ErrorIsNil)

	c.Check(res, jc.DeepEquals, resource.Resource{
		Resource: charmresource.Resource{
			Meta: charmresource.Meta{
				Name:        "spam",
				Type:        charmresource.TypeFile,
				Path:        "spam.tgz",
				Description: "you need this!",
			},
			Origin:      charmresource.OriginUpload,
			Revision:    0,
			Fingerprint: fp,
			Size:        int64(len(content)),
		},
		ID:        "a-service/spam",
		PendingID: "some-unique-ID-001",
		ServiceID: serviceID,
		Username:  "a-user",
		Timestamp: now,
		Outdated:  true,
	})
}

func (s *MongoSuite) TestDoc2BasicResourceUploadBasic(c *gc.C) {
	serviceID := "a-service"
	docID := serviceResourceID("spam")
	content := "some data\n..."
	fp, err := charmresource.GenerateFingerprint(strings.NewReader(content))
	c.Assert(err, jc.ErrorIsNil)
	now := time.Now().UTC()

	res, err := doc2basicResource(resourceDoc{
		DocID:     docID,
		ID:        "a-service/spam",
		ServiceID: serviceID,

		Name: "spam",
		Type: "file",
		Path: "spam.tgz",

		Origin:      "upload",
		Fingerprint: fp.Bytes(),
		Size:        int64(len(content)),

		Username:  "a-user",
		Timestamp: now,

		StoragePath: "service-a-service/resources/spam",
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
		ID:        "a-service/spam",
		ServiceID: serviceID,
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
	docID := serviceResourceID("spam")
	res := resource.Resource{
		Resource: charmresource.Resource{
			Meta: charmresource.Meta{
				Name:        "spam",
				Type:        charmresource.TypeFile,
				Path:        "spam.tgz",
				Description: "you need this!",
			},
			Origin:      charmresource.OriginStore,
			Revision:    5,
			Fingerprint: fp,
			Size:        int64(len(content)),
		},
		ID:        "a-service/spam",
		PendingID: "some-unique-ID",
		ServiceID: serviceID,
		Username:  "a-user",
		Timestamp: now,
		Outdated:  true,
	}
	doc := resource2doc(docID, storedResource{
		Resource:    res,
		storagePath: "service-a-service/resources/spam",
	})

	c.Check(doc, jc.DeepEquals, &resourceDoc{
		DocID:     docID,
		ID:        res.ID,
		PendingID: "some-unique-ID",

		ServiceID: serviceID,

		Name:        "spam",
		Type:        "file",
		Path:        "spam.tgz",
		Description: "you need this!",

		Origin:      "store",
		Revision:    5,
		Fingerprint: fp.Bytes(),
		Size:        int64(len(content)),

		Username:  "a-user",
		Timestamp: now,

		Outdated: true,

		StoragePath: "service-a-service/resources/spam",
	})
}

func (s *MongoSuite) TestDoc2BasicResourceCharmstoreFull(c *gc.C) {
	serviceID := "a-service"
	docID := serviceResourceID("spam")
	content := "some data\n..."
	fp, err := charmresource.GenerateFingerprint(strings.NewReader(content))
	c.Assert(err, jc.ErrorIsNil)
	now := time.Now().UTC()

	res, err := doc2basicResource(resourceDoc{
		DocID:     docID,
		ID:        "a-service/spam",
		PendingID: "some-unique-ID",

		ServiceID: serviceID,

		Name:        "spam",
		Type:        "file",
		Path:        "spam.tgz",
		Description: "you need this!",

		Origin:      "store",
		Revision:    5,
		Fingerprint: fp.Bytes(),
		Size:        int64(len(content)),

		Username:  "a-user",
		Timestamp: now,

		Outdated: true,

		StoragePath: "service-a-service/resources/spam",
	})
	c.Assert(err, jc.ErrorIsNil)

	c.Check(res, jc.DeepEquals, resource.Resource{
		Resource: charmresource.Resource{
			Meta: charmresource.Meta{
				Name:        "spam",
				Type:        charmresource.TypeFile,
				Path:        "spam.tgz",
				Description: "you need this!",
			},
			Origin:      charmresource.OriginStore,
			Revision:    5,
			Fingerprint: fp,
			Size:        int64(len(content)),
		},
		ID:        "a-service/spam",
		PendingID: "some-unique-ID",
		ServiceID: serviceID,
		Username:  "a-user",
		Timestamp: now,
		Outdated:  true,
	})
}

func (s *MongoSuite) TestDoc2BasicResourcePlaceholder(c *gc.C) {
	serviceID := "a-service"
	docID := serviceResourceID("spam")
	res, err := doc2basicResource(resourceDoc{
		DocID:     docID,
		ID:        "a-service/spam",
		ServiceID: serviceID,

		Name: "spam",
		Type: "file",
		Path: "spam.tgz",

		Origin: "upload",

		StoragePath: "service-a-service/resources/spam",
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
		ID:        "a-service/spam",
		ServiceID: serviceID,
	})
}

func (s *MongoSuite) TestResource2DocLocalPlaceholder(c *gc.C) {
	serviceID := "a-service"
	docID := serviceResourceID("spam")
	res := resource.Resource{
		Resource: charmresource.Resource{
			Meta: charmresource.Meta{
				Name: "spam",
				Type: charmresource.TypeFile,
				Path: "spam.tgz",
			},
			Origin: charmresource.OriginUpload,
		},
		ID:        "a-service/spam",
		ServiceID: serviceID,
	}
	doc := resource2doc(docID, storedResource{
		Resource:    res,
		storagePath: "service-a-service/resources/spam",
	})

	c.Check(doc, jc.DeepEquals, &resourceDoc{
		DocID:     docID,
		ID:        res.ID,
		ServiceID: serviceID,

		Name: "spam",
		Type: "file",
		Path: "spam.tgz",

		Origin: "upload",

		StoragePath: "service-a-service/resources/spam",
	})
}

// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"strings"

	charmresource "github.com/juju/charm/v7/resource"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/resource"
	coretesting "github.com/juju/juju/testing"
)

type ResourcesMongoSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&ResourcesMongoSuite{})

func (s *ResourcesMongoSuite) TestResource2DocUploadFull(c *gc.C) {
	content := "some data\n..."
	fp, err := charmresource.GenerateFingerprint(strings.NewReader(content))
	c.Assert(err, jc.ErrorIsNil)
	now := coretesting.ZeroTime()

	applicationID := "a-application"
	docID := applicationResourceID("spam")
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
		ID:            applicationID + "/spam",
		PendingID:     "some-unique-ID",
		ApplicationID: applicationID,
		Username:      "a-user",
		Timestamp:     now,
	}
	doc := resource2doc(docID, storedResource{
		Resource:    res,
		storagePath: "application-a-application/resources/spam",
	})

	c.Check(doc, jc.DeepEquals, &resourceDoc{
		DocID:         docID,
		ID:            res.ID,
		PendingID:     "some-unique-ID",
		ApplicationID: applicationID,

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

		StoragePath: "application-a-application/resources/spam",
	})
}

func (s *ResourcesMongoSuite) TestResource2DocUploadBasic(c *gc.C) {
	content := "some data\n..."
	fp, err := charmresource.GenerateFingerprint(strings.NewReader(content))
	c.Assert(err, jc.ErrorIsNil)
	now := coretesting.ZeroTime()

	applicationID := "a-application"
	docID := applicationResourceID("spam")
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
		ID:            applicationID + "/spam",
		ApplicationID: applicationID,
		Username:      "a-user",
		Timestamp:     now,
	}
	doc := resource2doc(docID, storedResource{
		Resource:    res,
		storagePath: "application-a-application/resources/spam",
	})

	c.Check(doc, jc.DeepEquals, &resourceDoc{
		DocID:         docID,
		ID:            res.ID,
		ApplicationID: applicationID,

		Name: "spam",
		Type: "file",
		Path: "spam.tgz",

		Origin:      "upload",
		Fingerprint: fp.Bytes(),
		Size:        int64(len(content)),

		Username:  "a-user",
		Timestamp: now,

		StoragePath: "application-a-application/resources/spam",
	})
}

func (s *ResourcesMongoSuite) TestResource2DocUploadPending(c *gc.C) {
	content := "some data\n..."
	fp, err := charmresource.GenerateFingerprint(strings.NewReader(content))
	c.Assert(err, jc.ErrorIsNil)
	now := coretesting.ZeroTime()

	applicationID := "a-application"
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
		ID:            applicationID + "/spam",
		PendingID:     "some-unique-ID-001",
		ApplicationID: applicationID,
		Username:      "a-user",
		Timestamp:     now,
	}
	doc := resource2doc(docID, storedResource{
		Resource:    res,
		storagePath: "application-a-application/resources/spam",
	})

	c.Check(doc, jc.DeepEquals, &resourceDoc{
		DocID:         docID,
		ID:            res.ID,
		PendingID:     "some-unique-ID-001",
		ApplicationID: applicationID,

		Name: "spam",
		Type: "file",
		Path: "spam.tgz",

		Origin:      "upload",
		Fingerprint: fp.Bytes(),
		Size:        int64(len(content)),

		Username:  "a-user",
		Timestamp: now,

		StoragePath: "application-a-application/resources/spam",
	})
}

func (s *ResourcesMongoSuite) TestDoc2Resource(c *gc.C) {
	applicationID := "a-application"
	docID := pendingResourceID("spam", "some-unique-ID-001")
	content := "some data\n..."
	fp, err := charmresource.GenerateFingerprint(strings.NewReader(content))
	c.Assert(err, jc.ErrorIsNil)
	now := coretesting.NonZeroTime()

	res, err := doc2resource(resourceDoc{
		DocID:         docID,
		ID:            "a-application/spam",
		PendingID:     "some-unique-ID-001",
		ApplicationID: applicationID,

		Name: "spam",
		Type: "file",
		Path: "spam.tgz",

		Origin:      "upload",
		Fingerprint: fp.Bytes(),
		Size:        int64(len(content)),

		Username:  "a-user",
		Timestamp: now,

		StoragePath: "application-a-application/resources/spam-some-unique-ID-001",
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
			ID:            "a-application/spam",
			PendingID:     "some-unique-ID-001",
			ApplicationID: applicationID,
			Username:      "a-user",
			Timestamp:     now,
		},
		storagePath: "application-a-application/resources/spam-some-unique-ID-001",
	})
}

func (s *ResourcesMongoSuite) TestDoc2BasicResourceUploadFull(c *gc.C) {
	applicationID := "a-application"
	docID := pendingResourceID("spam", "some-unique-ID-001")
	content := "some data\n..."
	fp, err := charmresource.GenerateFingerprint(strings.NewReader(content))
	c.Assert(err, jc.ErrorIsNil)
	now := coretesting.NonZeroTime()

	res, err := doc2basicResource(resourceDoc{
		DocID:         docID,
		ID:            "a-application/spam",
		ApplicationID: applicationID,
		PendingID:     "some-unique-ID-001",

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

		StoragePath: "application-a-application/resources/spam",
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
		ID:            "a-application/spam",
		PendingID:     "some-unique-ID-001",
		ApplicationID: applicationID,
		Username:      "a-user",
		Timestamp:     now,
	})
}

func (s *ResourcesMongoSuite) TestDoc2BasicResourceUploadBasic(c *gc.C) {
	applicationID := "a-application"
	docID := applicationResourceID("spam")
	content := "some data\n..."
	fp, err := charmresource.GenerateFingerprint(strings.NewReader(content))
	c.Assert(err, jc.ErrorIsNil)
	now := coretesting.NonZeroTime()

	res, err := doc2basicResource(resourceDoc{
		DocID:         docID,
		ID:            "a-application/spam",
		ApplicationID: applicationID,

		Name: "spam",
		Type: "file",
		Path: "spam.tgz",

		Origin:      "upload",
		Fingerprint: fp.Bytes(),
		Size:        int64(len(content)),

		Username:  "a-user",
		Timestamp: now,

		StoragePath: "application-a-application/resources/spam",
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
		ID:            "a-application/spam",
		ApplicationID: applicationID,
		Username:      "a-user",
		Timestamp:     now,
	})
}

func (s *ResourcesMongoSuite) TestResource2DocCharmstoreFull(c *gc.C) {
	content := "some data\n..."
	fp, err := charmresource.GenerateFingerprint(strings.NewReader(content))
	c.Assert(err, jc.ErrorIsNil)
	now := coretesting.ZeroTime()

	applicationID := "a-application"
	docID := applicationResourceID("spam")
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
		ID:            "a-application/spam",
		PendingID:     "some-unique-ID",
		ApplicationID: applicationID,
		Username:      "a-user",
		Timestamp:     now,
	}
	doc := resource2doc(docID, storedResource{
		Resource:    res,
		storagePath: "application-a-application/resources/spam",
	})

	c.Check(doc, jc.DeepEquals, &resourceDoc{
		DocID:     docID,
		ID:        res.ID,
		PendingID: "some-unique-ID",

		ApplicationID: applicationID,

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

		StoragePath: "application-a-application/resources/spam",
	})
}

func (s *ResourcesMongoSuite) TestDoc2BasicResourceCharmstoreFull(c *gc.C) {
	applicationID := "a-application"
	docID := applicationResourceID("spam")
	content := "some data\n..."
	fp, err := charmresource.GenerateFingerprint(strings.NewReader(content))
	c.Assert(err, jc.ErrorIsNil)
	now := coretesting.NonZeroTime()

	res, err := doc2basicResource(resourceDoc{
		DocID:     docID,
		ID:        "a-application/spam",
		PendingID: "some-unique-ID",

		ApplicationID: applicationID,

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

		StoragePath: "application-a-application/resources/spam",
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
		ID:            "a-application/spam",
		PendingID:     "some-unique-ID",
		ApplicationID: applicationID,
		Username:      "a-user",
		Timestamp:     now,
	})
}

func (s *ResourcesMongoSuite) TestDoc2BasicResourcePlaceholder(c *gc.C) {
	applicationID := "a-application"
	docID := applicationResourceID("spam")
	res, err := doc2basicResource(resourceDoc{
		DocID:         docID,
		ID:            "a-application/spam",
		ApplicationID: applicationID,

		Name: "spam",
		Type: "file",
		Path: "spam.tgz",

		Origin: "upload",

		StoragePath: "application-a-application/resources/spam",
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
		ID:            "a-application/spam",
		ApplicationID: applicationID,
	})
}

func (s *ResourcesMongoSuite) TestResource2DocLocalPlaceholder(c *gc.C) {
	applicationID := "a-application"
	docID := applicationResourceID("spam")
	res := resource.Resource{
		Resource: charmresource.Resource{
			Meta: charmresource.Meta{
				Name: "spam",
				Type: charmresource.TypeFile,
				Path: "spam.tgz",
			},
			Origin: charmresource.OriginUpload,
		},
		ID:            "a-application/spam",
		ApplicationID: applicationID,
	}
	doc := resource2doc(docID, storedResource{
		Resource:    res,
		storagePath: "application-a-application/resources/spam",
	})

	c.Check(doc, jc.DeepEquals, &resourceDoc{
		DocID:         docID,
		ID:            res.ID,
		ApplicationID: applicationID,

		Name: "spam",
		Type: "file",
		Path: "spam.tgz",

		Origin: "upload",

		StoragePath: "application-a-application/resources/spam",
	})
}

func (s *ResourcesMongoSuite) TestCharmStoreResource2DocFull(c *gc.C) {
	content := "some data\n..."
	fp, err := charmresource.GenerateFingerprint(strings.NewReader(content))
	c.Assert(err, jc.ErrorIsNil)
	now := coretesting.ZeroTime()

	applicationID := "a-application"
	id := applicationID + "/spam"
	docID := applicationResourceID("spam") + "#charmstore"
	res := charmresource.Resource{
		Meta: charmresource.Meta{
			Name:        "spam",
			Type:        charmresource.TypeFile,
			Path:        "spam.tgz",
			Description: "you need this!",
		},
		Origin:      charmresource.OriginStore,
		Revision:    3,
		Fingerprint: fp,
		Size:        int64(len(content)),
	}
	doc := charmStoreResource2Doc(docID, charmStoreResource{
		Resource:      res,
		id:            id,
		applicationID: applicationID,
		lastPolled:    now,
	})

	c.Check(doc, jc.DeepEquals, &resourceDoc{
		DocID:         docID,
		ID:            id,
		ApplicationID: applicationID,

		Name:        "spam",
		Type:        "file",
		Path:        "spam.tgz",
		Description: "you need this!",

		Origin:      "store",
		Revision:    3,
		Fingerprint: fp.Bytes(),
		Size:        int64(len(content)),

		LastPolled: now,
	})
}

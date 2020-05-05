// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/resource"
	"github.com/juju/juju/resource/resourcetesting"
	"github.com/juju/juju/state/statetest"
	coretesting "github.com/juju/juju/testing"
)

var _ = gc.Suite(&ResourcePersistenceSuite{})

type ResourcePersistenceSuite struct {
	testing.IsolationSuite

	stub *testing.Stub
	base *statetest.StubPersistence
}

func (s *ResourcePersistenceSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.stub = &testing.Stub{}
	s.base = statetest.NewStubPersistence(s.stub)
	s.base.ReturnApplicationExistsOps = []txn.Op{{
		C:      "application",
		Id:     "a-application",
		Assert: txn.DocExists,
	}}
}

func (s *ResourcePersistenceSuite) TestListResourcesOkay(c *gc.C) {
	expected, docs := newPersistenceResources(c, "a-application", "spam", "eggs")
	expected.CharmStoreResources[1].Revision += 1
	docs[3].Revision += 1
	unitRes, unitDocs := newPersistenceUnitResources(c, "a-application", "a-application/0", expected.Resources)
	var progress int64 = 3
	unitDocs[1].DownloadProgress = &progress // the "eggs" doc
	expected.UnitResources = []resource.UnitResources{{
		Tag:       names.NewUnitTag("a-application/0"),
		Resources: unitRes,
		DownloadProgress: map[string]int64{
			"eggs": progress,
		},
	}}
	docs = append(docs, unitDocs...)
	s.base.ReturnAll = docs
	p := NewResourcePersistence(s.base)

	resources, err := p.ListResources("a-application")
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "All")
	s.stub.CheckCall(c, 0, "All",
		"resources",
		bson.D{{"application-id", "a-application"}},
		&docs,
	)
	c.Check(resources, jc.DeepEquals, expected)
}

func (s *ResourcePersistenceSuite) TestListResourcesNoResources(c *gc.C) {
	p := NewResourcePersistence(s.base)
	resources, err := p.ListResources("a-application")
	c.Assert(err, jc.ErrorIsNil)

	c.Check(resources.Resources, gc.HasLen, 0)
	s.stub.CheckCallNames(c, "All")
	s.stub.CheckCall(c, 0, "All",
		"resources",
		bson.D{{"application-id", "a-application"}},
		&[]resourceDoc{},
	)
}

func (s *ResourcePersistenceSuite) TestListResourcesIgnorePending(c *gc.C) {
	expected, docs := newPersistenceResources(c, "a-application", "spam", "eggs")
	expected.Resources = expected.Resources[:1]
	docs[2].PendingID = "some-unique-ID-001"
	s.base.ReturnAll = docs
	p := NewResourcePersistence(s.base)

	resources, err := p.ListResources("a-application")
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "All")
	s.stub.CheckCall(c, 0, "All",
		"resources",
		bson.D{{"application-id", "a-application"}},
		&docs,
	)
	checkResources(c, resources, expected)
}

func (s *ResourcePersistenceSuite) TestListResourcesBaseError(c *gc.C) {
	failure := errors.New("<failure>")
	s.stub.SetErrors(failure)

	p := NewResourcePersistence(s.base)
	_, err := p.ListResources("a-application")

	c.Check(errors.Cause(err), gc.Equals, failure)
	s.stub.CheckCallNames(c, "All")
	s.stub.CheckCall(c, 0, "All",
		"resources",
		bson.D{{"application-id", "a-application"}},
		&[]resourceDoc{},
	)
}

func (s *ResourcePersistenceSuite) TestListResourcesBadDoc(c *gc.C) {
	_, docs := newPersistenceResources(c, "a-application", "spam", "eggs")
	docs[0].Timestamp = coretesting.ZeroTime()
	s.base.ReturnAll = docs

	p := NewResourcePersistence(s.base)
	_, err := p.ListResources("a-application")

	c.Check(err, gc.ErrorMatches, `got invalid data from DB.*`)
	s.stub.CheckCallNames(c, "All")
	s.stub.CheckCall(c, 0, "All",
		"resources",
		bson.D{{"application-id", "a-application"}},
		&docs,
	)
}

func (s *ResourcePersistenceSuite) TestListPendingResourcesOkay(c *gc.C) {
	var expected []resource.Resource
	var docs []resourceDoc
	for _, name := range []string{"spam", "ham"} {
		res, doc := newPersistenceResource(c, "a-application", name)
		expected = append(expected, res.Resource)
		docs = append(docs, doc)
	}
	expected = expected[1:]
	expected[0].PendingID = "some-unique-ID-001"
	docs[1].PendingID = "some-unique-ID-001"
	s.base.ReturnAll = docs
	p := NewResourcePersistence(s.base)

	resources, err := p.ListPendingResources("a-application")
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "All")
	s.stub.CheckCall(c, 0, "All",
		"resources",
		bson.D{{"application-id", "a-application"}},
		&docs,
	)
	checkBasicResources(c, resources, expected)
}

func (s *ResourcePersistenceSuite) TestGetResourceOkay(c *gc.C) {
	expected, doc := newPersistenceResource(c, "a-application", "spam")
	unitDoc := doc // a copy
	unitDoc.ID = doc.ID + "#unit-a-application/0"
	unitDoc.UnitID = "a-application/0"
	pendingDoc := doc // a copy
	pendingDoc.ID = doc.ID + "#pending-some-unique-ID"
	pendingDoc.PendingID = "some-unique-ID"
	s.base.ReturnAll = []resourceDoc{
		doc,
		unitDoc,
		pendingDoc,
	}
	s.base.ReturnOne = doc
	p := NewResourcePersistence(s.base)

	res, storagePath, err := p.GetResource("a-application/spam")
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "One")
	s.stub.CheckCall(c, 0, "One", "resources", "resource#a-application/spam", &doc)
	c.Check(res, jc.DeepEquals, expected.Resource)
	c.Check(storagePath, gc.Equals, expected.storagePath)
}

func (s *ResourcePersistenceSuite) TestStageResourceOkay(c *gc.C) {
	res, doc := newPersistenceResource(c, "a-application", "spam")
	doc.DocID += "#staged"
	p := NewResourcePersistence(s.base)
	ignoredErr := errors.New("<never reached>")
	s.stub.SetErrors(nil, nil, nil, ignoredErr)

	staged, err := p.StageResource(res.Resource, res.storagePath)
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "Run", "ApplicationExistsOps", "RunTransaction")
	s.stub.CheckCall(c, 2, "RunTransaction", []txn.Op{{
		C:      "resources",
		Id:     "resource#a-application/spam#staged",
		Assert: txn.DocMissing,
		Insert: &doc,
	}, {
		C:      "application",
		Id:     "a-application",
		Assert: txn.DocExists,
	}})
	c.Check(staged, jc.DeepEquals, &StagedResource{
		base:   s.base,
		id:     res.ID,
		stored: res,
	})
}

func (s *ResourcePersistenceSuite) TestStageResourceMissingStoragePath(c *gc.C) {
	res, _ := newPersistenceResource(c, "a-application", "spam")
	p := NewResourcePersistence(s.base)

	_, err := p.StageResource(res.Resource, "")

	s.stub.CheckNoCalls(c)
	c.Check(err, gc.ErrorMatches, `missing storage path`)
}

func (s *ResourcePersistenceSuite) TestStageResourceBadResource(c *gc.C) {
	res, _ := newPersistenceResource(c, "a-application", "spam")
	res.Resource.Timestamp = coretesting.ZeroTime()
	p := NewResourcePersistence(s.base)

	_, err := p.StageResource(res.Resource, res.storagePath)

	c.Check(err, jc.Satisfies, errors.IsNotValid)
	c.Check(err, gc.ErrorMatches, `bad resource.*`)

	s.stub.CheckNoCalls(c)
}

func (s *ResourcePersistenceSuite) TestSetResourceOkay(c *gc.C) {
	applicationname := "a-application"
	res, doc := newPersistenceResource(c, applicationname, "spam")
	s.base.ReturnOne = doc
	p := NewResourcePersistence(s.base)
	ignoredErr := errors.New("<never reached>")
	s.stub.SetErrors(nil, nil, nil, nil, ignoredErr)

	err := p.SetResource(res.Resource)
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c,
		"One",
		"Run",
		"ApplicationExistsOps",
		"RunTransaction",
	)
	s.stub.CheckCall(c, 3, "RunTransaction", []txn.Op{{
		C:      "resources",
		Id:     "resource#a-application/spam",
		Assert: txn.DocMissing,
		Insert: &doc,
	}, {
		C:      "application",
		Id:     "a-application",
		Assert: txn.DocExists,
	}})
}

func (s *ResourcePersistenceSuite) TestSetResourceNotFound(c *gc.C) {
	applicationname := "a-application"
	res, doc := newPersistenceResource(c, applicationname, "spam")
	s.base.ReturnOne = doc
	expected := doc // a copy
	expected.StoragePath = ""
	p := NewResourcePersistence(s.base)
	notFound := errors.NewNotFound(nil, "")
	ignoredErr := errors.New("<never reached>")
	s.stub.SetErrors(notFound, nil, nil, nil, ignoredErr)

	err := p.SetResource(res.Resource)
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c,
		"One",
		"Run",
		"ApplicationExistsOps",
		"RunTransaction",
	)
	s.stub.CheckCall(c, 3, "RunTransaction", []txn.Op{{
		C:      "resources",
		Id:     "resource#a-application/spam",
		Assert: txn.DocMissing,
		Insert: &expected,
	}, {
		C:      "application",
		Id:     "a-application",
		Assert: txn.DocExists,
	}})
}

func (s *ResourcePersistenceSuite) TestSetCharmStoreResourceOkay(c *gc.C) {
	lastPolled := coretesting.NonZeroTime().UTC()
	applicationname := "a-application"
	res, doc := newPersistenceResource(c, applicationname, "spam")
	expected := doc // a copy
	expected.DocID += "#charmstore"
	expected.Username = ""
	expected.Timestamp = coretesting.ZeroTime()
	expected.StoragePath = ""
	expected.LastPolled = lastPolled
	p := NewResourcePersistence(s.base)
	ignoredErr := errors.New("<never reached>")
	s.stub.SetErrors(nil, nil, nil, ignoredErr)

	err := p.SetCharmStoreResource(res.ID, res.ApplicationID, res.Resource.Resource, lastPolled)
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c,
		"Run",
		"ApplicationExistsOps",
		"RunTransaction",
	)
	s.stub.CheckCall(c, 2, "RunTransaction", []txn.Op{{
		C:      "resources",
		Id:     "resource#a-application/spam#charmstore",
		Assert: txn.DocMissing,
		Insert: &expected,
	}, {
		C:      "application",
		Id:     "a-application",
		Assert: txn.DocExists,
	}})
}

func (s *ResourcePersistenceSuite) TestSetUnitResourceOkay(c *gc.C) {
	applicationname := "a-application"
	unitname := "a-application/0"
	res, doc := newPersistenceUnitResource(c, applicationname, unitname, "eggs")
	s.base.ReturnOne = doc
	p := NewResourcePersistence(s.base)
	ignoredErr := errors.New("<never reached>")
	s.stub.SetErrors(nil, nil, nil, nil, ignoredErr)

	err := p.SetUnitResource("a-application/0", res)
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "One", "Run", "ApplicationExistsOps", "RunTransaction")
	s.stub.CheckCall(c, 3, "RunTransaction", []txn.Op{{
		C:      "resources",
		Id:     "resource#a-application/eggs#unit-a-application/0",
		Assert: txn.DocMissing,
		Insert: &doc,
	}, {
		C:      "application",
		Id:     "a-application",
		Assert: txn.DocExists,
	}})
}

func (s *ResourcePersistenceSuite) TestSetUnitResourceNotFound(c *gc.C) {
	applicationname := "a-application"
	unitname := "a-application/0"
	res, _ := newPersistenceUnitResource(c, applicationname, unitname, "eggs")
	p := NewResourcePersistence(s.base)
	notFound := errors.NewNotFound(nil, "")
	s.stub.SetErrors(notFound)

	err := p.SetUnitResource("a-application/0", res)

	s.stub.CheckCallNames(c, "One")
	c.Check(err, jc.Satisfies, errors.IsNotFound)
	c.Check(err, gc.ErrorMatches, `resource "eggs" not found`)
}

func (s *ResourcePersistenceSuite) TestSetUnitResourceExists(c *gc.C) {
	res, doc := newPersistenceUnitResource(c, "a-application", "a-application/0", "spam")
	s.base.ReturnOne = doc
	p := NewResourcePersistence(s.base)
	ignoredErr := errors.New("<never reached>")
	s.stub.SetErrors(nil, nil, nil, txn.ErrAborted, nil, nil, ignoredErr)

	err := p.SetUnitResource("a-application/0", res)
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "One", "Run", "ApplicationExistsOps", "RunTransaction", "ApplicationExistsOps", "RunTransaction")
	s.stub.CheckCall(c, 3, "RunTransaction", []txn.Op{{
		C:      "resources",
		Id:     "resource#a-application/spam#unit-a-application/0",
		Assert: txn.DocMissing,
		Insert: &doc,
	}, {
		C:      "application",
		Id:     "a-application",
		Assert: txn.DocExists,
	}})
	s.stub.CheckCall(c, 5, "RunTransaction", []txn.Op{{
		C:      "resources",
		Id:     "resource#a-application/spam#unit-a-application/0",
		Assert: txn.DocExists,
		Update: bson.M{"$set": bson.M{
			"resource-id":                doc.ID,
			"pending-id":                 doc.PendingID,
			"application-id":             doc.ApplicationID,
			"unit-id":                    doc.UnitID,
			"name":                       doc.Name,
			"type":                       doc.Type,
			"path":                       doc.Path,
			"description":                doc.Description,
			"origin":                     doc.Origin,
			"revision":                   doc.Revision,
			"fingerprint":                doc.Fingerprint,
			"size":                       doc.Size,
			"username":                   doc.Username,
			"timestamp-when-added":       doc.Timestamp,
			"storage-path":               doc.StoragePath,
			"download-progress":          doc.DownloadProgress,
			"timestamp-when-last-polled": doc.LastPolled,
		}},
	}, {
		C:      "application",
		Id:     "a-application",
		Assert: txn.DocExists,
	}})
}

func (s *ResourcePersistenceSuite) TestSetUnitResourceBadResource(c *gc.C) {
	res, doc := newPersistenceUnitResource(c, "a-application", "a-application/0", "spam")
	s.base.ReturnOne = doc
	res.Timestamp = coretesting.ZeroTime()
	p := NewResourcePersistence(s.base)

	err := p.SetUnitResource("a-application/0", res)

	c.Check(err, jc.Satisfies, errors.IsNotValid)
	c.Check(err, gc.ErrorMatches, `bad resource.*`)

	s.stub.CheckCallNames(c, "One")
}

func (s *ResourcePersistenceSuite) TestSetUnitResourceProgress(c *gc.C) {
	applicationname := "a-application"
	unitname := "a-application/0"
	res, doc := newPersistenceUnitResource(c, applicationname, unitname, "eggs")
	s.base.ReturnOne = doc
	pendingID := "<a pending ID>"
	res.PendingID = pendingID
	expected := doc // a copy
	expected.PendingID = pendingID
	var progress int64 = 2
	expected.DownloadProgress = &progress
	p := NewResourcePersistence(s.base)
	ignoredErr := errors.New("<never reached>")
	s.stub.SetErrors(nil, nil, nil, nil, ignoredErr)

	err := p.SetUnitResourceProgress("a-application/0", res, progress)
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "One", "Run", "ApplicationExistsOps", "RunTransaction")
	s.stub.CheckCall(c, 3, "RunTransaction", []txn.Op{{
		C:      "resources",
		Id:     "resource#a-application/eggs#unit-a-application/0",
		Assert: txn.DocMissing,
		Insert: &expected,
	}, {
		C:      "application",
		Id:     "a-application",
		Assert: txn.DocExists,
	}})
}

func (s *ResourcePersistenceSuite) TestNewResourcePendingResourceOpsExists(c *gc.C) {
	pendingID := "some-unique-ID-001"
	stored, expected := newPersistenceResource(c, "a-application", "spam")
	stored.PendingID = pendingID
	doc := expected // a copy
	doc.DocID = pendingResourceID(stored.ID, pendingID)
	doc.PendingID = pendingID
	s.base.ReturnOne = doc
	p := NewResourcePersistence(s.base)

	// TODO(macgreagoir) We need to keep using time.Now() for now, while we
	// have NewResolvePendingResourceOps returning LastPolled based on
	// timeNow(). lp:1558657
	// Note: truncate the time to remove monotonic time for Go 1.9+.
	lastPolled := time.Now().UTC().Round(time.Second).Truncate(1)

	ops, err := p.NewResolvePendingResourceOps(stored.ID, stored.PendingID)
	c.Assert(err, jc.ErrorIsNil)

	csresourceDoc := expected
	csresourceDoc.DocID = "resource#a-application/spam#charmstore"
	csresourceDoc.Username = ""
	csresourceDoc.Timestamp = coretesting.ZeroTime()
	csresourceDoc.StoragePath = ""
	csresourceDoc.LastPolled = lastPolled

	res := ops[2].Update.(bson.M)["$set"].(bson.M)
	res["timestamp-when-last-polled"] = res["timestamp-when-last-polled"].(time.Time).Round(time.Second)

	s.stub.CheckCallNames(c, "One", "One")
	s.stub.CheckCall(c, 0, "One", "resources", "resource#a-application/spam#pending-some-unique-ID-001", &doc)
	c.Check(ops, jc.DeepEquals, []txn.Op{{
		C:      "resources",
		Id:     doc.DocID,
		Assert: txn.DocExists,
		Remove: true,
	}, {
		C:      "resources",
		Id:     expected.DocID,
		Assert: txn.DocExists,
		Update: bson.M{"$set": bson.M{
			"resource-id":                expected.ID,
			"pending-id":                 expected.PendingID,
			"application-id":             expected.ApplicationID,
			"unit-id":                    expected.UnitID,
			"name":                       expected.Name,
			"type":                       expected.Type,
			"path":                       expected.Path,
			"description":                expected.Description,
			"origin":                     expected.Origin,
			"revision":                   expected.Revision,
			"fingerprint":                expected.Fingerprint,
			"size":                       expected.Size,
			"username":                   expected.Username,
			"timestamp-when-added":       expected.Timestamp,
			"storage-path":               expected.StoragePath,
			"download-progress":          expected.DownloadProgress,
			"timestamp-when-last-polled": expected.LastPolled,
		}},
	}, {
		C:      "resources",
		Id:     csresourceDoc.DocID,
		Assert: txn.DocExists,
		Update: bson.M{"$set": bson.M{
			"resource-id":                csresourceDoc.ID,
			"pending-id":                 csresourceDoc.PendingID,
			"application-id":             csresourceDoc.ApplicationID,
			"unit-id":                    csresourceDoc.UnitID,
			"name":                       csresourceDoc.Name,
			"type":                       csresourceDoc.Type,
			"path":                       csresourceDoc.Path,
			"description":                csresourceDoc.Description,
			"origin":                     csresourceDoc.Origin,
			"revision":                   csresourceDoc.Revision,
			"fingerprint":                csresourceDoc.Fingerprint,
			"size":                       csresourceDoc.Size,
			"username":                   csresourceDoc.Username,
			"timestamp-when-added":       csresourceDoc.Timestamp,
			"storage-path":               csresourceDoc.StoragePath,
			"download-progress":          csresourceDoc.DownloadProgress,
			"timestamp-when-last-polled": csresourceDoc.LastPolled,
		}},
	},
	})
}

func (s *ResourcePersistenceSuite) TestNewResourcePendingResourceOpsNotFound(c *gc.C) {
	pendingID := "some-unique-ID-001"
	stored, expected := newPersistenceResource(c, "a-application", "spam")
	stored.PendingID = pendingID
	doc := expected // a copy
	doc.DocID = pendingResourceID(stored.ID, pendingID)
	doc.PendingID = pendingID
	s.base.ReturnOne = doc
	notFound := errors.NewNotFound(nil, "")
	s.stub.SetErrors(nil, notFound)
	p := NewResourcePersistence(s.base)

	// TODO(macgreagoir) We need to keep using time.Now() for now, while we
	// have NewResolvePendingResourceOps returning LastPolled based on
	// timeNow(). lp:1558657
	lastPolled := time.Now().UTC().Round(time.Second)
	ops, err := p.NewResolvePendingResourceOps(stored.ID, stored.PendingID)
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "One", "One")
	s.stub.CheckCall(c, 0, "One", "resources", "resource#a-application/spam#pending-some-unique-ID-001", &doc)

	csresourceDoc := expected
	csresourceDoc.DocID = "resource#a-application/spam#charmstore"
	csresourceDoc.Username = ""
	csresourceDoc.Timestamp = coretesting.ZeroTime()
	csresourceDoc.StoragePath = ""
	csresourceDoc.LastPolled = lastPolled

	res := ops[2].Insert.(*resourceDoc)
	res.LastPolled = res.LastPolled.Round(time.Second)

	c.Check(ops, jc.DeepEquals, []txn.Op{
		{
			C:      "resources",
			Id:     doc.DocID,
			Assert: txn.DocExists,
			Remove: true,
		}, {
			C:      "resources",
			Id:     expected.DocID,
			Assert: txn.DocMissing,
			Insert: &expected,
		},
		{
			C:      "resources",
			Id:     csresourceDoc.DocID,
			Assert: txn.DocMissing,
			Insert: &csresourceDoc,
		},
	})
}

func (s *ResourcePersistenceSuite) TestRemoveResourcesCleansUpUniqueStoragePaths(c *gc.C) {
	// We shouldn't schedule multiple cleanups for the same path (when
	// application and units use the same resource).
	appResource, appDoc := newPersistenceResource(c, "appa", "yipyip")
	_, unitDoc := newPersistenceUnitResource(c, "appa", "appa/0", "yipyip")
	s.base.ReturnAll = []resourceDoc{appDoc, unitDoc}
	p := NewResourcePersistence(s.base)

	ops, err := p.NewRemoveResourcesOps("appa")
	c.Assert(err, jc.ErrorIsNil)

	var cleanups []txn.Op
	for _, op := range ops {
		if op.C == cleanupsC {
			cleanups = append(cleanups, op)
		}
	}
	c.Assert(cleanups, gc.HasLen, 1)
	c.Assert(cleanups[0].Insert, gc.Not(gc.IsNil))
	c.Assert(cleanups[0].Insert.(*cleanupDoc).Kind, gc.Equals, cleanupResourceBlob)
	c.Assert(cleanups[0].Insert.(*cleanupDoc).Prefix, gc.Equals, appResource.storagePath)
}

func (s *ResourcePersistenceSuite) TestRemovePendingAppResources(c *gc.C) {
	_, appDoc1 := newPersistenceResource(c, "appa", "yipyip")
	appDoc1.DocID += "#pending-freewifi"
	appDoc1.PendingID = "freewifi"
	appResource2, appDoc2 := newPersistenceResource(c, "appa", "momo")
	appDoc2.DocID += "#pending-parallax"
	appDoc2.PendingID = "parallax"
	_, unitDoc := newPersistenceUnitResource(c, "appa", "appa/0", "momo")
	unitDoc.DocID += "#pending-parallax"
	unitDoc.PendingID = "parallax"
	s.base.ReturnAll = []resourceDoc{appDoc1, appDoc2, unitDoc}
	p := NewResourcePersistence(s.base)

	ops, err := p.NewRemovePendingAppResourcesOps("appa", map[string]string{"momo": "parallax"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ops, gc.HasLen, 2)
	c.Assert(ops[0], gc.DeepEquals, txn.Op{
		C:      resourcesC,
		Id:     appDoc2.DocID,
		Remove: true,
	})
	c.Assert(ops[1].Insert, gc.Not(gc.IsNil))
	c.Assert(ops[1].Insert.(*cleanupDoc).Kind, gc.Equals, cleanupResourceBlob)
	c.Assert(ops[1].Insert.(*cleanupDoc).Prefix, gc.Equals, appResource2.storagePath)
}

func newPersistenceUnitResources(c *gc.C, applicationID, unitID string, resources []resource.Resource) ([]resource.Resource, []resourceDoc) {
	var unitResources []resource.Resource
	var docs []resourceDoc
	for _, res := range resources {
		res, doc := newPersistenceUnitResource(c, applicationID, unitID, res.Name)
		unitResources = append(unitResources, res)
		docs = append(docs, doc)
	}
	return unitResources, docs
}

func newPersistenceUnitResource(c *gc.C, applicationID, unitID, name string) (resource.Resource, resourceDoc) {
	res, doc := newPersistenceResource(c, applicationID, name)
	doc.DocID += "#unit-" + unitID
	doc.UnitID = unitID
	return res.Resource, doc
}

func newPersistenceResources(c *gc.C, applicationID string, names ...string) (resource.ApplicationResources, []resourceDoc) {
	var appResources resource.ApplicationResources
	var docs []resourceDoc
	for _, name := range names {
		res, doc := newPersistenceResource(c, applicationID, name)
		appResources.Resources = append(appResources.Resources, res.Resource)
		appResources.CharmStoreResources = append(appResources.CharmStoreResources, res.Resource.Resource)
		docs = append(docs, doc)
		csDoc := doc // a copy
		csDoc.DocID += "#charmstore"
		csDoc.Username = ""
		csDoc.Timestamp = coretesting.ZeroTime()
		csDoc.StoragePath = ""
		csDoc.LastPolled = coretesting.NonZeroTime().UTC()
		docs = append(docs, csDoc)
	}
	return appResources, docs
}

func newPersistenceResource(c *gc.C, applicationID, name string) (storedResource, resourceDoc) {
	content := name
	opened := resourcetesting.NewResource(c, nil, name, applicationID, content)
	res := opened.Resource

	stored := storedResource{
		Resource:    res,
		storagePath: "application-" + applicationID + "/resources/" + name,
	}

	doc := resourceDoc{
		DocID:         "resource#" + res.ID,
		ID:            res.ID,
		ApplicationID: res.ApplicationID,

		Name:        res.Name,
		Type:        res.Type.String(),
		Path:        res.Path,
		Description: res.Description,

		Origin:      res.Origin.String(),
		Revision:    res.Revision,
		Fingerprint: res.Fingerprint.Bytes(),
		Size:        res.Size,

		Username:  res.Username,
		Timestamp: res.Timestamp,

		StoragePath: stored.storagePath,
	}

	return stored, doc
}

func checkResources(c *gc.C, resources, expected resource.ApplicationResources) {
	resMap := make(map[string]resource.Resource)
	for _, res := range resources.Resources {
		resMap[res.Name] = res
	}
	expMap := make(map[string]resource.Resource)
	for _, res := range expected.Resources {
		expMap[res.Name] = res
	}
	c.Check(resMap, jc.DeepEquals, expMap)
}

func checkBasicResources(c *gc.C, resources, expected []resource.Resource) {
	resMap := make(map[string]resource.Resource)
	for _, res := range resources {
		resMap[res.ID] = res
	}
	expMap := make(map[string]resource.Resource)
	for _, res := range expected {
		expMap[res.ID] = res
	}
	c.Check(resMap, jc.DeepEquals, expMap)
}

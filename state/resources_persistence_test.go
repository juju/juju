// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/names"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/resource"
	"github.com/juju/juju/resource/resourcetesting"
	"github.com/juju/juju/state/statetest"
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
}

func (s *ResourcePersistenceSuite) TestListResourcesOkay(c *gc.C) {
	expected, docs := newPersistenceResources(c, "a-service", "spam", "eggs")
	expected.CharmStoreResources[1].Revision += 1
	docs[3].Revision += 1
	unitRes, doc := newPersistenceUnitResource(c, "a-service", "a-service/0", "something")
	expected.UnitResources = []resource.UnitResources{{
		Tag: names.NewUnitTag("a-service/0"),
		Resources: []resource.Resource{
			unitRes,
		},
	}}
	docs = append(docs, doc)
	s.base.ReturnAll = docs
	p := NewResourcePersistence(s.base)

	resources, err := p.ListResources("a-service")
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "All")
	s.stub.CheckCall(c, 0, "All",
		"resources",
		bson.D{{"service-id", "a-service"}},
		&docs,
	)
	c.Check(resources, jc.DeepEquals, expected)
}

func (s *ResourcePersistenceSuite) TestListResourcesNoResources(c *gc.C) {
	p := NewResourcePersistence(s.base)
	resources, err := p.ListResources("a-service")
	c.Assert(err, jc.ErrorIsNil)

	c.Check(resources.Resources, gc.HasLen, 0)
	s.stub.CheckCallNames(c, "All")
	s.stub.CheckCall(c, 0, "All",
		"resources",
		bson.D{{"service-id", "a-service"}},
		&[]resourceDoc{},
	)
}

func (s *ResourcePersistenceSuite) TestListResourcesIgnorePending(c *gc.C) {
	expected, docs := newPersistenceResources(c, "a-service", "spam", "eggs")
	expected.Resources = expected.Resources[:1]
	docs[2].PendingID = "some-unique-ID-001"
	s.base.ReturnAll = docs
	p := NewResourcePersistence(s.base)

	resources, err := p.ListResources("a-service")
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "All")
	s.stub.CheckCall(c, 0, "All",
		"resources",
		bson.D{{"service-id", "a-service"}},
		&docs,
	)
	checkResources(c, resources, expected)
}

func (s *ResourcePersistenceSuite) TestListResourcesBaseError(c *gc.C) {
	failure := errors.New("<failure>")
	s.stub.SetErrors(failure)

	p := NewResourcePersistence(s.base)
	_, err := p.ListResources("a-service")

	c.Check(errors.Cause(err), gc.Equals, failure)
	s.stub.CheckCallNames(c, "All")
	s.stub.CheckCall(c, 0, "All",
		"resources",
		bson.D{{"service-id", "a-service"}},
		&[]resourceDoc{},
	)
}

func (s *ResourcePersistenceSuite) TestListResourcesBadDoc(c *gc.C) {
	_, docs := newPersistenceResources(c, "a-service", "spam", "eggs")
	docs[0].Timestamp = time.Time{}
	s.base.ReturnAll = docs

	p := NewResourcePersistence(s.base)
	_, err := p.ListResources("a-service")

	c.Check(err, gc.ErrorMatches, `got invalid data from DB.*`)
	s.stub.CheckCallNames(c, "All")
	s.stub.CheckCall(c, 0, "All",
		"resources",
		bson.D{{"service-id", "a-service"}},
		&docs,
	)
}

func (s *ResourcePersistenceSuite) TestListPendingResourcesOkay(c *gc.C) {
	var expected []resource.Resource
	var docs []resourceDoc
	for _, name := range []string{"spam", "ham"} {
		res, doc := newPersistenceResource(c, "a-service", name)
		expected = append(expected, res.Resource)
		docs = append(docs, doc)
	}
	expected = expected[1:]
	expected[0].PendingID = "some-unique-ID-001"
	docs[1].PendingID = "some-unique-ID-001"
	s.base.ReturnAll = docs
	p := NewResourcePersistence(s.base)

	resources, err := p.ListPendingResources("a-service")
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "All")
	s.stub.CheckCall(c, 0, "All",
		"resources",
		bson.D{{"service-id", "a-service"}},
		&docs,
	)
	checkBasicResources(c, resources, expected)
}

func (s *ResourcePersistenceSuite) TestGetResourceOkay(c *gc.C) {
	expected, doc := newPersistenceResource(c, "a-service", "spam")
	unitDoc := doc // a copy
	unitDoc.ID = doc.ID + "#unit-a-service/0"
	unitDoc.UnitID = "a-service/0"
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

	res, storagePath, err := p.GetResource("a-service/spam")
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "One")
	s.stub.CheckCall(c, 0, "One", "resources", "resource#a-service/spam", &doc)
	c.Check(res, jc.DeepEquals, expected.Resource)
	c.Check(storagePath, gc.Equals, expected.storagePath)
}

func (s *ResourcePersistenceSuite) TestStageResourceOkay(c *gc.C) {
	res, doc := newPersistenceResource(c, "a-service", "spam")
	doc.DocID += "#staged"
	p := NewResourcePersistence(s.base)
	ignoredErr := errors.New("<never reached>")
	s.stub.SetErrors(nil, nil, ignoredErr)

	staged, err := p.StageResource(res.Resource, res.storagePath)
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "Run", "RunTransaction")
	s.stub.CheckCall(c, 1, "RunTransaction", []txn.Op{{
		C:      "resources",
		Id:     "resource#a-service/spam#staged",
		Assert: txn.DocMissing,
		Insert: &doc,
	}})
	c.Check(staged, jc.DeepEquals, &StagedResource{
		base:   s.base,
		id:     res.ID,
		stored: res,
	})
}

func (s *ResourcePersistenceSuite) TestStageResourceMissingStoragePath(c *gc.C) {
	res, _ := newPersistenceResource(c, "a-service", "spam")
	p := NewResourcePersistence(s.base)

	_, err := p.StageResource(res.Resource, "")

	s.stub.CheckNoCalls(c)
	c.Check(err, gc.ErrorMatches, `missing storage path`)
}

func (s *ResourcePersistenceSuite) TestStageResourceBadResource(c *gc.C) {
	res, _ := newPersistenceResource(c, "a-service", "spam")
	res.Resource.Timestamp = time.Time{}
	p := NewResourcePersistence(s.base)

	_, err := p.StageResource(res.Resource, res.storagePath)

	c.Check(err, jc.Satisfies, errors.IsNotValid)
	c.Check(err, gc.ErrorMatches, `bad resource.*`)

	s.stub.CheckNoCalls(c)
}

func (s *ResourcePersistenceSuite) TestSetResourceOkay(c *gc.C) {
	servicename := "a-service"
	res, doc := newPersistenceResource(c, servicename, "spam")
	s.base.ReturnOne = doc
	p := NewResourcePersistence(s.base)
	ignoredErr := errors.New("<never reached>")
	s.stub.SetErrors(nil, nil, nil, ignoredErr)

	err := p.SetResource(res.Resource)
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c,
		"One",
		"Run",
		"RunTransaction",
	)
	s.stub.CheckCall(c, 2, "RunTransaction", []txn.Op{{
		C:      "resources",
		Id:     "resource#a-service/spam",
		Assert: txn.DocMissing,
		Insert: &doc,
	}})
}

func (s *ResourcePersistenceSuite) TestSetResourceNotFound(c *gc.C) {
	servicename := "a-service"
	res, doc := newPersistenceResource(c, servicename, "spam")
	s.base.ReturnOne = doc
	expected := doc // a copy
	expected.StoragePath = ""
	p := NewResourcePersistence(s.base)
	notFound := errors.NewNotFound(nil, "")
	ignoredErr := errors.New("<never reached>")
	s.stub.SetErrors(notFound, nil, nil, ignoredErr)

	err := p.SetResource(res.Resource)
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c,
		"One",
		"Run",
		"RunTransaction",
	)
	s.stub.CheckCall(c, 2, "RunTransaction", []txn.Op{{
		C:      "resources",
		Id:     "resource#a-service/spam",
		Assert: txn.DocMissing,
		Insert: &expected,
	}})
}

func (s *ResourcePersistenceSuite) TestSetCharmStoreResourceOkay(c *gc.C) {
	lastPolled := time.Now().UTC()
	servicename := "a-service"
	res, doc := newPersistenceResource(c, servicename, "spam")
	expected := doc // a copy
	expected.DocID += "#charmstore"
	expected.Username = ""
	expected.Timestamp = time.Time{}
	expected.StoragePath = ""
	expected.LastPolled = lastPolled
	p := NewResourcePersistence(s.base)
	ignoredErr := errors.New("<never reached>")
	s.stub.SetErrors(nil, nil, nil, ignoredErr)

	err := p.SetCharmStoreResource(res.ID, res.ServiceID, res.Resource.Resource, lastPolled)
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c,
		"Run",
		"RunTransaction",
	)
	s.stub.CheckCall(c, 1, "RunTransaction", []txn.Op{{
		C:      "resources",
		Id:     "resource#a-service/spam#charmstore",
		Assert: txn.DocMissing,
		Insert: &expected,
	}})
}

func (s *ResourcePersistenceSuite) TestSetUnitResourceOkay(c *gc.C) {
	servicename := "a-service"
	unitname := "a-service/0"
	res, doc := newPersistenceUnitResource(c, servicename, unitname, "eggs")
	s.base.ReturnOne = doc
	p := NewResourcePersistence(s.base)
	ignoredErr := errors.New("<never reached>")
	s.stub.SetErrors(nil, nil, nil, ignoredErr)

	err := p.SetUnitResource("a-service/0", res)
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "One", "Run", "RunTransaction")
	s.stub.CheckCall(c, 2, "RunTransaction", []txn.Op{{
		C:      "resources",
		Id:     "resource#a-service/eggs#unit-a-service/0",
		Assert: txn.DocMissing,
		Insert: &doc,
	}})
}

func (s *ResourcePersistenceSuite) TestSetUnitResourceNotFound(c *gc.C) {
	servicename := "a-service"
	unitname := "a-service/0"
	res, _ := newPersistenceUnitResource(c, servicename, unitname, "eggs")
	p := NewResourcePersistence(s.base)
	notFound := errors.NewNotFound(nil, "")
	s.stub.SetErrors(notFound)

	err := p.SetUnitResource("a-service/0", res)

	s.stub.CheckCallNames(c, "One")
	c.Check(err, jc.Satisfies, errors.IsNotFound)
	c.Check(err, gc.ErrorMatches, `resource "eggs" not found`)
}

func (s *ResourcePersistenceSuite) TestSetUnitResourceExists(c *gc.C) {
	res, doc := newPersistenceUnitResource(c, "a-service", "a-service/0", "spam")
	s.base.ReturnOne = doc
	p := NewResourcePersistence(s.base)
	ignoredErr := errors.New("<never reached>")
	s.stub.SetErrors(nil, nil, txn.ErrAborted, nil, ignoredErr)

	err := p.SetUnitResource("a-service/0", res)
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "One", "Run", "RunTransaction", "RunTransaction")
	s.stub.CheckCall(c, 2, "RunTransaction", []txn.Op{{
		C:      "resources",
		Id:     "resource#a-service/spam#unit-a-service/0",
		Assert: txn.DocMissing,
		Insert: &doc,
	}})
	s.stub.CheckCall(c, 3, "RunTransaction", []txn.Op{{
		C:      "resources",
		Id:     "resource#a-service/spam#unit-a-service/0",
		Assert: txn.DocExists,
		Remove: true,
	}, {
		C:      "resources",
		Id:     "resource#a-service/spam#unit-a-service/0",
		Assert: txn.DocMissing,
		Insert: &doc,
	}})
}

func (s *ResourcePersistenceSuite) TestSetUnitResourceBadResource(c *gc.C) {
	res, doc := newPersistenceUnitResource(c, "a-service", "a-service/0", "spam")
	s.base.ReturnOne = doc
	res.Timestamp = time.Time{}
	p := NewResourcePersistence(s.base)

	err := p.SetUnitResource("a-service/0", res)

	c.Check(err, jc.Satisfies, errors.IsNotValid)
	c.Check(err, gc.ErrorMatches, `bad resource.*`)

	s.stub.CheckCallNames(c, "One")
}

func (s *ResourcePersistenceSuite) TestNewResourcePendingResourceOpsExists(c *gc.C) {
	pendingID := "some-unique-ID-001"
	stored, expected := newPersistenceResource(c, "a-service", "spam")
	stored.PendingID = pendingID
	doc := expected // a copy
	doc.DocID = pendingResourceID(stored.ID, pendingID)
	doc.PendingID = pendingID
	s.base.ReturnOne = doc
	p := NewResourcePersistence(s.base)

	ops, err := p.NewResolvePendingResourceOps(stored.ID, stored.PendingID)
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "One", "One")
	s.stub.CheckCall(c, 0, "One", "resources", "resource#a-service/spam#pending-some-unique-ID-001", &doc)
	c.Check(ops, jc.DeepEquals, []txn.Op{{
		C:      "resources",
		Id:     doc.DocID,
		Assert: txn.DocExists,
		Remove: true,
	}, {
		C:      "resources",
		Id:     expected.DocID,
		Assert: txn.DocExists,
		Remove: true,
	}, {
		C:      "resources",
		Id:     expected.DocID,
		Assert: txn.DocMissing,
		Insert: &expected,
	}})
}

func (s *ResourcePersistenceSuite) TestNewResourcePendingResourceOpsNotFound(c *gc.C) {
	pendingID := "some-unique-ID-001"
	stored, expected := newPersistenceResource(c, "a-service", "spam")
	stored.PendingID = pendingID
	doc := expected // a copy
	doc.DocID = pendingResourceID(stored.ID, pendingID)
	doc.PendingID = pendingID
	s.base.ReturnOne = doc
	notFound := errors.NewNotFound(nil, "")
	s.stub.SetErrors(nil, notFound)
	p := NewResourcePersistence(s.base)

	ops, err := p.NewResolvePendingResourceOps(stored.ID, stored.PendingID)
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "One", "One")
	s.stub.CheckCall(c, 0, "One", "resources", "resource#a-service/spam#pending-some-unique-ID-001", &doc)
	c.Check(ops, jc.DeepEquals, []txn.Op{{
		C:      "resources",
		Id:     doc.DocID,
		Assert: txn.DocExists,
		Remove: true,
	}, {
		C:      "resources",
		Id:     expected.DocID,
		Assert: txn.DocMissing,
		Insert: &expected,
	}})
}

func newPersistenceResources(c *gc.C, serviceID string, names ...string) (resource.ServiceResources, []resourceDoc) {
	var svcResources resource.ServiceResources
	var docs []resourceDoc
	for _, name := range names {
		res, doc := newPersistenceResource(c, serviceID, name)
		svcResources.Resources = append(svcResources.Resources, res.Resource)
		svcResources.CharmStoreResources = append(svcResources.CharmStoreResources, res.Resource.Resource)
		docs = append(docs, doc)
		csDoc := doc // a copy
		csDoc.DocID += "#charmstore"
		csDoc.Username = ""
		csDoc.Timestamp = time.Time{}
		csDoc.StoragePath = ""
		csDoc.LastPolled = time.Now().UTC()
		docs = append(docs, csDoc)
	}
	return svcResources, docs
}

func newPersistenceUnitResource(c *gc.C, serviceID, unitID, name string) (resource.Resource, resourceDoc) {
	res, doc := newPersistenceResource(c, serviceID, name)
	doc.DocID += "#unit-" + unitID
	doc.UnitID = unitID
	return res.Resource, doc
}

func newPersistenceResource(c *gc.C, serviceID, name string) (storedResource, resourceDoc) {
	content := name
	opened := resourcetesting.NewResource(c, nil, name, serviceID, content)
	res := opened.Resource

	stored := storedResource{
		Resource:    res,
		storagePath: "service-" + serviceID + "/resources/" + name,
	}

	doc := resourceDoc{
		DocID:     "resource#" + res.ID,
		ID:        res.ID,
		ServiceID: res.ServiceID,

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

func checkResources(c *gc.C, resources, expected resource.ServiceResources) {
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

// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package persistence

import (
	"strings"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	charmresource "gopkg.in/juju/charm.v6-unstable/resource"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/resource"
)

var _ = gc.Suite(&PersistenceSuite{})

type PersistenceSuite struct {
	testing.IsolationSuite

	stub *testing.Stub
	base *stubStatePersistence
}

func (s *PersistenceSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.stub = &testing.Stub{}
	s.base = &stubStatePersistence{
		stub: s.stub,
	}
}

func (s *PersistenceSuite) TestListResourcesOkay(c *gc.C) {
	expected, docs := newResources(c, "a-service", "spam", "eggs")
	unitRes, doc := newUnitResource(c, "a-service", "foo/0", "something")
	expected.UnitResources = []resource.UnitResources{{
		Tag: names.NewUnitTag("foo/0"),
		Resources: []resource.Resource{
			unitRes,
		},
	}}
	docs = append(docs, doc)
	s.base.docs = docs

	p := NewPersistence(s.base)
	resources, err := p.ListResources("a-service")
	c.Assert(err, jc.ErrorIsNil)

	checkResources(c, resources, expected)
	s.stub.CheckCallNames(c, "All")
	s.stub.CheckCall(c, 0, "All",
		"resources",
		bson.D{{"service-id", "a-service"}},
		&docs,
	)
}

func (s *PersistenceSuite) TestListResourcesNoResources(c *gc.C) {
	p := NewPersistence(s.base)
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

func (s *PersistenceSuite) TestListResourcesBaseError(c *gc.C) {
	failure := errors.New("<failure>")
	s.stub.SetErrors(failure)

	p := NewPersistence(s.base)
	_, err := p.ListResources("a-service")

	c.Check(errors.Cause(err), gc.Equals, failure)
	s.stub.CheckCallNames(c, "All")
	s.stub.CheckCall(c, 0, "All",
		"resources",
		bson.D{{"service-id", "a-service"}},
		&[]resourceDoc{},
	)
}

func (s *PersistenceSuite) TestListResourcesBadDoc(c *gc.C) {
	_, docs := newResources(c, "a-service", "spam", "eggs")
	docs[0].Timestamp = time.Time{}
	s.base.docs = docs

	p := NewPersistence(s.base)
	_, err := p.ListResources("a-service")

	c.Check(err, gc.ErrorMatches, `got invalid data from DB.*`)
	s.stub.CheckCallNames(c, "All")
	s.stub.CheckCall(c, 0, "All",
		"resources",
		bson.D{{"service-id", "a-service"}},
		&docs,
	)
}

func (s *PersistenceSuite) TestStageResourceOkay(c *gc.C) {
	res, doc := newResource(c, "a-service", "spam")
	doc.DocID += "#staged"
	p := NewPersistence(s.base)
	ignoredErr := errors.New("<never reached>")
	s.stub.SetErrors(nil, nil, ignoredErr)
	args := ModelResource{
		ID:        res.Name,
		ServiceID: "a-service",
		Resource:  res,
	}

	err := p.StageResource(args)
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "Run", "RunTransaction")
	s.stub.CheckCall(c, 1, "RunTransaction", []txn.Op{{
		C:      "resources",
		Id:     "resource#a-service#spam#staged",
		Assert: txn.DocMissing,
		Insert: &doc,
	}})
}

func (s *PersistenceSuite) TestStageResourceExists(c *gc.C) {
	res, doc := newResource(c, "a-service", "spam")
	doc.DocID += "#staged"
	p := NewPersistence(s.base)
	ignoredErr := errors.New("<never reached>")
	s.stub.SetErrors(nil, txn.ErrAborted, nil, ignoredErr)
	args := ModelResource{
		ID:        res.Name,
		ServiceID: "a-service",
		Resource:  res,
	}

	err := p.StageResource(args)
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "Run", "RunTransaction", "RunTransaction")
	s.stub.CheckCall(c, 1, "RunTransaction", []txn.Op{{
		C:      "resources",
		Id:     "resource#a-service#spam#staged",
		Assert: txn.DocMissing,
		Insert: &doc,
	}})
	s.stub.CheckCall(c, 2, "RunTransaction", []txn.Op{{
		C:      "resources",
		Id:     "resource#a-service#spam#staged",
		Assert: &doc,
	}})
}

func (s *PersistenceSuite) TestStageResourceBadResource(c *gc.C) {
	res, _ := newResource(c, "a-service", "spam")
	res.Timestamp = time.Time{}
	p := NewPersistence(s.base)
	args := ModelResource{
		ID:        res.Name,
		ServiceID: "a-service",
		Resource:  res,
	}

	err := p.StageResource(args)

	c.Check(err, jc.Satisfies, errors.IsNotValid)
	c.Check(err, gc.ErrorMatches, `bad resource.*`)

	s.stub.CheckNoCalls(c)
}

func (s *PersistenceSuite) TestUnstageResourceOkay(c *gc.C) {
	res, _ := newResource(c, "a-service", "spam")
	p := NewPersistence(s.base)
	ignoredErr := errors.New("<never reached>")
	s.stub.SetErrors(nil, nil, ignoredErr)

	err := p.UnstageResource(res.Name, "a-service")
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "Run", "RunTransaction")
	s.stub.CheckCall(c, 1, "RunTransaction", []txn.Op{{
		C:      "resources",
		Id:     "resource#a-service#spam#staged",
		Remove: true,
	}})
}

func (s *PersistenceSuite) TestSetResourceOkay(c *gc.C) {
	res, doc := newResource(c, "a-service", "spam")
	p := NewPersistence(s.base)
	ignoredErr := errors.New("<never reached>")
	s.stub.SetErrors(nil, nil, ignoredErr)
	args := ModelResource{
		ID:        res.Name,
		ServiceID: "a-service",
		Resource:  res,
	}

	err := p.SetResource(args)
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "Run", "RunTransaction")
	s.stub.CheckCall(c, 1, "RunTransaction", []txn.Op{{
		C:      "resources",
		Id:     "resource#a-service#spam",
		Assert: txn.DocMissing,
		Insert: &doc,
	}, {
		C:      "resources",
		Id:     "resource#a-service#spam#staged",
		Remove: true,
	}})
}

func (s *PersistenceSuite) TestSetUnitResourceOkay(c *gc.C) {
	servicename := "a-service"
	unitname := "foo/0"
	res, doc := newUnitResource(c, servicename, unitname, "eggs")
	p := NewPersistence(s.base)
	ignoredErr := errors.New("<never reached>")
	s.stub.SetErrors(nil, nil, ignoredErr)

	unit := fakeUnit{unitname, servicename}

	err := p.SetUnitResource(res.Name, unit, res)
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "Run", "RunTransaction")
	s.stub.CheckCall(c, 1, "RunTransaction", []txn.Op{{
		C:      "resources",
		Id:     "resource#foo/0#eggs",
		Assert: txn.DocMissing,
		Insert: &doc,
	}})
}

func (s *PersistenceSuite) TestSetResourceExists(c *gc.C) {
	res, doc := newResource(c, "a-service", "spam")
	p := NewPersistence(s.base)
	ignoredErr := errors.New("<never reached>")
	s.stub.SetErrors(nil, txn.ErrAborted, nil, ignoredErr)
	args := ModelResource{
		ID:        res.Name,
		ServiceID: "a-service",
		Resource:  res,
	}

	err := p.SetResource(args)
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "Run", "RunTransaction", "RunTransaction")
	s.stub.CheckCall(c, 1, "RunTransaction", []txn.Op{{
		C:      "resources",
		Id:     "resource#a-service#spam",
		Assert: txn.DocMissing,
		Insert: &doc,
	}, {
		C:      "resources",
		Id:     "resource#a-service#spam#staged",
		Remove: true,
	}})
	s.stub.CheckCall(c, 2, "RunTransaction", []txn.Op{{
		C:      "resources",
		Id:     "resource#a-service#spam",
		Assert: txn.DocExists,
		Remove: true,
	}, {
		C:      "resources",
		Id:     "resource#a-service#spam",
		Assert: txn.DocMissing,
		Insert: &doc,
	}, {
		C:      "resources",
		Id:     "resource#a-service#spam#staged",
		Remove: true,
	}})
}

func (s *PersistenceSuite) TestSetUnitResourceExists(c *gc.C) {
	res, doc := newUnitResource(c, "a-service", "foo/0", "spam")
	p := NewPersistence(s.base)
	ignoredErr := errors.New("<never reached>")
	s.stub.SetErrors(nil, txn.ErrAborted, nil, ignoredErr)

	unit := fakeUnit{"foo/0", "a-service"}

	err := p.SetUnitResource(res.Name, unit, res)
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "Run", "RunTransaction", "RunTransaction")
	s.stub.CheckCall(c, 1, "RunTransaction", []txn.Op{{
		C:      "resources",
		Id:     "resource#foo/0#spam",
		Assert: txn.DocMissing,
		Insert: &doc,
	}})
	s.stub.CheckCall(c, 2, "RunTransaction", []txn.Op{{
		C:      "resources",
		Id:     "resource#foo/0#spam",
		Assert: txn.DocExists,
		Remove: true,
	}, {
		C:      "resources",
		Id:     "resource#foo/0#spam",
		Assert: txn.DocMissing,
		Insert: &doc,
	}})
}

func (s *PersistenceSuite) TestSetResourceBadResource(c *gc.C) {
	res, _ := newResource(c, "a-service", "spam")
	res.Timestamp = time.Time{}
	p := NewPersistence(s.base)
	args := ModelResource{
		ID:        res.Name,
		ServiceID: "a-service",
		Resource:  res,
	}

	err := p.SetResource(args)

	c.Check(err, jc.Satisfies, errors.IsNotValid)
	c.Check(err, gc.ErrorMatches, `bad resource.*`)

	s.stub.CheckNoCalls(c)
}

func (s *PersistenceSuite) TestSetUnitResourceBadResource(c *gc.C) {
	res, _ := newUnitResource(c, "a-service", "foo/0", "spam")
	res.Timestamp = time.Time{}
	p := NewPersistence(s.base)

	unit := fakeUnit{"foo/0", "a-service"}
	err := p.SetUnitResource(res.Name, unit, res)

	c.Check(err, jc.Satisfies, errors.IsNotValid)
	c.Check(err, gc.ErrorMatches, `bad resource.*`)

	s.stub.CheckNoCalls(c)
}
func newResources(c *gc.C, serviceID string, names ...string) (resource.ServiceResources, []resourceDoc) {
	var resources []resource.Resource
	var docs []resourceDoc
	for _, name := range names {
		res, doc := newResource(c, serviceID, name)
		resources = append(resources, res)
		docs = append(docs, doc)
	}
	return resource.ServiceResources{Resources: resources}, docs
}

func newUnitResource(c *gc.C, serviceID, unitID, name string) (resource.Resource, resourceDoc) {
	res, doc := newResource(c, serviceID, name)
	doc.DocID = "resource#" + unitID + "#" + name
	doc.UnitID = unitID
	return res, doc
}

func newResource(c *gc.C, serviceID, name string) (resource.Resource, resourceDoc) {
	reader := strings.NewReader(name)
	fp, err := charmresource.GenerateFingerprint(reader)
	c.Assert(err, jc.ErrorIsNil)

	res := resource.Resource{
		Resource: charmresource.Resource{
			Meta: charmresource.Meta{
				Name:    name,
				Type:    charmresource.TypeFile,
				Path:    name + ".tgz",
				Comment: "you need it",
			},
			Origin:      charmresource.OriginUpload,
			Revision:    1,
			Fingerprint: fp,
		},
		Username:  "a-user",
		Timestamp: time.Now().UTC(),
	}

	doc := resourceDoc{
		DocID:     "resource#" + serviceID + "#" + name,
		ServiceID: serviceID,

		Name:    res.Name,
		Type:    res.Type.String(),
		Path:    res.Path,
		Comment: res.Comment,

		Origin:      res.Origin.String(),
		Revision:    res.Revision,
		Fingerprint: res.Fingerprint.Bytes(),

		Username:  res.Username,
		Timestamp: res.Timestamp,
	}

	return res, doc
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

type fakeUnit struct {
	unit    string
	service string
}

func (f fakeUnit) Name() string {
	return f.unit
}

func (f fakeUnit) ServiceName() string {
	return f.service
}

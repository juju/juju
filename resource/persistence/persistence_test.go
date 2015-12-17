// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package persistence

import (
	"time"

	"github.com/juju/errors"
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

	c.Check(resources, gc.HasLen, 0)
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

func (s *PersistenceSuite) TestSetStagedResourceOkay(c *gc.C) {
	res, doc := newResource(c, "a-service", "spam")
	doc.DocID += "#staged"
	p := NewPersistence(s.base)
	ignoredErr := errors.New("<never reached>")
	s.stub.SetErrors(nil, nil, ignoredErr)

	err := p.SetStagedResource(res.Name, "a-service", res)
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "Run", "RunTransaction")
	s.stub.CheckCall(c, 1, "RunTransaction", []txn.Op{{
		C:      "resources",
		Id:     "resource#a-service#spam#staged",
		Assert: txn.DocMissing,
		Insert: &doc,
	}})
}

func (s *PersistenceSuite) TestSetStagedResourceExists(c *gc.C) {
	res, doc := newResource(c, "a-service", "spam")
	doc.DocID += "#staged"
	p := NewPersistence(s.base)
	ignoredErr := errors.New("<never reached>")
	s.stub.SetErrors(nil, txn.ErrAborted, nil, ignoredErr)

	err := p.SetStagedResource(res.Name, "a-service", res)
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

func (s *PersistenceSuite) TestSetStagedResourceBadResource(c *gc.C) {
	res, _ := newResource(c, "a-service", "spam")
	res.Timestamp = time.Time{}
	p := NewPersistence(s.base)

	err := p.SetStagedResource(res.Name, "a-service", res)

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

	err := p.SetResource(res.Name, "a-service", res)
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

func (s *PersistenceSuite) TestSetResourceExists(c *gc.C) {
	res, doc := newResource(c, "a-service", "spam")
	p := NewPersistence(s.base)
	ignoredErr := errors.New("<never reached>")
	s.stub.SetErrors(nil, txn.ErrAborted, nil, ignoredErr)

	err := p.SetResource(res.Name, "a-service", res)
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
		Update: &doc,
	}, {
		C:      "resources",
		Id:     "resource#a-service#spam#staged",
		Remove: true,
	}})
}

func (s *PersistenceSuite) TestSetResourceBadResource(c *gc.C) {
	res, _ := newResource(c, "a-service", "spam")
	res.Timestamp = time.Time{}
	p := NewPersistence(s.base)

	err := p.SetResource(res.Name, "a-service", res)

	c.Check(err, jc.Satisfies, errors.IsNotValid)
	c.Check(err, gc.ErrorMatches, `bad resource.*`)

	s.stub.CheckNoCalls(c)
}

func newResources(c *gc.C, serviceID string, names ...string) ([]resource.Resource, []resourceDoc) {
	var resources []resource.Resource
	var docs []resourceDoc
	for _, name := range names {
		res, doc := newResource(c, serviceID, name)
		resources = append(resources, res)
		docs = append(docs, doc)
	}
	return resources, docs
}

func newResource(c *gc.C, serviceID, name string) (resource.Resource, resourceDoc) {
	fp, err := charmresource.GenerateFingerprint([]byte(name))
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
		Timestamp: time.Now(),
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

func checkResources(c *gc.C, resources, expected []resource.Resource) {
	resMap := make(map[string]resource.Resource)
	for _, res := range resources {
		resMap[res.Name] = res
	}
	expMap := make(map[string]resource.Resource)
	for _, res := range expected {
		expMap[res.Name] = res
	}
	c.Check(resMap, jc.DeepEquals, expMap)
}

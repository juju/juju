// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"sort"

	"github.com/juju/errors"
	"github.com/juju/mgo/v3"
	"github.com/juju/mgo/v3/bson"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v3"
	"github.com/kr/pretty"
	gc "gopkg.in/check.v1"

	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/storage/provider"
	coretesting "github.com/juju/juju/testing"
)

type upgradesSuite struct {
	internalStateSuite
}

var _ = gc.Suite(&upgradesSuite{})

type expectUpgradedData struct {
	coll     *mgo.Collection
	expected []bson.M
	filter   bson.D
}

func upgradedData(coll *mgo.Collection, expected []bson.M) expectUpgradedData {
	return expectUpgradedData{
		coll:     coll,
		expected: expected,
	}
}

func (s *upgradesSuite) assertUpgradedData(c *gc.C, upgrade func(*StatePool) error, expect ...expectUpgradedData) {
	// Two rounds to check idempotency.
	for i := 0; i < 2; i++ {
		c.Logf("Run: %d", i)
		err := upgrade(s.pool)
		c.Assert(err, jc.ErrorIsNil)

		for _, expect := range expect {
			var docs []bson.M
			err = expect.coll.Find(expect.filter).Sort("_id").All(&docs)
			c.Assert(err, jc.ErrorIsNil)
			for i, d := range docs {
				doc := d
				delete(doc, "txn-queue")
				delete(doc, "txn-revno")
				delete(doc, "version")
				docs[i] = doc
			}
			c.Assert(docs, jc.DeepEquals, expect.expected,
				gc.Commentf("differences: %s", pretty.Diff(docs, expect.expected)))
		}
	}
}

func (s *upgradesSuite) makeModel(c *gc.C, name string, attr coretesting.Attrs) *State {
	uuid := utils.MustNewUUID()
	cfg := coretesting.CustomModelConfig(c, coretesting.Attrs{
		"name": name,
		"uuid": uuid.String(),
	}.Merge(attr))
	m, err := s.state.Model()
	c.Assert(err, jc.ErrorIsNil)
	_, st, err := s.controller.NewModel(ModelArgs{
		Type:                    ModelTypeIAAS,
		CloudName:               "dummy",
		CloudRegion:             "dummy-region",
		Config:                  cfg,
		Owner:                   m.Owner(),
		StorageProviderRegistry: provider.CommonStorageProviders(),
	})
	c.Assert(err, jc.ErrorIsNil)
	return st
}

type bsonMById []bson.M

func (x bsonMById) Len() int { return len(x) }

func (x bsonMById) Swap(i, j int) { x[i], x[j] = x[j], x[i] }

func (x bsonMById) Less(i, j int) bool {
	return x[i]["_id"].(string) < x[j]["_id"].(string)
}

func (s *upgradesSuite) TestRemoveOrphanedSecretPermissions(c *gc.C) {
	model1 := s.makeModel(c, "model-1", coretesting.Attrs{})
	model2 := s.makeModel(c, "model-2", coretesting.Attrs{})
	defer func() {
		_ = model1.Close()
		_ = model2.Close()
	}()

	uuid1 := model1.ModelUUID()
	uuid2 := model2.ModelUUID()

	permissionsColl, closer := s.state.db().GetRawCollection(secretPermissionsC)
	defer closer()

	appsColl, closer := s.state.db().GetRawCollection(applicationsC)
	defer closer()

	unitsColl, closer := s.state.db().GetRawCollection(unitsC)
	defer closer()

	var err error
	err = appsColl.Insert(bson.M{
		"_id":        ensureModelUUID(uuid1, "app1"),
		"name":       "app1",
		"model-uuid": uuid1,
	})
	c.Assert(err, jc.ErrorIsNil)
	err = appsColl.Insert(bson.M{
		"_id":        ensureModelUUID(uuid2, "app2"),
		"name":       "app2",
		"model-uuid": uuid2,
	})
	c.Assert(err, jc.ErrorIsNil)

	err = unitsColl.Insert(bson.M{
		"_id":        ensureModelUUID(uuid1, "unit/1"),
		"name":       "unit/1",
		"model-uuid": uuid1,
	})
	c.Assert(err, jc.ErrorIsNil)
	err = unitsColl.Insert(bson.M{
		"_id":        ensureModelUUID(uuid2, "unit/2"),
		"name":       "unit/2",
		"model-uuid": uuid2,
	})
	c.Assert(err, jc.ErrorIsNil)

	secretID := "4fdg37dgag3jdjej49sj"
	err = permissionsColl.Insert(bson.M{
		"model-uuid":  uuid1,
		"_id":         ensureModelUUID(uuid1, secretID+"#application-app1"),
		"subject-tag": "application-app1",
		"scope-tag":   "relation-blah",
		"role":        "view",
	})
	c.Assert(err, jc.ErrorIsNil)
	err = permissionsColl.Insert(bson.M{
		"model-uuid":  uuid1,
		"_id":         ensureModelUUID(uuid1, secretID+"#application-appbad1"),
		"subject-tag": "application-appbad1",
		"scope-tag":   "relation-blah",
		"role":        "view",
	})
	c.Assert(err, jc.ErrorIsNil)
	err = permissionsColl.Insert(bson.M{
		"model-uuid":  uuid1,
		"_id":         ensureModelUUID(uuid1, secretID+"#unit-unit-1"),
		"subject-tag": "unit-unit-1",
		"scope-tag":   "relation-blah",
		"role":        "view",
	})
	c.Assert(err, jc.ErrorIsNil)
	err = permissionsColl.Insert(bson.M{
		"model-uuid":  uuid1,
		"_id":         ensureModelUUID(uuid1, secretID+"#unit-unitbad-1"),
		"subject-tag": "unit-unitbad-1",
		"scope-tag":   "relation-blah",
		"role":        "view",
	})
	c.Assert(err, jc.ErrorIsNil)

	err = permissionsColl.Insert(bson.M{
		"model-uuid":  uuid2,
		"_id":         ensureModelUUID(uuid2, secretID+"#application-app2"),
		"subject-tag": "application-app2",
		"scope-tag":   "relation-blah",
		"role":        "view",
	})
	c.Assert(err, jc.ErrorIsNil)
	err = permissionsColl.Insert(bson.M{
		"model-uuid":  uuid2,
		"_id":         ensureModelUUID(uuid2, secretID+"#application-appbad2"),
		"subject-tag": "application-appbad2",
		"scope-tag":   "relation-blah",
		"role":        "view",
	})
	c.Assert(err, jc.ErrorIsNil)
	err = permissionsColl.Insert(bson.M{
		"model-uuid":  uuid2,
		"_id":         ensureModelUUID(uuid2, secretID+"#unit-unit-2"),
		"subject-tag": "unit-unit-2",
		"scope-tag":   "relation-blah",
		"role":        "view",
	})
	c.Assert(err, jc.ErrorIsNil)
	err = permissionsColl.Insert(bson.M{
		"model-uuid":  uuid2,
		"_id":         ensureModelUUID(uuid2, secretID+"#unit-unitbad-2"),
		"subject-tag": "unit-unitbad-2",
		"scope-tag":   "relation-blah",
		"role":        "view",
	})
	c.Assert(err, jc.ErrorIsNil)

	expected := bsonMById{
		{
			"_id":         ensureModelUUID(uuid1, secretID+"#application-app1"),
			"model-uuid":  uuid1,
			"subject-tag": "application-app1",
			"scope-tag":   "relation-blah",
			"role":        "view",
		},
		{
			"_id":         ensureModelUUID(uuid1, secretID+"#unit-unit-1"),
			"model-uuid":  uuid1,
			"subject-tag": "unit-unit-1",
			"scope-tag":   "relation-blah",
			"role":        "view",
		},
		{
			"_id":         ensureModelUUID(uuid2, secretID+"#application-app2"),
			"model-uuid":  uuid2,
			"subject-tag": "application-app2",
			"scope-tag":   "relation-blah",
			"role":        "view",
		},
		{
			"_id":         ensureModelUUID(uuid2, secretID+"#unit-unit-2"),
			"model-uuid":  uuid2,
			"subject-tag": "unit-unit-2",
			"scope-tag":   "relation-blah",
			"role":        "view",
		},
	}

	sort.Sort(expected)
	s.assertUpgradedData(c, RemoveOrphanedSecretPermissions,
		upgradedData(permissionsColl, expected),
	)
}

func (s *upgradesSuite) TestMigrateApplicationOpenedPortsToUnitScope(c *gc.C) {
	model := s.makeModel(c, "model-1", coretesting.Attrs{})
	defer func() {
		_ = model.Close()
	}()

	modelUUID := model.ModelUUID()

	openedPorts, closer := s.state.db().GetRawCollection(openedPortsC)
	defer closer()

	appsColl, closer := s.state.db().GetRawCollection(applicationsC)
	defer closer()

	unitsColl, closer := s.state.db().GetRawCollection(unitsC)
	defer closer()

	var err error
	err = appsColl.Insert(bson.M{
		"_id":        ensureModelUUID(modelUUID, "app1"),
		"name":       "app1",
		"model-uuid": modelUUID,
		"life":       Alive,
	})
	c.Assert(err, jc.ErrorIsNil)

	err = unitsColl.Insert(bson.M{
		"_id":         ensureModelUUID(modelUUID, "unit/0"),
		"name":        "unit/0",
		"model-uuid":  modelUUID,
		"application": "app1",
		"life":        Alive,
	})
	c.Assert(err, jc.ErrorIsNil)

	pg := bson.M{
		"": []interface{}{
			bson.M{
				"fromport": 3000,
				"toport":   3000,
				"protocol": "tcp",
			},
			bson.M{
				"fromport": 3001,
				"toport":   3001,
				"protocol": "tcp",
			},
		},
	}
	err = openedPorts.Insert(bson.M{
		"_id":              ensureModelUUID(modelUUID, "app1"),
		"model-uuid":       modelUUID,
		"application-name": "app1",
		"port-ranges":      pg,
	})
	c.Assert(err, jc.ErrorIsNil)

	expected := bsonMById{
		{
			"_id":              ensureModelUUID(modelUUID, "app1"),
			"model-uuid":       modelUUID,
			"application-name": "app1",
			"port-ranges":      bson.M{},
			"unit-port-ranges": bson.M{
				"unit/0": pg,
			},
		},
	}
	sort.Sort(expected)
	s.assertUpgradedData(c, MigrateApplicationOpenedPortsToUnitScope,
		upgradedData(openedPorts, expected),
	)
}

func (s *upgradesSuite) TestEnsureInitalRefCountForExternalSecretBackends(c *gc.C) {
	backendStore := NewSecretBackends(s.state)
	_, err := backendStore.CreateSecretBackend(CreateSecretBackendParams{
		ID:          "backend-id-1",
		Name:        "foo",
		BackendType: "vault",
	})
	c.Assert(err, jc.ErrorIsNil)
	backendRefCount, err := s.state.ReadBackendRefCount("backend-id-1")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(backendRefCount, gc.Equals, 0)

	_, err = backendStore.CreateSecretBackend(CreateSecretBackendParams{
		ID:          "backend-id-2",
		Name:        "bar",
		BackendType: "vault",
	})
	c.Assert(err, jc.ErrorIsNil)
	ops, err := s.state.incBackendRevisionCountOps("backend-id-2", 3)
	c.Assert(err, jc.ErrorIsNil)
	err = s.state.db().RunTransaction(ops)
	c.Assert(err, jc.ErrorIsNil)
	backendRefCount, err = s.state.ReadBackendRefCount("backend-id-2")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(backendRefCount, gc.Equals, 3)

	ops, err = s.state.removeBackendRefCountOp("backend-id-1", true)
	c.Assert(err, jc.ErrorIsNil)
	err = s.state.db().RunTransaction(ops)
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.state.ReadBackendRefCount("backend-id-1")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)

	expected := bsonMById{
		{
			// created by EnsureInitalRefCountForExternalSecretBackends
			"_id":      secretBackendRefCountKey("backend-id-1"),
			"refcount": 0,
		},
		{
			// no touch existing records.
			"_id":      secretBackendRefCountKey("backend-id-2"),
			"refcount": 3,
		},
	}
	sort.Sort(expected)

	refCountCollection, closer := s.state.db().GetRawCollection(globalRefcountsC)
	defer closer()

	expectedData := upgradedData(refCountCollection, expected)
	expectedData.filter = bson.D{{"_id", bson.M{"$regex": "secretbackend#revisions#.*"}}}
	s.assertUpgradedData(c, EnsureInitalRefCountForExternalSecretBackends, expectedData)
}

func (s *upgradesSuite) TestEnsureApplicationCharmOriginsNormaliseLocal(c *gc.C) {
	ch := AddTestingCharm(c, s.state, "dummy")
	platform := Platform{OS: "ubuntu", Channel: "20.04"}
	rev := 6
	_ = addTestingApplication(c, addTestingApplicationParams{
		st:   s.state,
		name: "my-local-app",
		ch:   ch,
		origin: &CharmOrigin{
			Source:   corecharm.CharmHub.String(),
			Platform: &platform,
			Hash:     "some-hash",
			ID:       "some-id",
			Revision: &rev,
			Channel:  &Channel{Track: "20.04", Risk: "stable", Branch: "deadbeef"},
		},
		numUnits: 1,
	})

	expected := bsonMById{{
		"_id": s.state.docID("my-local-app"),
		"charm-origin": bson.M{
			"hash":     "",
			"id":       "",
			"platform": bson.M{"os": "ubuntu", "channel": "20.04"},
			"source":   "local",
			"revision": 1,
		},
		"charmmodifiedversion": 0,
		"charmurl":             "local:quantal/quantal-dummy-1",
		"exposed":              false,
		"forcecharm":           false,
		"life":                 0,
		"metric-credentials":   []uint8{},
		"minunits":             0,
		"model-uuid":           s.state.ModelUUID(),
		"name":                 "my-local-app",
		"passwordhash":         "",
		"provisioning-state":   nil,
		"relationcount":        0,
		"scale":                0,
		"subordinate":          false,
		"unitcount":            1,
	}}

	appColl, closer := s.state.db().GetRawCollection(applicationsC)
	defer closer()

	expectedData := upgradedData(appColl, expected)
	s.assertUpgradedData(c, EnsureApplicationCharmOriginsNormalised, expectedData)
}

func (s *upgradesSuite) TestEnsureApplicationCharmOriginsNormaliseCH(c *gc.C) {
	ch := AddTestingCharmhubCharmForSeries(c, s.state, "quantal", "dummy")
	platform := Platform{OS: "ubuntu", Channel: "20.04"}
	rev := 6
	_ = addTestingApplication(c, addTestingApplicationParams{
		st:   s.state,
		name: "my-local-app",
		ch:   ch,
		origin: &CharmOrigin{
			Source:   corecharm.Local.String(),
			Platform: &platform,
			Hash:     "some-hash",
			ID:       "some-id",
			Revision: &rev,
			Channel:  &Channel{Track: "20.04", Risk: "stable", Branch: "deadbeef"},
		},
		numUnits: 1,
	})

	expected := bsonMById{{
		"_id": s.state.docID("my-local-app"),
		"charm-origin": bson.M{
			"hash":     "some-hash",
			"id":       "some-id",
			"platform": bson.M{"os": "ubuntu", "channel": "20.04"},
			"channel":  bson.M{"track": "20.04", "risk": "stable", "branch": "deadbeef"},
			"source":   "charm-hub",
			"revision": 1,
		},
		"charmmodifiedversion": 0,
		"charmurl":             "ch:amd64/quantal/dummy-1",
		"exposed":              false,
		"forcecharm":           false,
		"life":                 0,
		"metric-credentials":   []uint8{},
		"minunits":             0,
		"model-uuid":           s.state.ModelUUID(),
		"name":                 "my-local-app",
		"passwordhash":         "",
		"provisioning-state":   nil,
		"relationcount":        0,
		"scale":                0,
		"subordinate":          false,
		"unitcount":            1,
	}}

	appColl, closer := s.state.db().GetRawCollection(applicationsC)
	defer closer()

	expectedData := upgradedData(appColl, expected)
	s.assertUpgradedData(c, EnsureApplicationCharmOriginsNormalised, expectedData)
}

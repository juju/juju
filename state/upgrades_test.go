// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"sort"
	"time"

	"github.com/juju/charm/v9"
	"github.com/juju/errors"
	"github.com/juju/mgo/v3"
	"github.com/juju/mgo/v3/bson"
	"github.com/juju/mgo/v3/txn"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v3"
	"github.com/kr/pretty"
	gc "gopkg.in/check.v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/juju/juju/caas"
	k8s "github.com/juju/juju/caas/kubernetes"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/network"
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

func (s *upgradesSuite) checkAddPruneSettings(c *gc.C, ageProp, sizeProp, defaultAge, defaultSize string, updateFunc func(pool *StatePool) error) {
	settingsColl, settingsCloser := s.state.db().GetRawCollection(settingsC)
	defer settingsCloser()
	_, err := settingsColl.RemoveAll(nil)
	c.Assert(err, jc.ErrorIsNil)

	m1 := s.makeModel(c, "m1", coretesting.Attrs{
		ageProp:  "96h",
		sizeProp: "4G",
	})
	defer m1.Close()

	m2 := s.makeModel(c, "m2", coretesting.Attrs{})
	defer m2.Close()

	err = settingsColl.Insert(bson.M{
		"_id": "someothersettingshouldnotbetouched",
		// non-model setting: should not be touched
		"settings": bson.M{"key": "value"},
	})
	c.Assert(err, jc.ErrorIsNil)

	model1, err := m1.Model()
	c.Assert(err, jc.ErrorIsNil)
	cfg1, err := model1.ModelConfig()
	c.Assert(err, jc.ErrorIsNil)
	expected1 := cfg1.AllAttrs()
	expected1["resource-tags"] = ""

	model2, err := m2.Model()
	c.Assert(err, jc.ErrorIsNil)
	cfg2, err := model2.ModelConfig()
	c.Assert(err, jc.ErrorIsNil)
	expected2 := cfg2.AllAttrs()
	expected2[ageProp] = defaultAge
	expected2[sizeProp] = defaultSize
	expected2["resource-tags"] = ""

	expectedSettings := bsonMById{
		{
			"_id":        m1.ModelUUID() + ":e",
			"settings":   bson.M(expected1),
			"model-uuid": m1.ModelUUID(),
		}, {
			"_id":        m2.ModelUUID() + ":e",
			"settings":   bson.M(expected2),
			"model-uuid": m2.ModelUUID(),
		}, {
			"_id":      "someothersettingshouldnotbetouched",
			"settings": bson.M{"key": "value"},
		},
	}
	sort.Sort(expectedSettings)

	s.assertUpgradedData(c, updateFunc,
		upgradedData(settingsColl, expectedSettings),
	)
}

type fakeBroker struct {
	caas.Broker
}

func (f *fakeBroker) GetClusterMetadata(storageClass string) (result *k8s.ClusterMetadata, err error) {
	return &k8s.ClusterMetadata{
		OperatorStorageClass: &storagev1.StorageClass{
			ObjectMeta: metav1.ObjectMeta{
				Name: "storage-provisioner",
			},
		},
		WorkloadStorageClass: &storagev1.StorageClass{
			ObjectMeta: metav1.ObjectMeta{
				Name: "storage-provisioner",
			},
		},
	}, nil
}

func (s *upgradesSuite) makeCaasModel(c *gc.C, name string, cred names.CloudCredentialTag, attr coretesting.Attrs) *State {
	uuid := utils.MustNewUUID()
	cfg := coretesting.CustomModelConfig(c, coretesting.Attrs{
		"name": name,
		"uuid": uuid.String(),
	}.Merge(attr))
	m, err := s.state.Model()
	c.Assert(err, jc.ErrorIsNil)
	_, st, err := s.controller.NewModel(ModelArgs{
		Type:                    ModelTypeCAAS,
		CloudName:               "dummy",
		CloudRegion:             "dummy-region",
		CloudCredential:         cred,
		Config:                  cfg,
		Owner:                   m.Owner(),
		StorageProviderRegistry: provider.CommonStorageProviders(),
	})
	c.Assert(err, jc.ErrorIsNil)
	return st
}

// makeApplication doesn't do what you think it does here. You can read the
// applicationDoc, but you can't update it using the txn.Op. It will report that
// the transaction failed because the `Assert: txn.DocExists` is wrong, even
// though we got the application from the database.
// We should move the Insert into a bson.M/bson.D
func (s *upgradesSuite) makeApplication(c *gc.C, uuid, name string, life Life) {
	coll, closer := s.state.db().GetRawCollection(applicationsC)
	defer closer()

	curl := "cs:test-charm"
	err := coll.Insert(applicationDoc{
		DocID:     ensureModelUUID(uuid, name),
		Name:      name,
		ModelUUID: uuid,
		CharmURL:  &curl,
		Life:      life,
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *upgradesSuite) TestRemoveUnusedLinkLayerDeviceProviderIDs(c *gc.C) {
	model1 := s.makeModel(c, "model-1", coretesting.Attrs{})
	defer func() { _ = model1.Close() }()

	// Insert 3 provider IDs.
	pidCol, pidCloser := s.state.db().GetRawCollection(providerIDsC)
	defer pidCloser()

	keepLLD := bson.M{"_id": model1.modelUUID() + ":linklayerdevice:keep"}
	keepSubnet := bson.M{"_id": model1.modelUUID() + ":subnet:keep"}
	docs := []interface{}{
		keepLLD,
		keepSubnet,
		bson.M{"_id": model1.modelUUID() + ":linklayerdevice:delete"},
	}
	err := pidCol.Insert(docs...)
	c.Assert(err, jc.ErrorIsNil)

	// Insert a device using one of the IDs.
	lldCol, lldCloser := model1.db().GetCollection(linkLayerDevicesC)
	defer lldCloser()

	err = lldCol.Writeable().Insert(linkLayerDeviceDoc{
		ProviderID: "keep",
	})
	c.Assert(err, jc.ErrorIsNil)

	// Check that only the unreferenced link-layer device ID was removed.
	s.assertUpgradedData(c, RemoveUnusedLinkLayerDeviceProviderIDs, upgradedData(pidCol, []bson.M{
		keepLLD,
		keepSubnet,
	}))
}

func (s *upgradesSuite) TestUpdateDHCPAddressConfigs(c *gc.C) {
	model1 := s.makeModel(c, "model-1", coretesting.Attrs{})
	defer func() { _ = model1.Close() }()

	col, closer := s.state.db().GetRawCollection(ipAddressesC)
	defer closer()

	docs := []interface{}{
		bson.M{"_id": model1.modelUUID() + ":m#0#d#eth0#ip#10.10.10.10", "config-method": "dynamic"},
		bson.M{"_id": model1.modelUUID() + ":m#1#d#eth1#ip#20.20.20.20", "config-method": network.ConfigStatic},
	}
	err := col.Insert(docs...)
	c.Assert(err, jc.ErrorIsNil)

	// The first of the docs has an upgraded config method.
	s.assertUpgradedData(c, UpdateDHCPAddressConfigs, upgradedData(col, []bson.M{
		{"_id": model1.modelUUID() + ":m#0#d#eth0#ip#10.10.10.10", "config-method": string(network.ConfigDHCP)},
		{"_id": model1.modelUUID() + ":m#1#d#eth1#ip#20.20.20.20", "config-method": string(network.ConfigStatic)},
	}))
}

func (s *upgradesSuite) TestAddSpawnedTaskCountToOperations(c *gc.C) {
	operationsCol, closerOne := s.state.db().GetRawCollection(operationsC)
	defer closerOne()

	actionsCol, closerTwo := s.state.db().GetRawCollection(actionsC)
	defer closerTwo()

	model1 := s.makeModel(c, "model-1", coretesting.Attrs{})
	model2 := s.makeModel(c, "model-2", coretesting.Attrs{})
	defer func() {
		_ = model1.Close()
		_ = model2.Close()
	}()

	uuid1 := model1.ModelUUID()
	uuid2 := model2.ModelUUID()

	err := operationsCol.Insert(
		// ---- model 1 ----
		bson.M{
			"_id":        ensureModelUUID(uuid1, "2"),
			"model-uuid": uuid1,
		},
		bson.M{
			"_id":        ensureModelUUID(uuid1, "10"),
			"model-uuid": uuid1,
		},
		// ---- model 2 ----
		bson.M{
			"_id":        ensureModelUUID(uuid2, "2"),
			"model-uuid": uuid2,
		},
	)
	c.Assert(err, jc.ErrorIsNil)

	err = actionsCol.Insert(
		bson.M{
			"_id":        ensureModelUUID(uuid1, "3"),
			"model-uuid": uuid1,
			"operation":  "2",
		},
		bson.M{
			"_id":        ensureModelUUID(uuid1, "11"),
			"model-uuid": uuid1,
			"operation":  "10",
		},
		bson.M{
			"_id":        ensureModelUUID(uuid1, "12"),
			"model-uuid": uuid1,
			"operation":  "10",
		},
		bson.M{
			"_id":        ensureModelUUID(uuid2, "3"),
			"operation":  "2",
			"model-uuid": uuid2,
		},
		bson.M{
			"_id":        ensureModelUUID(uuid2, "4"),
			"operation":  "2",
			"model-uuid": uuid2,
		},
		bson.M{
			"_id":        ensureModelUUID(uuid2, "5"),
			"operation":  "2",
			"model-uuid": uuid2,
		},
	)
	c.Assert(err, jc.ErrorIsNil)

	expectedOperations := bsonMById{
		{
			"_id":                ensureModelUUID(uuid1, "2"),
			"model-uuid":         uuid1,
			"spawned-task-count": 1,
		},
		{
			"_id":                ensureModelUUID(uuid1, "10"),
			"model-uuid":         uuid1,
			"spawned-task-count": 2,
		},
		{
			"_id":                ensureModelUUID(uuid2, "2"),
			"model-uuid":         uuid2,
			"spawned-task-count": 3,
		},
	}

	sort.Sort(expectedOperations)

	s.assertUpgradedData(c, AddSpawnedTaskCountToOperations,
		upgradedData(operationsCol, expectedOperations),
	)
}

func (s *upgradesSuite) TestTransformEmptyManifestsToNil(c *gc.C) {
	model1 := s.makeModel(c, "model-1", coretesting.Attrs{})
	model2 := s.makeModel(c, "model-2", coretesting.Attrs{})
	defer func() {
		_ = model1.Close()
		_ = model2.Close()
	}()

	uuid1 := model1.ModelUUID()
	uuid2 := model2.ModelUUID()

	coll, closer := s.state.db().GetRawCollection(charmsC)
	defer closer()

	err := coll.Insert(bson.M{
		"_id":        ensureModelUUID(uuid1, "charm1"),
		"model-uuid": uuid1,
		"url":        charm.MustParseURL("cs:test").String(),
		"manifest":   nil,
	})
	c.Assert(err, jc.ErrorIsNil)
	err = coll.Insert(bson.M{
		"_id":        ensureModelUUID(uuid2, "charm2"),
		"model-uuid": uuid2,
		"url":        charm.MustParseURL("local:test").String(),
		"manifest":   &charm.Manifest{},
	})
	c.Assert(err, jc.ErrorIsNil)
	err = coll.Insert(bson.M{
		"_id":        ensureModelUUID(uuid2, "charm3"),
		"model-uuid": uuid2,
		"url":        charm.MustParseURL("ch:test").String(),
		"manifest": &charm.Manifest{
			Bases: []charm.Base{},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	err = coll.Insert(bson.M{
		"_id":        ensureModelUUID(uuid1, "charm4"),
		"model-uuid": uuid1,
		"url":        charm.MustParseURL("ch:test2").String(),
		"manifest": &charm.Manifest{
			Bases: []charm.Base{
				{Name: "ubuntu"},
			},
		},
	})
	c.Assert(err, jc.ErrorIsNil)

	expected := bsonMById{
		{
			"_id":        ensureModelUUID(uuid1, "charm1"),
			"model-uuid": uuid1,
			"url":        "cs:test",
		},
		{
			"_id":        ensureModelUUID(uuid2, "charm2"),
			"model-uuid": uuid2,
			"url":        "local:test",
		},
		{
			"_id":        ensureModelUUID(uuid2, "charm3"),
			"model-uuid": uuid2,
			"url":        "ch:test",
		},
		{
			"_id":        ensureModelUUID(uuid1, "charm4"),
			"model-uuid": uuid1,
			"url":        "ch:test2",
			"manifest": bson.M{
				"bases": []interface{}{
					bson.M{
						"name": "ubuntu",
					},
				},
			},
		},
	}

	sort.Sort(expected)
	s.assertUpgradedData(c, TransformEmptyManifestsToNil,
		upgradedData(coll, expected),
	)
}

func (s *upgradesSuite) TestEnsureCharmOriginRisk(c *gc.C) {
	model1 := s.makeModel(c, "model-1", coretesting.Attrs{})
	model2 := s.makeModel(c, "model-2", coretesting.Attrs{})
	defer func() {
		_ = model1.Close()
		_ = model2.Close()
	}()

	uuid1 := model1.ModelUUID()
	uuid2 := model2.ModelUUID()

	appColl, appCloser := s.state.db().GetRawCollection(applicationsC)
	defer appCloser()

	var err error
	err = appColl.Insert(bson.M{
		"_id":        ensureModelUUID(uuid1, "app1"),
		"name":       "app1",
		"model-uuid": uuid1,
		"charmurl":   charm.MustParseURL("cs:test").String(),
	})
	c.Assert(err, jc.ErrorIsNil)
	err = appColl.Insert(bson.M{
		"_id":        ensureModelUUID(uuid1, "app2"),
		"name":       "app2",
		"model-uuid": uuid1,
		"charmurl":   charm.MustParseURL("local:test").String(),
		"charm-origin": bson.M{
			"source":   "local",
			"type":     "charm",
			"revision": 12,
			"platform": bson.M{
				"architecture": "amd64",
				"series":       "focal",
			},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	err = appColl.Insert(bson.M{
		"_id":        ensureModelUUID(uuid2, "app3"),
		"name":       "app3",
		"model-uuid": uuid2,
		"charmurl":   charm.MustParseURL("local:test2").String(),
		"charm-origin": bson.M{
			"source":   "local",
			"type":     "charm",
			"id":       "local:test",
			"hash":     "",
			"revision": -1,
			"platform": bson.M{
				"architecture": "amd64",
				"series":       "focal",
			},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	err = appColl.Insert(bson.M{
		"_id":        ensureModelUUID(uuid2, "app4"),
		"name":       "app4",
		"model-uuid": uuid2,
		"charmurl":   charm.MustParseURL("cs:focal/test-13").String(),
		"cs-channel": "edge",
		"charm-origin": bson.M{
			"source":   "charm-store",
			"type":     "charm",
			"revision": 12,
			"channel": bson.M{
				"track": "latest",
				"risk":  "",
			},
			"platform": bson.M{
				"architecture": "amd64",
				"series":       "focal",
			},
		},
	})
	c.Assert(err, jc.ErrorIsNil)

	err = appColl.Insert(bson.M{
		"_id":        ensureModelUUID(uuid2, "app5"),
		"name":       "app5",
		"model-uuid": uuid2,
		"charmurl":   charm.MustParseURL("ch:amd64/focal/test").String(),
		"charm-origin": bson.M{
			"source":   "charm-hub",
			"type":     "charm",
			"id":       "yyyy",
			"hash":     "xxxx",
			"revision": 12,
			"channel": bson.M{
				"track": "latest",
				"risk":  "",
			},
			"platform": bson.M{
				"architecture": "amd64",
				"series":       "focal",
			},
		},
	})
	c.Assert(err, jc.ErrorIsNil)

	expected := bsonMById{
		{
			"_id":        ensureModelUUID(uuid1, "app1"),
			"model-uuid": uuid1,
			"name":       "app1",
			"charmurl":   "cs:test",
		},
		{
			"_id":        ensureModelUUID(uuid1, "app2"),
			"model-uuid": uuid1,
			"name":       "app2",
			"charmurl":   "local:test",
			"charm-origin": bson.M{
				"source":   "local",
				"type":     "charm",
				"revision": 12,
				"platform": bson.M{
					"architecture": "amd64",
					"series":       "focal",
				},
			},
		},
		{
			"_id":        ensureModelUUID(uuid2, "app3"),
			"model-uuid": uuid2,
			"name":       "app3",
			"charmurl":   "local:test2",
			"charm-origin": bson.M{
				"source":   "local",
				"type":     "charm",
				"id":       "local:test",
				"hash":     "",
				"revision": -1,
				"platform": bson.M{
					"architecture": "amd64",
					"series":       "focal",
				},
			},
		},
		{
			"_id":        ensureModelUUID(uuid2, "app4"),
			"model-uuid": uuid2,
			"name":       "app4",
			"charmurl":   "cs:focal/test-13",
			"cs-channel": "edge",
			"charm-origin": bson.M{
				"source":   "charm-store",
				"type":     "charm",
				"revision": 12,
				"channel": bson.M{
					"track": "latest",
					"risk":  "edge",
				},
				"platform": bson.M{
					"architecture": "amd64",
					"series":       "focal",
				},
			},
		},
		{
			"_id":        ensureModelUUID(uuid2, "app5"),
			"model-uuid": uuid2,
			"name":       "app5",
			"charmurl":   "ch:amd64/focal/test",
			"charm-origin": bson.M{
				"source":   "charm-hub",
				"type":     "charm",
				"revision": 12,
				"hash":     "xxxx",
				"id":       "yyyy",
				"channel": bson.M{
					"track": "latest",
					"risk":  "stable",
				},
				"platform": bson.M{
					"architecture": "amd64",
					"series":       "focal",
				},
			},
		},
	}

	sort.Sort(expected)
	s.assertUpgradedData(c, EnsureCharmOriginRisk,
		upgradedData(appColl, expected),
	)
}

func (s *upgradesSuite) TestRemoveOrphanedCrossModelProxies(c *gc.C) {
	ch := AddTestingCharm(c, s.state, "mysql")
	AddTestingApplication(c, s.state, "test", ch)
	_, err := s.state.AddUser("fred", "fred", "secret", "admin")
	c.Assert(err, jc.ErrorIsNil)
	sd := NewApplicationOffers(s.state)
	offerArgs := crossmodel.AddApplicationOfferArgs{
		OfferName:       "test",
		ApplicationName: "test",
		Endpoints:       map[string]string{"db": "server"},
		Owner:           "fred",
	}
	offer, err := sd.AddOffer(offerArgs)
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.state.AddRemoteApplication(AddRemoteApplicationParams{
		Name:            "good",
		SourceModel:     s.state.modelTag,
		OfferUUID:       offer.OfferUUID,
		IsConsumerProxy: true,
		Endpoints: []charm.Relation{{
			Interface: "mysql",
			Name:      "server",
			Role:      charm.RoleProvider,
			Scope:     charm.ScopeGlobal,
		}}})
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.state.AddRemoteApplication(AddRemoteApplicationParams{
		Name:        "saas",
		SourceModel: s.state.modelTag,
		OfferUUID:   offer.OfferUUID,
		Endpoints: []charm.Relation{{
			Interface: "mysql",
			Name:      "database",
			Role:      charm.RoleRequirer,
			Scope:     charm.ScopeGlobal,
		}}})
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.state.AddRemoteApplication(AddRemoteApplicationParams{
		Name:            "orphaned",
		SourceModel:     s.state.modelTag,
		OfferUUID:       "missing-uuid",
		IsConsumerProxy: true,
		Endpoints: []charm.Relation{{
			Interface: "mysql",
			Name:      "server",
			Role:      charm.RoleProvider,
			Scope:     charm.ScopeGlobal,
		}}})
	c.Assert(err, jc.ErrorIsNil)

	// Two rounds to check idempotency.
	for i := 0; i < 2; i++ {
		c.Logf("Run: %d", i)
		err := RemoveOrphanedCrossModelProxies(s.pool)
		c.Assert(err, jc.ErrorIsNil)

		_, err = s.state.RemoteApplication("orphaned")
		c.Assert(err, jc.Satisfies, errors.IsNotFound)
		_, err = s.state.RemoteApplication("good")
		c.Assert(err, jc.ErrorIsNil)
		_, err = s.state.RemoteApplication("saas")
		c.Assert(err, jc.ErrorIsNil)
	}
}

func (s *upgradesSuite) TestDropLegacyAssumesSectionsFromCharmMetadata(c *gc.C) {
	model1 := s.makeModel(c, "model-1", coretesting.Attrs{})
	model2 := s.makeModel(c, "model-2", coretesting.Attrs{})
	defer func() {
		_ = model1.Close()
		_ = model2.Close()
	}()

	uuid1 := model1.ModelUUID()
	uuid2 := model2.ModelUUID()

	charmsColl, closer := s.state.db().GetRawCollection(charmsC)
	defer closer()

	var err error
	err = charmsColl.Insert(bson.M{
		"_id":        ensureModelUUID(uuid1, "charm1"),
		"model-uuid": uuid1,
		"name":       "charm1",
		"assumes":    []string{"lorem", "ipsum"},
	})
	c.Assert(err, jc.ErrorIsNil)
	err = charmsColl.Insert(bson.M{
		"_id":        ensureModelUUID(uuid2, "charm2"),
		"model-uuid": uuid2,
		"name":       "charm2",
		"assumes":    []string{"foo", "bar"},
	})
	c.Assert(err, jc.ErrorIsNil)

	expected := bsonMById{
		{
			"_id":        ensureModelUUID(uuid1, "charm1"),
			"model-uuid": uuid1,
			"name":       "charm1",
		},
		{
			"_id":        ensureModelUUID(uuid2, "charm2"),
			"model-uuid": uuid2,
			"name":       "charm2",
		},
	}

	sort.Sort(expected)
	s.assertUpgradedData(c, DropLegacyAssumesSectionsFromCharmMetadata,
		upgradedData(charmsColl, expected),
	)
}

func (s *upgradesSuite) TestMigrateLegacyCrossModelTokens(c *gc.C) {
	model1 := s.makeModel(c, "model-1", coretesting.Attrs{})
	model2 := s.makeModel(c, "model-2", coretesting.Attrs{})
	defer func() {
		_ = model1.Close()
		_ = model2.Close()
	}()

	uuid1 := model1.ModelUUID()
	uuid2 := model2.ModelUUID()

	makeOffer := func(st *State, username, exportedName string) (string, string) {
		ch := AddTestingCharm(c, st, "mysql")
		AddTestingApplication(c, st, "test", ch)
		_, err := st.AddUser(username, username, "secret", "admin")
		c.Assert(err, jc.ErrorIsNil)
		sd := NewApplicationOffers(st)
		offerArgs := crossmodel.AddApplicationOfferArgs{
			OfferName:       "testoffer",
			ApplicationName: "test",
			Endpoints:       map[string]string{"db": "server"},
			Owner:           username,
		}
		_, err = sd.AddOffer(offerArgs)
		c.Assert(err, jc.ErrorIsNil)

		r := st.RemoteEntities()
		token, err := r.ExportLocalEntity(names.NewApplicationTag(exportedName))
		c.Assert(err, jc.ErrorIsNil)
		relToken, err := r.ExportLocalEntity(names.NewRelationTag("foo:bar"))
		c.Assert(err, jc.ErrorIsNil)
		return token, relToken
	}

	token1, relToken1 := makeOffer(model1, "fred", "test")
	token2, relToken2 := makeOffer(model2, "mary", "testoffer")

	expected := bsonMById{
		{
			"_id":        ensureModelUUID(uuid1, "application-testoffer"),
			"model-uuid": uuid1,
			"token":      token1,
		}, {
			"_id":        ensureModelUUID(uuid1, "relation-foo.bar"),
			"model-uuid": uuid1,
			"token":      relToken1,
		}, {
			"_id":        ensureModelUUID(uuid2, "application-testoffer"),
			"model-uuid": uuid2,
			"token":      token2,
		}, {
			"_id":        ensureModelUUID(uuid2, "relation-foo.bar"),
			"model-uuid": uuid2,
			"token":      relToken2,
		},
	}
	sort.Sort(expected)

	col, closer := s.state.db().GetRawCollection(remoteEntitiesC)
	defer closer()

	s.assertUpgradedData(c, MigrateLegacyCrossModelTokens,
		upgradedData(col, expected),
	)
}

func (s *upgradesSuite) TestCleanupDeadAssignUnits(c *gc.C) {
	model0 := s.makeModel(c, "model-0", coretesting.Attrs{})
	model1 := s.makeModel(c, "model-1", coretesting.Attrs{})
	defer func() {
		_ = model0.Close()
		_ = model1.Close()
	}()

	assignUnitColl, assignUnitCloser := s.state.db().GetRawCollection(assignUnitC)
	defer assignUnitCloser()
	s.makeApplication(c, model0.ModelUUID(), "app01", Alive)
	s.makeApplication(c, model0.ModelUUID(), "app02", Dying)
	s.makeApplication(c, model0.ModelUUID(), "app03", Dead)
	s.makeApplication(c, model1.ModelUUID(), "app11", Alive)
	s.makeApplication(c, model1.ModelUUID(), "app12", Dying)
	s.makeApplication(c, model1.ModelUUID(), "app13", Dead)
	err := assignUnitColl.Insert(
		bson.M{
			"_id":        model0.docID("app01/0"),
			"model-uuid": model0.ModelUUID(),
		},
		bson.M{
			"_id":        model0.docID("app02/0"),
			"model-uuid": model0.ModelUUID(),
		},
		bson.M{
			"_id":        model0.docID("app03/0"), // remove: dead app.
			"model-uuid": model0.ModelUUID(),
		},
		bson.M{
			"_id":        model0.docID("non-exist-app/0"), // remove: non-exist app.
			"model-uuid": model0.ModelUUID(),
		},
		bson.M{
			"_id":        model1.docID("app11/0"),
			"model-uuid": model1.ModelUUID(),
		},
		bson.M{
			"_id":        model1.docID("app12/0"),
			"model-uuid": model1.ModelUUID(),
		},
		bson.M{
			"_id":        model1.docID("app13/0"), // remove: dead app.
			"model-uuid": model1.ModelUUID(),
		},
		bson.M{
			"_id":        model1.docID("non-exist-app/0"), // remove: non-exist app.
			"model-uuid": model1.ModelUUID(),
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	count, err := assignUnitColl.Count()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(count, gc.Equals, 8)

	expected := bsonMById{
		{
			"_id":        model0.docID("app01/0"),
			"model-uuid": model0.ModelUUID(),
		},
		{
			"_id":        model0.docID("app02/0"),
			"model-uuid": model0.ModelUUID(),
		},
		{
			"_id":        model1.docID("app11/0"),
			"model-uuid": model1.ModelUUID(),
		},
		{
			"_id":        model1.docID("app12/0"),
			"model-uuid": model1.ModelUUID(),
		},
	}
	sort.Sort(expected)

	s.assertUpgradedData(c, CleanupDeadAssignUnits,
		upgradedData(assignUnitColl, expected),
	)
}

func (s *upgradesSuite) TestRemoveOrphanedLinkLayerDevices(c *gc.C) {
	// Add 2 machines with link-layer devices.
	m0, err := s.state.AddOneMachine(MachineTemplate{
		Series: "focal",
		Jobs:   []MachineJob{JobHostUnits},
	})
	c.Assert(err, jc.ErrorIsNil)

	m1, err := s.state.AddOneMachine(MachineTemplate{
		Series: "focal",
		Jobs:   []MachineJob{JobHostUnits},
	})
	c.Assert(err, jc.ErrorIsNil)

	ops, err := m0.AddLinkLayerDeviceOps(
		LinkLayerDeviceArgs{
			Name: "eth0",
			Type: network.EthernetDevice,
		},
		LinkLayerDeviceAddress{
			DeviceName:  "eth0",
			CIDRAddress: "192.168.0.66/24",
		},
	)
	c.Assert(err, jc.ErrorIsNil)

	err = s.state.db().RunTransaction(ops)
	c.Assert(err, jc.ErrorIsNil)

	ops, err = m1.AddLinkLayerDeviceOps(
		LinkLayerDeviceArgs{
			Name: "eth0",
			Type: network.EthernetDevice,
		},
		LinkLayerDeviceAddress{
			DeviceName:  "eth0",
			CIDRAddress: "192.168.0.99/24",
		},
	)
	c.Assert(err, jc.ErrorIsNil)

	err = s.state.db().RunTransaction(ops)
	c.Assert(err, jc.ErrorIsNil)

	// Delete the first machine directly leaving the link-layer data orphaned.
	mCol, mCloser := s.state.db().GetCollection(machinesC)
	defer mCloser()

	err = mCol.Writeable().Remove(bson.M{"machineid": m0.Id()})
	c.Assert(err, jc.ErrorIsNil)

	devCol, devCloser := s.state.db().GetRawCollection(linkLayerDevicesC)
	defer devCloser()

	addrCol, addrCloser := s.state.db().GetRawCollection(ipAddressesC)
	defer addrCloser()

	// Only the link-layer data for the second machine should be retained.
	devExp := bsonMById{{
		"_id":               ensureModelUUID(s.state.modelUUID(), fmt.Sprintf("m#%s#d#eth0", m1.Id())),
		"model-uuid":        s.state.modelUUID(),
		"is-auto-start":     false,
		"is-up":             false,
		"mac-address":       "",
		"machine-id":        m1.Id(),
		"mtu":               0,
		"name":              "eth0",
		"parent-name":       "",
		"type":              "ethernet",
		"virtual-port-type": "",
	}}

	addrExp := bsonMById{{
		"_id":           ensureModelUUID(s.state.modelUUID(), fmt.Sprintf("m#%s#d#eth0#ip#192.168.0.99", m1.Id())),
		"model-uuid":    s.state.modelUUID(),
		"config-method": "",
		"device-name":   "eth0",
		"machine-id":    m1.Id(),
		"origin":        "",
		"subnet-cidr":   "192.168.0.0/24",
		"value":         "192.168.0.99",
	}}

	s.assertUpgradedData(c, RemoveOrphanedLinkLayerDevices,
		upgradedData(devCol, devExp),
		upgradedData(addrCol, addrExp),
	)
}

func (s *upgradesSuite) TestUpdateExternalControllerInfo(c *gc.C) {
	model0 := s.makeModel(c, "model-0", coretesting.Attrs{})
	model1 := s.makeModel(c, "model-1", coretesting.Attrs{})
	defer func() {
		_ = model0.Close()
		_ = model1.Close()
	}()

	extControllerUUID := utils.MustNewUUID().String()
	modelUUID1 := utils.MustNewUUID().String()
	modelUUID2 := utils.MustNewUUID().String()

	ec := NewExternalControllers(s.state)
	_, err := ec.Save(crossmodel.ControllerInfo{
		ControllerTag: names.NewControllerTag(extControllerUUID),
		Addrs:         []string{"10.0.0.1:17070"},
	}, modelUUID1)
	c.Assert(err, jc.ErrorIsNil)
	_, err = ec.Save(crossmodel.ControllerInfo{
		ControllerTag: coretesting.ControllerTag,
		Addrs:         []string{"10.0.0.2:17070"},
	}, coretesting.ModelTag.Id())
	c.Assert(err, jc.ErrorIsNil)

	_, err = model0.AddRemoteApplication(AddRemoteApplicationParams{
		Name:        "remote-application",
		SourceModel: names.NewModelTag(modelUUID1),
	})
	c.Assert(err, jc.ErrorIsNil)
	_, err = model0.AddRemoteApplication(AddRemoteApplicationParams{
		Name:        "remote-application2",
		SourceModel: names.NewModelTag(modelUUID1),
	})
	c.Assert(err, jc.ErrorIsNil)
	_, err = model0.AddRemoteApplication(AddRemoteApplicationParams{
		Name:        "remote-application3",
		SourceModel: names.NewModelTag(modelUUID2),
	})
	c.Assert(err, jc.ErrorIsNil)
	_, err = model0.AddRemoteApplication(AddRemoteApplicationParams{
		Name:            "remote-application4",
		SourceModel:     names.NewModelTag(modelUUID1),
		IsConsumerProxy: true,
	})
	c.Assert(err, jc.ErrorIsNil)
	_, err = model1.AddRemoteApplication(AddRemoteApplicationParams{
		Name:        "remote-application5",
		SourceModel: names.NewModelTag(modelUUID1),
	})
	c.Assert(err, jc.ErrorIsNil)

	expectedApps := bsonMById{
		{
			"_id":                    model0.docID("remote-application"),
			"bindings":               bson.M{},
			"endpoints":              []interface{}{},
			"is-consumer-proxy":      false,
			"life":                   0,
			"model-uuid":             model0.modelUUID(),
			"name":                   "remote-application",
			"offer-uuid":             "",
			"relationcount":          0,
			"source-controller-uuid": extControllerUUID,
			"source-model-uuid":      modelUUID1,
			"spaces":                 []interface{}{},
		},
		{
			"_id":                    model0.docID("remote-application2"),
			"bindings":               bson.M{},
			"endpoints":              []interface{}{},
			"is-consumer-proxy":      false,
			"life":                   0,
			"model-uuid":             model0.modelUUID(),
			"name":                   "remote-application2",
			"offer-uuid":             "",
			"relationcount":          0,
			"source-controller-uuid": extControllerUUID,
			"source-model-uuid":      modelUUID1,
			"spaces":                 []interface{}{},
		},
		{
			"_id":                    model0.docID("remote-application3"),
			"bindings":               bson.M{},
			"endpoints":              []interface{}{},
			"is-consumer-proxy":      false,
			"life":                   0,
			"model-uuid":             model0.modelUUID(),
			"name":                   "remote-application3",
			"offer-uuid":             "",
			"relationcount":          0,
			"source-controller-uuid": "",
			"source-model-uuid":      modelUUID2,
			"spaces":                 []interface{}{},
		},
		{
			"_id":                    model0.docID("remote-application4"),
			"bindings":               bson.M{},
			"endpoints":              []interface{}{},
			"is-consumer-proxy":      true,
			"life":                   0,
			"model-uuid":             model0.modelUUID(),
			"name":                   "remote-application4",
			"offer-uuid":             "",
			"relationcount":          0,
			"source-controller-uuid": "",
			"source-model-uuid":      modelUUID1,
			"spaces":                 []interface{}{},
		},
		{
			"_id":                    model1.docID("remote-application5"),
			"bindings":               bson.M{},
			"endpoints":              []interface{}{},
			"is-consumer-proxy":      false,
			"life":                   0,
			"model-uuid":             model1.modelUUID(),
			"name":                   "remote-application5",
			"offer-uuid":             "",
			"relationcount":          0,
			"source-controller-uuid": extControllerUUID,
			"source-model-uuid":      modelUUID1,
			"spaces":                 []interface{}{},
		},
	}
	sort.Sort(expectedApps)

	appColl, aCloser := s.state.db().GetRawCollection(remoteApplicationsC)
	defer aCloser()
	s.assertUpgradedData(c, UpdateExternalControllerInfo,
		upgradedData(appColl, expectedApps),
	)

	// Check the ref counts.
	refcounts, closer := s.state.db().GetCollection(globalRefcountsC)
	defer closer()
	key := externalControllerRefCountKey(extControllerUUID)
	count, err := nsRefcounts.read(refcounts, key)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(count, gc.Equals, 3)

	// Check the orphaned controller is removed and we still have
	// the in use controller.
	ec = NewExternalControllers(s.state)
	_, err = ec.Controller(coretesting.ControllerTag.Id())
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	_, err = ec.Controller(extControllerUUID)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *upgradesSuite) TestUpdateExternalControllerInfoFixRefCount(c *gc.C) {
	model0 := s.makeModel(c, "model-0", coretesting.Attrs{})
	defer func() {
		_ = model0.Close()
	}()

	extControllerUUID := utils.MustNewUUID().String()
	modelUUID1 := utils.MustNewUUID().String()

	ec := NewExternalControllers(s.state)
	_, err := ec.Save(crossmodel.ControllerInfo{
		ControllerTag: names.NewControllerTag(extControllerUUID),
		Addrs:         []string{"10.0.0.1:17070"},
	}, modelUUID1)
	c.Assert(err, jc.ErrorIsNil)
	_, err = ec.Save(crossmodel.ControllerInfo{
		ControllerTag: coretesting.ControllerTag,
		Addrs:         []string{"10.0.0.2:17070"},
	}, coretesting.ModelTag.Id())
	c.Assert(err, jc.ErrorIsNil)

	_, err = model0.AddRemoteApplication(AddRemoteApplicationParams{
		Name:        "remote-application",
		SourceModel: names.NewModelTag(modelUUID1),
	})
	c.Assert(err, jc.ErrorIsNil)
	_, err = model0.AddRemoteApplication(AddRemoteApplicationParams{
		Name:        "remote-application2",
		SourceModel: names.NewModelTag(modelUUID1),
	})
	c.Assert(err, jc.ErrorIsNil)

	// Add a bad ref count.
	refcounts, closer := s.state.db().GetCollection(globalRefcountsC)
	defer closer()
	key := externalControllerRefCountKey(extControllerUUID)
	op, err := nsRefcounts.CreateOrIncRefOp(refcounts, key, 1)
	c.Assert(err, jc.ErrorIsNil)
	err = s.state.db().RunTransaction([]txn.Op{op})
	c.Assert(err, jc.ErrorIsNil)

	expectedApps := bsonMById{
		{
			"_id":                    model0.docID("remote-application"),
			"bindings":               bson.M{},
			"endpoints":              []interface{}{},
			"is-consumer-proxy":      false,
			"life":                   0,
			"model-uuid":             model0.modelUUID(),
			"name":                   "remote-application",
			"offer-uuid":             "",
			"relationcount":          0,
			"source-controller-uuid": extControllerUUID,
			"source-model-uuid":      modelUUID1,
			"spaces":                 []interface{}{},
		},
		{
			"_id":                    model0.docID("remote-application2"),
			"bindings":               bson.M{},
			"endpoints":              []interface{}{},
			"is-consumer-proxy":      false,
			"life":                   0,
			"model-uuid":             model0.modelUUID(),
			"name":                   "remote-application2",
			"offer-uuid":             "",
			"relationcount":          0,
			"source-controller-uuid": extControllerUUID,
			"source-model-uuid":      modelUUID1,
			"spaces":                 []interface{}{},
		},
	}
	sort.Sort(expectedApps)

	appColl, aCloser := s.state.db().GetRawCollection(remoteApplicationsC)
	defer aCloser()
	s.assertUpgradedData(c, UpdateExternalControllerInfo,
		upgradedData(appColl, expectedApps),
	)

	// Check the ref counts.
	count, err := nsRefcounts.read(refcounts, key)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(count, gc.Equals, 2)
}

func (s *upgradesSuite) TestRemoveInvalidCharmPlaceholders(c *gc.C) {
	model1 := s.makeModel(c, "model-1", coretesting.Attrs{})
	model2 := s.makeModel(c, "model-2", coretesting.Attrs{})
	defer func() {
		_ = model1.Close()
		_ = model2.Close()
	}()

	uuid1 := model1.ModelUUID()
	uuid2 := model2.ModelUUID()

	charmColl, charmCloser := s.state.db().GetRawCollection(charmsC)
	defer charmCloser()

	appColl, appCloser := s.state.db().GetRawCollection(applicationsC)
	defer appCloser()

	var err error
	err = charmColl.Insert(bson.M{
		"_id":         ensureModelUUID(uuid1, "charm1"),
		"model-uuid":  uuid1,
		"url":         charm.MustParseURL("ch:test-1").String(),
		"placeholder": true,
	})
	c.Assert(err, jc.ErrorIsNil)
	err = charmColl.Insert(bson.M{
		"_id":         ensureModelUUID(uuid2, "charm2"),
		"model-uuid":  uuid2,
		"url":         charm.MustParseURL("ch:test-2").String(),
		"placeholder": true,
	})
	c.Assert(err, jc.ErrorIsNil)
	err = charmColl.Insert(bson.M{
		"_id":         ensureModelUUID(uuid2, "charm3"),
		"model-uuid":  uuid2,
		"url":         charm.MustParseURL("ch:test-3").String(),
		"placeholder": false,
	})
	c.Assert(err, jc.ErrorIsNil)
	err = appColl.Insert(bson.M{
		"_id":        ensureModelUUID(uuid1, "app1"),
		"name":       "app1",
		"model-uuid": uuid1,
		"charmurl":   charm.MustParseURL("ch:test-1").String(),
	})
	c.Assert(err, jc.ErrorIsNil)

	expected := bsonMById{
		{
			"_id":         ensureModelUUID(uuid1, "charm1"),
			"model-uuid":  uuid1,
			"url":         charm.MustParseURL("ch:test-1").String(),
			"placeholder": true,
		},
		{
			"_id":         ensureModelUUID(uuid2, "charm3"),
			"model-uuid":  uuid2,
			"url":         charm.MustParseURL("ch:test-3").String(),
			"placeholder": false,
		},
	}

	sort.Sort(expected)
	s.assertUpgradedData(c, RemoveInvalidCharmPlaceholders,
		upgradedData(charmColl, expected),
	)
}

func (s *upgradesSuite) TestSetContainerAddressOriginToMachine(c *gc.C) {
	col, closer := s.state.db().GetRawCollection(ipAddressesC)
	defer closer()

	uuid1 := utils.MustNewUUID().String()
	uuid2 := utils.MustNewUUID().String()

	err := col.Insert(bson.M{
		"_id":        uuid1 + ":principal/1",
		"model-uuid": uuid1,
		"machine-id": "0",
		"origin":     "provider",
	}, bson.M{
		"_id":        uuid1 + ":telegraf/1",
		"model-uuid": uuid1,
		"machine-id": "0/lxd/0",
		"origin":     "provider",
	}, bson.M{
		"_id":        uuid2 + ":telegraf/0",
		"model-uuid": uuid2,
		"machine-id": "11/kvm/11",
		"origin":     "provider",
	})
	c.Assert(err, jc.ErrorIsNil)

	// The first origin is unchanged - it is not a container/VM in-machine.
	expected := bsonMById{
		{
			"_id":        uuid1 + ":principal/1",
			"model-uuid": uuid1,
			"machine-id": "0",
			"origin":     "provider",
		}, {
			"_id":        uuid1 + ":telegraf/1",
			"model-uuid": uuid1,
			"machine-id": "0/lxd/0",
			"origin":     "machine",
		}, {
			"_id":        uuid2 + ":telegraf/0",
			"model-uuid": uuid2,
			"machine-id": "11/kvm/11",
			"origin":     "machine",
		},
	}

	sort.Sort(expected)
	s.assertUpgradedData(c, SetContainerAddressOriginToMachine, upgradedData(col, expected))
}

func (s *upgradesSuite) TestUpdateCharmOriginAfterSetSeries(c *gc.C) {
	model1 := s.makeModel(c, "model-1", coretesting.Attrs{})
	model2 := s.makeModel(c, "model-2", coretesting.Attrs{})
	defer func() {
		_ = model1.Close()
		_ = model2.Close()
	}()

	uuid1 := model1.ModelUUID()
	uuid2 := model2.ModelUUID()

	appColl, appCloser := s.state.db().GetRawCollection(applicationsC)
	defer appCloser()

	var err error
	err = appColl.Insert(bson.M{
		"_id":        ensureModelUUID(uuid1, "app1"),
		"model-uuid": uuid1,
		"charmurl":   charm.MustParseURL("cs:test").String(),
		"series":     "focal",
		"charm-origin": bson.M{
			"platform": bson.M{
				"architecture": "amd64",
				"series":       "focal",
			},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	err = appColl.Insert(bson.M{
		"_id":        ensureModelUUID(uuid2, "app2"),
		"model-uuid": uuid2,
		"charmurl":   charm.MustParseURL("ch:test").String(),
		"series":     "focal",
		"charm-origin": bson.M{
			"platform": bson.M{
				"architecture": "amd64",
				"series":       "bionic",
			},
		},
	})
	c.Assert(err, jc.ErrorIsNil)

	expected := bsonMById{
		{
			"_id":        ensureModelUUID(uuid1, "app1"),
			"model-uuid": uuid1,
			"charmurl":   "cs:test",
			"charm-origin": bson.M{
				"platform": bson.M{
					"architecture": "amd64",
					"series":       "focal",
				},
			},
			"series": "focal",
		},
		{
			"_id":        ensureModelUUID(uuid2, "app2"),
			"model-uuid": uuid2,
			"charmurl":   "ch:test",
			"charm-origin": bson.M{
				"platform": bson.M{
					"architecture": "amd64",
					"series":       "focal",
				},
			},
			"series": "focal",
		},
	}

	sort.Sort(expected)
	s.assertUpgradedData(c, UpdateCharmOriginAfterSetSeries,
		upgradedData(appColl, expected),
	)
}

func (s *upgradesSuite) TestUpdateOperationWithEnqueuingErrors(c *gc.C) {
	model1 := s.makeModel(c, "model-1", coretesting.Attrs{})
	model2 := s.makeModel(c, "model-2", coretesting.Attrs{})
	defer func() {
		_ = model1.Close()
		_ = model2.Close()
	}()

	uuid1 := model1.ModelUUID()
	uuid2 := model2.ModelUUID()

	opColl, opCloser := s.state.db().GetRawCollection(operationsC)
	defer opCloser()
	setupOperationsTestUpdateOperationWithEnqueuingErrors(c, opColl, uuid1, uuid2)

	actColl, actCloser := s.state.db().GetRawCollection(actionsC)
	defer actCloser()
	setupActionsTestUpdateOperationWithEnqueuingErrors(c, actColl, uuid1, uuid2)

	expected := bsonMById{
		{
			"_id":                 ensureModelUUID(uuid1, "1"),
			"model-uuid":          uuid1,
			"summary":             "fortune run on unit-juju-qa-test-0,unit-juju-qa-test-1,unit-juju-qa-test-2,unit-juju-qa-test-3",
			"fail":                "\"unit-juju-qa-test-3\" not found",
			"complete-task-count": 0,
			"status":              "running",
			"spawned-task-count":  3,
		},
		{
			"_id":                 ensureModelUUID(uuid2, "1"),
			"model-uuid":          uuid2,
			"summary":             "fortune run on unit-juju-qa-test-3",
			"fail":                "\"unit-juju-qa-test-3\" not found",
			"status":              "error",
			"complete-task-count": 0,
			"spawned-task-count":  0,
		},
		{
			"_id":                 ensureModelUUID(uuid2, "2"),
			"model-uuid":          uuid2,
			"summary":             "fortune run on unit-juju-qa-test-3,unit-juju-qa-test-3",
			"fail":                "",
			"status":              "completed",
			"complete-task-count": 2,
			"spawned-task-count":  2,
		},
	}
	sort.Sort(expected)
	s.assertUpgradedData(c, UpdateOperationWithEnqueuingErrors,
		upgradedData(opColl, expected),
	)
}

func setupOperationsTestUpdateOperationWithEnqueuingErrors(c *gc.C, opColl *mgo.Collection, uuid1, uuid2 string) {

	docs := []bson.M{
		{ // One of N actions failed enqueuing.
			"_id":                 ensureModelUUID(uuid1, "1"),
			"model-uuid":          uuid1,
			"summary":             "fortune run on unit-juju-qa-test-0,unit-juju-qa-test-1,unit-juju-qa-test-2,unit-juju-qa-test-3",
			"status":              "error",
			"fail":                "\"unit-juju-qa-test-3\" not found",
			"complete-task-count": 0,
			"spawned-task-count":  4,
		},
		{ // All actions failed enqueuing.
			"_id":                 ensureModelUUID(uuid2, "1"),
			"model-uuid":          uuid2,
			"summary":             "fortune run on unit-juju-qa-test-3",
			"fail":                "\"unit-juju-qa-test-3\" not found",
			"status":              "error",
			"complete-task-count": 0,
			"spawned-task-count":  1,
		},
		{ // Enqueuing was successful.
			"_id":                 ensureModelUUID(uuid2, "2"),
			"model-uuid":          uuid2,
			"summary":             "fortune run on unit-juju-qa-test-3,unit-juju-qa-test-3",
			"fail":                "",
			"status":              "completed",
			"complete-task-count": 2,
			"spawned-task-count":  2,
		},
	}
	err := opColl.Insert(docs[0], docs[1], docs[2])
	c.Assert(err, jc.ErrorIsNil)
}

func setupActionsTestUpdateOperationWithEnqueuingErrors(c *gc.C, actColl *mgo.Collection, uuid1, uuid2 string) {
	docs := []bson.M{
		{
			"_id":        ensureModelUUID(uuid1, "2"),
			"model-uuid": uuid1,
			"operation":  "1",
		},
		{
			"_id":        ensureModelUUID(uuid1, "3"),
			"model-uuid": uuid1,
			"operation":  "1",
		},
		{
			"_id":        ensureModelUUID(uuid1, "4"),
			"model-uuid": uuid1,
			"operation":  "1",
		},
		{
			"_id":        ensureModelUUID(uuid2, "3"),
			"model-uuid": uuid2,
			"operation":  "2",
		},
		{
			"_id":        ensureModelUUID(uuid2, "4"),
			"model-uuid": uuid2,
			"operation":  "2",
		},
	}
	err := actColl.Insert(docs[0], docs[1], docs[2])
	c.Assert(err, jc.ErrorIsNil)
}

func (s *upgradesSuite) TestRemoveLocalCharmOriginChannels(c *gc.C) {
	model1 := s.makeModel(c, "model-1", coretesting.Attrs{})
	model2 := s.makeModel(c, "model-2", coretesting.Attrs{})
	defer func() {
		_ = model1.Close()
		_ = model2.Close()
	}()

	uuid1 := model1.ModelUUID()
	uuid2 := model2.ModelUUID()

	appColl, appCloser := s.state.db().GetRawCollection(applicationsC)
	defer appCloser()

	var err error
	err = appColl.Insert(bson.M{
		"_id":        ensureModelUUID(uuid1, "app1"),
		"name":       "app1",
		"model-uuid": uuid1,
		"charmurl":   charm.MustParseURL("cs:test").String(),
	})
	c.Assert(err, jc.ErrorIsNil)
	err = appColl.Insert(bson.M{
		"_id":        ensureModelUUID(uuid1, "app2"),
		"name":       "app2",
		"model-uuid": uuid1,
		"charmurl":   charm.MustParseURL("local:test").String(),
		"charm-origin": bson.M{
			"source":   "local",
			"type":     "charm",
			"revision": 12,
			"channel": bson.M{
				"risk": "",
			},
			"platform": bson.M{
				"architecture": "amd64",
				"series":       "focal",
			},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	err = appColl.Insert(bson.M{
		"_id":        ensureModelUUID(uuid2, "app3"),
		"name":       "app3",
		"model-uuid": uuid2,
		"charmurl":   charm.MustParseURL("local:test2").String(),
		"charm-origin": bson.M{
			"source":   "local",
			"type":     "charm",
			"id":       "local:test",
			"hash":     "",
			"revision": -1,
			"platform": bson.M{
				"architecture": "amd64",
				"series":       "focal",
			},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	err = appColl.Insert(bson.M{
		"_id":        ensureModelUUID(uuid2, "app4"),
		"name":       "app4",
		"model-uuid": uuid2,
		"charmurl":   charm.MustParseURL("cs:focal/test-13").String(),
		"cs-channel": "edge",
		"charm-origin": bson.M{
			"source":   "charm-store",
			"type":     "charm",
			"revision": 12,
			"channel": bson.M{
				"track": "latest",
				"risk":  "stable",
			},
			"platform": bson.M{
				"architecture": "amd64",
				"series":       "focal",
			},
		},
	})
	c.Assert(err, jc.ErrorIsNil)

	err = appColl.Insert(bson.M{
		"_id":        ensureModelUUID(uuid2, "app5"),
		"name":       "app5",
		"model-uuid": uuid2,
		"charmurl":   charm.MustParseURL("ch:amd64/focal/test").String(),
		"charm-origin": bson.M{
			"source":   "charm-hub",
			"type":     "charm",
			"id":       "yyyy",
			"hash":     "xxxx",
			"revision": 12,
			"channel": bson.M{
				"track": "latest",
				"risk":  "edge",
			},
			"platform": bson.M{
				"architecture": "amd64",
				"series":       "focal",
			},
		},
	})
	c.Assert(err, jc.ErrorIsNil)

	expected := bsonMById{
		{
			"_id":        ensureModelUUID(uuid1, "app1"),
			"model-uuid": uuid1,
			"name":       "app1",
			"charmurl":   "cs:test",
		},
		{
			"_id":        ensureModelUUID(uuid1, "app2"),
			"model-uuid": uuid1,
			"name":       "app2",
			"charmurl":   "local:test",
			"charm-origin": bson.M{
				"source":   "local",
				"type":     "charm",
				"revision": 12,
				"platform": bson.M{
					"architecture": "amd64",
					"series":       "focal",
				},
			},
		},
		{
			"_id":        ensureModelUUID(uuid2, "app3"),
			"model-uuid": uuid2,
			"name":       "app3",
			"charmurl":   "local:test2",
			"charm-origin": bson.M{
				"source":   "local",
				"type":     "charm",
				"id":       "local:test",
				"hash":     "",
				"revision": -1,
				"platform": bson.M{
					"architecture": "amd64",
					"series":       "focal",
				},
			},
		},
		{
			"_id":        ensureModelUUID(uuid2, "app4"),
			"model-uuid": uuid2,
			"name":       "app4",
			"charmurl":   "cs:focal/test-13",
			"cs-channel": "edge",
			"charm-origin": bson.M{
				"source":   "charm-store",
				"type":     "charm",
				"revision": 12,
				"channel": bson.M{
					"track": "latest",
					"risk":  "stable",
				},
				"platform": bson.M{
					"architecture": "amd64",
					"series":       "focal",
				},
			},
		},
		{
			"_id":        ensureModelUUID(uuid2, "app5"),
			"model-uuid": uuid2,
			"name":       "app5",
			"charmurl":   "ch:amd64/focal/test",
			"charm-origin": bson.M{
				"source":   "charm-hub",
				"type":     "charm",
				"revision": 12,
				"hash":     "xxxx",
				"id":       "yyyy",
				"channel": bson.M{
					"track": "latest",
					"risk":  "edge",
				},
				"platform": bson.M{
					"architecture": "amd64",
					"series":       "focal",
				},
			},
		},
	}

	sort.Sort(expected)
	s.assertUpgradedData(c, RemoveLocalCharmOriginChannels,
		upgradedData(appColl, expected),
	)
}

func (s *upgradesSuite) TestFixCharmhubLastPollTime(c *gc.C) {
	model1 := s.makeModel(c, "model-1", coretesting.Attrs{})
	model2 := s.makeModel(c, "model-2", coretesting.Attrs{})
	defer func() {
		_ = model1.Close()
		_ = model2.Close()
	}()

	uuid1 := model1.ModelUUID()
	uuid2 := model2.ModelUUID()

	coll, resCloser := s.state.db().GetRawCollection(resourcesC)
	defer resCloser()

	existingNow := time.Now().Round(time.Second).UTC()
	var err error
	err = coll.Insert(bson.M{
		"_id":         ensureModelUUID(uuid1, "res1"),
		"resource-id": "res1-id",
		"name":        "res1",
		"model-uuid":  uuid1,
	})
	c.Assert(err, jc.ErrorIsNil)
	err = coll.Insert(bson.M{
		"_id":                        ensureModelUUID(uuid1, "res1#charmstore"),
		"resource-id":                "res1-id",
		"name":                       "res1",
		"model-uuid":                 uuid1,
		"timestamp-when-last-polled": existingNow,
	})
	c.Assert(err, jc.ErrorIsNil)
	err = coll.Insert(bson.M{
		"_id":         ensureModelUUID(uuid1, "res2#charmstore"),
		"resource-id": "res2-id",
		"name":        "res2",
		"model-uuid":  uuid1,
	})
	c.Assert(err, jc.ErrorIsNil)
	err = coll.Insert(bson.M{
		"_id":                        ensureModelUUID(uuid2, "res3#charmstore"),
		"resource-id":                "res3-id",
		"name":                       "res3",
		"model-uuid":                 uuid2,
		"timestamp-when-last-polled": time.Time{},
	})
	c.Assert(err, jc.ErrorIsNil)
	expected := bsonMById{
		{
			"_id":         ensureModelUUID(uuid1, "res1"),
			"resource-id": "res1-id",
			"name":        "res1",
			"model-uuid":  uuid1,
		}, {
			"_id":                        ensureModelUUID(uuid1, "res1#charmstore"),
			"resource-id":                "res1-id",
			"name":                       "res1",
			"model-uuid":                 uuid1,
			"timestamp-when-last-polled": existingNow,
		}, {
			"_id":                        ensureModelUUID(uuid1, "res2#charmstore"),
			"resource-id":                "res2-id",
			"name":                       "res2",
			"model-uuid":                 uuid1,
			"timestamp-when-last-polled": model1.nowToTheSecond(),
		}, {
			"_id":                        ensureModelUUID(uuid2, "res3#charmstore"),
			"resource-id":                "res3-id",
			"name":                       "res3",
			"model-uuid":                 uuid2,
			"timestamp-when-last-polled": model2.nowToTheSecond(),
		},
	}

	sort.Sort(expected)
	s.assertUpgradedData(c, FixCharmhubLastPolltime,
		upgradedData(coll, expected),
	)
}

func (s *upgradesSuite) TestRemoveUseFloatingIPConfigFalse(c *gc.C) {
	model1 := s.makeModel(c, "model-1", coretesting.Attrs{"use-floating-ip": true})
	model2 := s.makeModel(c, "model-2", coretesting.Attrs{"use-floating-ip": false})
	model3 := s.makeModel(c, "model-3", coretesting.Attrs{})
	defer func() {
		_ = model1.Close()
		_ = model2.Close()
		_ = model3.Close()
	}()

	err := RemoveUseFloatingIPConfigFalse(s.pool)
	c.Assert(err, jc.ErrorIsNil)

	m1, _, err := s.pool.GetModel(model1.ModelUUID())
	c.Assert(err, jc.ErrorIsNil)
	cfg1, err := m1.Config()
	c.Assert(err, jc.ErrorIsNil)
	m1Val, ok := cfg1.AllAttrs()["use-floating-ip"]
	c.Assert(ok, jc.IsTrue)
	c.Assert(m1Val, jc.IsTrue)

	m2, _, err := s.pool.GetModel(model2.ModelUUID())
	c.Assert(err, jc.ErrorIsNil)
	cfg2, err := m2.Config()
	c.Assert(err, jc.ErrorIsNil)
	_, ok = cfg2.AllAttrs()["use-floating-ip"]
	c.Assert(ok, jc.IsFalse)

	m3, _, err := s.pool.GetModel(model3.ModelUUID())
	c.Assert(err, jc.ErrorIsNil)
	cfg3, err := m3.Config()
	c.Assert(err, jc.ErrorIsNil)
	_, ok = cfg3.AllAttrs()["use-floating-ip"]
	c.Assert(ok, jc.IsFalse)
}

func (s *upgradesSuite) TestCharmOriginChannelMustHaveTrack(c *gc.C) {
	model1 := s.makeModel(c, "model-1", coretesting.Attrs{})
	model2 := s.makeModel(c, "model-2", coretesting.Attrs{})
	defer func() {
		_ = model1.Close()
		_ = model2.Close()
	}()

	uuid1 := model1.ModelUUID()
	uuid2 := model2.ModelUUID()

	appColl, appCloser := s.state.db().GetRawCollection(applicationsC)
	defer appCloser()

	var err error
	err = appColl.Insert(bson.M{
		"_id":        ensureModelUUID(uuid1, "app1"),
		"name":       "app1",
		"model-uuid": uuid1,
		"charmurl":   charm.MustParseURL("cs:test").String(),
		"charm-origin": bson.M{
			"source": "charmstore",
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	err = appColl.Insert(bson.M{
		"_id":        ensureModelUUID(uuid1, "app2"),
		"name":       "app2",
		"model-uuid": uuid1,
		"charmurl":   charm.MustParseURL("ch:amd64/focal/test").String(),
		"charm-origin": bson.M{
			"source":   "charm-hub",
			"type":     "charm",
			"id":       "yyyy",
			"hash":     "xxxx",
			"revision": 12,
			"channel": bson.M{
				"track": "latest",
				"risk":  "edge",
			},
			"platform": bson.M{
				"architecture": "amd64",
				"series":       "20.04",
			},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	err = appColl.Insert(bson.M{
		"_id":        ensureModelUUID(uuid2, "app3"),
		"name":       "app3",
		"model-uuid": uuid2,
		"charmurl":   charm.MustParseURL("local:test2").String(),
		"charm-origin": bson.M{
			"source": "local",
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	err = appColl.Insert(bson.M{
		"_id":        ensureModelUUID(uuid2, "app4"),
		"name":       "app4",
		"model-uuid": uuid2,
		"charmurl":   charm.MustParseURL("ch:amd64/focal/testtwo").String(),
		"charm-origin": bson.M{
			"source":   "charm-hub",
			"type":     "charm",
			"id":       "yyyy",
			"hash":     "xxxx",
			"revision": 12,
			"channel": bson.M{
				"risk": "edge",
			},
			"platform": bson.M{
				"architecture": "amd64",
				"series":       "20.04",
			},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	err = appColl.Insert(bson.M{
		"_id":        ensureModelUUID(uuid2, "app5"),
		"name":       "app5",
		"model-uuid": uuid2,
		"charmurl":   charm.MustParseURL("ch:amd64/focal/test").String(),
		"charm-origin": bson.M{
			"source":   "charm-hub",
			"type":     "charm",
			"id":       "yyyy",
			"hash":     "xxxx",
			"revision": 12,
			"channel": bson.M{
				"track": "8.0",
				"risk":  "stable",
			},
			"platform": bson.M{
				"architecture": "amd64",
				"series":       "20.04",
			},
		},
	})
	c.Assert(err, jc.ErrorIsNil)

	expected := bsonMById{
		{
			"_id":        ensureModelUUID(uuid1, "app1"),
			"model-uuid": uuid1,
			"name":       "app1",
			"charmurl":   "cs:test",
			"charm-origin": bson.M{
				"source": "charmstore",
			},
		},
		{
			"_id":        ensureModelUUID(uuid1, "app2"),
			"model-uuid": uuid1,
			"name":       "app2",
			"charmurl":   "ch:amd64/focal/test",
			"charm-origin": bson.M{
				"source":   "charm-hub",
				"type":     "charm",
				"id":       "yyyy",
				"hash":     "xxxx",
				"revision": 12,
				"channel": bson.M{
					"track": "latest",
					"risk":  "edge",
				},
				"platform": bson.M{
					"architecture": "amd64",
					"series":       "20.04",
				},
			},
		},
		{
			"_id":        ensureModelUUID(uuid2, "app3"),
			"model-uuid": uuid2,
			"name":       "app3",
			"charmurl":   "local:test2",
			"charm-origin": bson.M{
				"source": "local",
			},
		},
		{
			"_id":        ensureModelUUID(uuid2, "app4"),
			"model-uuid": uuid2,
			"name":       "app4",
			"charmurl":   charm.MustParseURL("ch:amd64/focal/testtwo").String(),
			"charm-origin": bson.M{
				"source":   "charm-hub",
				"type":     "charm",
				"id":       "yyyy",
				"hash":     "xxxx",
				"revision": 12,
				"channel": bson.M{
					"track": "latest",
					"risk":  "edge",
				},
				"platform": bson.M{
					"architecture": "amd64",
					"series":       "20.04",
				},
			},
		},
		{
			"_id":        ensureModelUUID(uuid2, "app5"),
			"model-uuid": uuid2,
			"name":       "app5",
			"charmurl":   charm.MustParseURL("ch:amd64/focal/test").String(),
			"charm-origin": bson.M{
				"source":   "charm-hub",
				"type":     "charm",
				"id":       "yyyy",
				"hash":     "xxxx",
				"revision": 12,
				"channel": bson.M{
					"track": "8.0",
					"risk":  "stable",
				},
				"platform": bson.M{
					"architecture": "amd64",
					"series":       "20.04",
				},
			},
		},
	}

	sort.Sort(expected)
	s.assertUpgradedData(c, CharmOriginChannelMustHaveTrack,
		upgradedData(appColl, expected),
	)
}

func (s *upgradesSuite) TestRemoveDefaultSeriesFromModelConfig(c *gc.C) {
	settingsColl, settingsCloser := s.state.db().GetRawCollection(settingsC)
	defer settingsCloser()
	_, err := settingsColl.RemoveAll(nil)
	c.Assert(err, jc.ErrorIsNil)
	err = settingsColl.Insert(bson.M{
		"_id": "modelXXX:e",
		// this document should be changed
		"settings": bson.M{
			"keepme":         "have value",
			"default-series": "delete-me",
		},
	}, bson.M{
		"_id": "modelXXX:a#testing",
		// this document has no change
		"settings": bson.M{
			"default-series": "application has no default series",
		},
	}, bson.M{
		"_id": "modelXYZ:e",
		// this document has no change, is skipped
		"settings": bson.M{
			"keepme":   "have value",
			"removeme": nil,
		},
	}, bson.M{
		"_id": "modelXYZ:epool#lxd-btrfs",
		// this document has no change
		"settings": bson.M{
			"keepme":   "have value",
			"removeme": nil,
		},
	})
	c.Assert(err, jc.ErrorIsNil)

	expectedSettings := []bson.M{
		{
			"_id":      "modelXXX:a#testing",
			"settings": bson.M{"default-series": "application has no default series"},
		}, {
			"_id":      "modelXXX:e",
			"settings": bson.M{"keepme": "have value", "default-series": ""},
		}, {
			"_id":      "modelXYZ:e",
			"settings": bson.M{"keepme": "have value", "removeme": nil},
		}, {
			"_id":      "modelXYZ:epool#lxd-btrfs",
			"settings": bson.M{"keepme": "have value", "removeme": nil},
		}}

	s.assertUpgradedData(c, RemoveDefaultSeriesFromModelConfig,
		upgradedData(settingsColl, expectedSettings),
	)
}

type docById []bson.M

func (d docById) Len() int           { return len(d) }
func (d docById) Swap(i, j int)      { d[i], d[j] = d[j], d[i] }
func (d docById) Less(i, j int) bool { return d[i]["_id"].(string) < d[j]["_id"].(string) }

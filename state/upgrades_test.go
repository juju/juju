// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"sort"

	"github.com/juju/charm/v9"
	"github.com/juju/mgo/v3"
	"github.com/juju/mgo/v3/bson"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v3"
	"github.com/kr/pretty"
	gc "gopkg.in/check.v1"

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

func (s *upgradesSuite) TestCorrectCharmOriginsMultiAppSingleCharm(c *gc.C) {
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
			"source": "charm-store",
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
			"id":       "yyyy5",
			"hash":     "xxxx5",
			"revision": 12,
			"channel": bson.M{
				"track": "latest",
				"risk":  "edge",
			},
			"platform": bson.M{
				"architecture": "amd64",
				"os":           "ubuntu",
				"channel":      "20.04",
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
				"track": "latest",
				"risk":  "edge",
			},
			"platform": bson.M{
				"architecture": "amd64",
				"os":           "ubuntu",
				"channel":      "20.04",
			},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	err = appColl.Insert(bson.M{
		"_id":        ensureModelUUID(uuid2, "app5"),
		"name":       "app5",
		"model-uuid": uuid2,
		"charmurl":   charm.MustParseURL("ch:amd64/focal/testtwo").String(),
		"charm-origin": bson.M{
			"source":   "charm-hub",
			"type":     "charm",
			"id":       "",
			"hash":     "",
			"revision": 12,
			"channel": bson.M{
				"track": "8.0",
				"risk":  "stable",
			},
			"platform": bson.M{
				"architecture": "amd64",
				"os":           "ubuntu",
				"channel":      "20.04",
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
				"source": "charm-store",
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
				"id":       "yyyy5",
				"hash":     "xxxx5",
				"revision": 12,
				"channel": bson.M{
					"track": "latest",
					"risk":  "edge",
				},
				"platform": bson.M{
					"architecture": "amd64",
					"os":           "ubuntu",
					"channel":      "20.04",
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
					"os":           "ubuntu",
					"channel":      "20.04",
				},
			},
		},
		{
			"_id":        ensureModelUUID(uuid2, "app5"),
			"model-uuid": uuid2,
			"name":       "app5",
			"charmurl":   charm.MustParseURL("ch:amd64/focal/testtwo").String(),
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
					"os":           "ubuntu",
					"channel":      "20.04",
				},
			},
		},
	}

	sort.Sort(expected)
	s.assertUpgradedData(c, CorrectCharmOriginsMultiAppSingleCharm,
		upgradedData(appColl, expected),
	)
}

type docById []bson.M

func (d docById) Len() int           { return len(d) }
func (d docById) Swap(i, j int)      { d[i], d[j] = d[j], d[i] }
func (d docById) Less(i, j int) bool { return d[i]["_id"].(string) < d[j]["_id"].(string) }

// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"sort"

	"github.com/juju/charm/v9"
	"github.com/juju/mgo/v3"
	"github.com/juju/mgo/v3/bson"
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

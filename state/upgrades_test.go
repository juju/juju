// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/juju/charm/v12"
	"github.com/juju/errors"
	"github.com/juju/mgo/v3"
	"github.com/juju/mgo/v3/bson"
	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v3"
	"github.com/kr/pretty"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/secrets"
	"github.com/juju/juju/environs/config"
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

type appNameAndID struct {
	appName  string
	uniqueID string
}

func upgradedData(coll *mgo.Collection, expected []bson.M) expectUpgradedData {
	return expectUpgradedData{
		coll:     coll,
		expected: expected,
	}
}

func (s *upgradesSuite) assertUpgradedData(c *gc.C, upgrade func(*StatePool) error, check gc.Checker, expect ...expectUpgradedData) {
	if check == nil {
		check = jc.DeepEquals
	}
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
			c.Assert(docs, check, expect.expected,
				gc.Commentf("differences: %s", pretty.Diff(docs, expect.expected)))
		}
	}
}

func (s *upgradesSuite) makeModel(c *gc.C, name string, attr coretesting.Attrs, modelArgs ModelArgs) *State {
	uuid := utils.MustNewUUID()
	cfg := coretesting.CustomModelConfig(c, coretesting.Attrs{
		"name": name,
		"uuid": uuid.String(),
	}.Merge(attr))
	m, err := s.state.Model()
	c.Assert(err, jc.ErrorIsNil)
	_, st, err := s.controller.NewModel(
		defaultModelArgs(&modelArgs, cfg, m.Owner()))
	c.Assert(err, jc.ErrorIsNil)
	return st
}

func defaultModelArgs(modelArgs *ModelArgs, cfg *config.Config, owner names.UserTag) ModelArgs {
	if modelArgs == nil {
		modelArgs = &ModelArgs{}
	}
	modelArgs.Config = cfg
	modelArgs.Owner = owner

	if modelArgs.Type == "" {
		modelArgs.Type = ModelTypeIAAS
	}
	if modelArgs.CloudName == "" {
		modelArgs.CloudName = "dummy"
	}
	if modelArgs.CloudRegion == "" {
		modelArgs.CloudRegion = "dummy-region"
	}
	if modelArgs.StorageProviderRegistry == nil {
		modelArgs.StorageProviderRegistry = provider.CommonStorageProviders()
	}

	return *modelArgs
}

// makeRemoteApplication creates a remote application with a "db" provider
// endpoint, as used by the cross-model relation repair upgrade tests.
func (s *upgradesSuite) makeRemoteApplication(c *gc.C, name string) *RemoteApplication {
	app, err := s.state.AddRemoteApplication(AddRemoteApplicationParams{
		Name:        name,
		SourceModel: names.NewModelTag("source-model"),
		OfferUUID:   "offer-" + name,
		Token:       name + "-token",
		Endpoints: []charm.Relation{{
			Interface: "mysql",
			Limit:     1,
			Name:      "db",
			Role:      charm.RoleProvider,
			Scope:     charm.ScopeGlobal,
		}},
		ConsumeVersion: 1,
	})
	c.Assert(err, jc.ErrorIsNil)
	return app
}

// mkRel creates a relation between the remote application proxy and a
// fresh local application named appName, and puts a unit of the local
// application in scope (generating scope and settings docs).
func (s *upgradesSuite) mkRel(c *gc.C, ch *Charm, appName string, proxy *RemoteApplication) (*Relation, *Unit) {
	wp := AddTestingApplication(c, s.state, appName, ch)
	proxyEP, err := proxy.Endpoint("db")
	c.Assert(err, jc.ErrorIsNil)
	wpEP, err := wp.Endpoint("db")
	c.Assert(err, jc.ErrorIsNil)
	rel, err := s.state.AddRelation(proxyEP, wpEP)
	c.Assert(err, jc.ErrorIsNil)
	wpUnit, err := wp.AddUnit(AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	ru, err := rel.Unit(wpUnit)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ru.EnterScope(nil), jc.ErrorIsNil)
	return rel, wpUnit
}

func (s *upgradesSuite) relPrefix(rel *Relation) string {
	return fmt.Sprintf("r#%d#", rel.Id())
}

func (s *upgradesSuite) countDocs(c *gc.C, collName, prefix string) int {
	coll, closer, err := s.state.db().GetRawCollection(collName)
	c.Assert(err, jc.ErrorIsNil)
	defer closer()
	n, err := coll.Find(bson.M{"_id": bson.M{"$regex": "^" + s.state.docID(prefix)}}).Count()
	c.Assert(err, jc.ErrorIsNil)
	return n
}

func (s *upgradesSuite) setRemoteCount(c *gc.C, name string, count int) {
	coll, closer, err := s.state.db().GetRawCollection(remoteApplicationsC)
	c.Assert(err, jc.ErrorIsNil)
	defer closer()
	err = coll.UpdateId(s.state.docID(name), bson.M{"$set": bson.M{"relationcount": count}})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *upgradesSuite) localRelationCount(c *gc.C, name string) int {
	coll, closer, err := s.state.db().GetRawCollection(applicationsC)
	c.Assert(err, jc.ErrorIsNil)
	defer closer()
	var doc struct {
		RelationCount int `bson:"relationcount"`
	}
	err = coll.FindId(s.state.docID(name)).One(&doc)
	c.Assert(err, jc.ErrorIsNil)
	return doc.RelationCount
}

func (s *upgradesSuite) TestSplitMigrationStatusMessages(c *gc.C) {
	model := s.makeModel(c, "m", coretesting.Attrs{}, ModelArgs{Type: ModelTypeIAAS})
	defer func() { _ = model.Close() }()

	migStatus, closer, err := s.state.db().GetRawCollection(migrationsStatusC)
	c.Assert(err, jc.ErrorIsNil)
	defer closer()

	migStatusMessage, closer2, err := s.state.db().GetRawCollection(migrationsStatusMessageC)
	c.Assert(err, jc.ErrorIsNil)
	defer closer2()

	err = migStatus.Insert(bson.M{
		"_id":                ensureModelUUID(model.ModelUUID(), "0"),
		"start-time":         "1742996705546941797",
		"success-time":       "1742996716038789910",
		"end-time":           "1742996722262468965",
		"phase":              "DONE",
		"phase-changed-time": "1742996722262468965",
		"status-message":     "successful, removing model from source controller",
	})
	c.Assert(err, jc.ErrorIsNil)

	expectedStatus := []bson.M{{
		"_id":                ensureModelUUID(model.ModelUUID(), "0"),
		"start-time":         "1742996705546941797",
		"success-time":       "1742996716038789910",
		"end-time":           "1742996722262468965",
		"phase":              "DONE",
		"phase-changed-time": "1742996722262468965",
	}}
	expectedStatusMessage := []bson.M{{
		"_id":            ensureModelUUID(model.ModelUUID(), "0"),
		"status-message": "successful, removing model from source controller",
	}}

	s.assertUpgradedData(c, SplitMigrationStatusMessages, nil,
		upgradedData(migStatus, expectedStatus),
		upgradedData(migStatusMessage, expectedStatusMessage),
	)
}

func (s *upgradesSuite) TestOpenControllerAPIPort(c *gc.C) {
	m0, err := s.state.AddMachine(UbuntuBase("12.10"), JobManageModel, JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.state.EnableHA(3, constraints.Value{}, UbuntuBase("12.10"), nil)
	c.Assert(err, jc.ErrorIsNil)
	m1, err := s.state.Machine("1")
	c.Assert(err, jc.ErrorIsNil)

	controllerApp := AddTestingApplication(c, s.state, "controller", AddTestingCharm(c, s.state, "wordpress"))

	// Unit 0 has no existing ports.
	u0, err := controllerApp.AddUnit(AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	err = u0.AssignToMachine(m0)
	c.Assert(err, jc.ErrorIsNil)

	// Unit 1 has existing ports.
	u1, err := controllerApp.AddUnit(AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	err = u1.AssignToMachine(m1)
	c.Assert(err, jc.ErrorIsNil)
	pcp, err := u1.OpenedPortRanges()
	c.Assert(err, jc.ErrorIsNil)
	pcp.Open("", network.PortRange{
		FromPort: 666,
		ToPort:   666,
		Protocol: "tcp",
	})
	err = s.state.ApplyOperation(pcp.Changes())
	c.Assert(err, jc.ErrorIsNil)

	openPorts, closer, err := s.state.db().GetRawCollection(openedPortsC)
	c.Assert(err, jc.ErrorIsNil)
	defer closer()

	s.assertUpgradedData(c, OpenControllerAPIPort, nil,
		upgradedData(openPorts, []bson.M{{
			"_id":        s.state.ModelUUID() + ":" + m0.Id(),
			"model-uuid": s.state.ModelUUID(),
			"machine-id": m0.Id(),
			"unit-port-ranges": bson.M{
				"controller/0": bson.M{"": []any{bson.M{
					"protocol": "tcp",
					"fromport": 17777,
					"toport":   17777,
				}}},
			},
		}, {
			"_id":        s.state.ModelUUID() + ":" + m1.Id(),
			"model-uuid": s.state.ModelUUID(),
			"machine-id": m1.Id(),
			"unit-port-ranges": bson.M{
				"controller/1": bson.M{"": []any{
					bson.M{
						"protocol": "tcp",
						"fromport": 666,
						"toport":   666,
					}, bson.M{
						"protocol": "tcp",
						"fromport": 17777,
						"toport":   17777,
					}},
				},
			},
		}}),
	)

}

func (s *upgradesSuite) TestExposeControllerApplication(c *gc.C) {
	AddTestingApplication(c, s.state, "controller", AddTestingCharm(c, s.state, "wordpress"))

	appsColl, closer, err := s.state.db().GetRawCollection(applicationsC)
	c.Assert(err, jc.ErrorIsNil)
	defer closer()

	var appData bson.M
	err = appsColl.Find(bson.M{"_id": s.state.docID(controllerAppName)}).One(&appData)
	c.Assert(err, jc.ErrorIsNil)
	exposed, ok := appData["exposed"].(bool)
	c.Assert(ok, jc.IsTrue)
	c.Assert(exposed, jc.IsFalse)

	appData["exposed"] = true
	delete(appData, "txn-revno")
	s.assertUpgradedData(c, ExposeControllerApplication, nil,
		upgradedData(appsColl, []bson.M{appData}),
	)
}

func (s *upgradesSuite) TestExposeControllerApplicationMissing(c *gc.C) {
	err := ExposeControllerApplication(s.pool)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *upgradesSuite) TestPopulateApplicationStorageUniqueID(c *gc.C) {
	state1 := s.makeModel(c, "m1", coretesting.Attrs{}, ModelArgs{Type: ModelTypeCAAS})
	state2 := s.makeModel(c, "m2", coretesting.Attrs{}, ModelArgs{Type: ModelTypeCAAS})
	defer func() {
		_ = state1.Close()
		_ = state2.Close()
	}()

	appColl1, closer, err := state1.db().GetRawCollection(applicationsC)
	c.Assert(err, jc.ErrorIsNil)
	defer closer()

	model1, err := state1.Model()
	c.Assert(err, gc.IsNil)

	// app1: scaling=true should become current-operation="scale".
	err = appColl1.Insert(bson.M{
		"_id":        ensureModelUUID(model1.UUID(), "app1"),
		"name":       "app1",
		"model-uuid": model1.UUID(),
	})
	c.Assert(err, gc.IsNil)
	// app2: scaling=false should become current-operation="".
	err = appColl1.Insert(bson.M{
		"_id":        ensureModelUUID(model1.UUID(), "app2"),
		"name":       "app2",
		"model-uuid": model1.UUID(),
	})
	c.Assert(err, gc.IsNil)
	// app3 does not get backfilled because its storage unique ID is already
	// populated.
	// app3: empty provisioning-state should remain empty.
	err = appColl1.Insert(bson.M{
		"_id":               ensureModelUUID(model1.UUID(), "app3"),
		"name":              "app3",
		"model-uuid":        model1.UUID(),
		"storage-unique-id": "uniqueid3",
	})
	c.Assert(err, gc.IsNil)

	model2, err := state2.Model()
	c.Assert(err, gc.IsNil)

	appColl2, closer, err := state2.db().GetRawCollection(applicationsC)
	c.Assert(err, jc.ErrorIsNil)
	defer closer()

	err = appColl2.Insert(bson.M{
		"_id":        ensureModelUUID(model2.UUID(), "app4"),
		"name":       "app4",
		"model-uuid": model2.UUID(),
	})
	c.Assert(err, gc.IsNil)
	err = appColl2.Insert(bson.M{
		"_id":        ensureModelUUID(model2.UUID(), "app5"),
		"name":       "app5",
		"model-uuid": model2.UUID(),
	})
	c.Assert(err, gc.IsNil)

	appMigratedCount := 0

	getStorageUniqueID := func(
		ctx context.Context,
		apps []AppAndStorageID,
		model *Model,
	) ([]AppAndStorageID, error) {
		fakeK8s := map[string][]appNameAndID{
			model1.UUID(): {
				{
					appName:  "app1",
					uniqueID: "uniqueid1",
				},
				{
					appName:  "app2",
					uniqueID: "uniqueid2",
				},
				// In practice, the unique ID in the annotation and in the document
				// are always consistent. For testing purposes, I’ve made them differ
				// to verify that app3 does not get backfilled, since the document
				// already contains the unique ID.
				{
					appName:  "app3",
					uniqueID: "uniqueid3-not-backfilled",
				},
			},
			model2.UUID(): {
				{
					appName:  "app4",
					uniqueID: "uniqueid4",
				},
				{
					appName:  "app5",
					uniqueID: "uniqueid5",
				},
			},
		}

		appsAndStorageIDs := make([]AppAndStorageID, 0, len(apps))
		k8sDeployment, ok := fakeK8s[model.UUID()]
		if !ok {
			return nil, errors.Errorf("unknown model %q", model.UUID())
		}
		for _, app := range apps {
			index := slices.IndexFunc(k8sDeployment, func(a appNameAndID) bool {
				return a.appName == app.Name
			})
			if index == -1 {
				c.Fatalf("app %q not found, this should not happen", app.Name)
			}

			appMigratedCount++
			appsAndStorageIDs = append(appsAndStorageIDs, AppAndStorageID{
				Id:              app.Id,
				Name:            app.Name,
				StorageUniqueID: k8sDeployment[index].uniqueID,
			})
		}

		return appsAndStorageIDs, nil
	}

	err = PopulateApplicationStorageUniqueID(s.pool, getStorageUniqueID)
	c.Assert(err, gc.IsNil)
	c.Assert(appMigratedCount, gc.Equals, 4)

	// Assert the values of storage unique ID in DB.
	app1 := bson.M{}
	err = appColl1.Find(bson.M{"name": "app1"}).One(&app1)
	c.Assert(err, gc.IsNil)
	c.Assert(app1["storage-unique-id"], gc.Equals, "uniqueid1")

	app2 := bson.M{}
	err = appColl1.Find(bson.M{"name": "app2"}).One(&app2)
	c.Assert(err, gc.IsNil)
	c.Assert(app2["storage-unique-id"], gc.Equals, "uniqueid2")

	app3 := bson.M{}
	err = appColl1.Find(bson.M{"name": "app3"}).One(&app3)
	c.Assert(err, gc.IsNil)
	c.Assert(app3["storage-unique-id"], gc.Equals, "uniqueid3")

	app4 := bson.M{}
	err = appColl2.Find(bson.M{"name": "app4"}).One(&app4)
	c.Assert(err, gc.IsNil)
	c.Assert(app4["storage-unique-id"], gc.Equals, "uniqueid4")

	app5 := bson.M{}
	err = appColl2.Find(bson.M{"name": "app5"}).One(&app5)
	c.Assert(err, gc.IsNil)
	c.Assert(app5["storage-unique-id"], gc.Equals, "uniqueid5")
}

func (s *upgradesSuite) TestConvertScalingToCurrentOperationEnumField(c *gc.C) {
	state1 := s.makeModel(c, "m1", coretesting.Attrs{}, ModelArgs{Type: ModelTypeCAAS})
	state2 := s.makeModel(c, "m2", coretesting.Attrs{}, ModelArgs{Type: ModelTypeIAAS})
	defer func() {
		_ = state1.Close()
		_ = state2.Close()
	}()

	// Insert apps to model1 (CAAS model)
	appColl1, closer, err := state1.db().GetRawCollection(applicationsC)
	c.Assert(err, jc.ErrorIsNil)
	defer closer()

	model1, err := state1.Model()
	c.Assert(err, jc.ErrorIsNil)

	// app1: scaling=true should become current-operation="scale".
	err = appColl1.Insert(bson.M{
		"_id":        ensureModelUUID(model1.UUID(), "app1"),
		"name":       "app1",
		"model-uuid": model1.UUID(),
		"provisioning-state": bson.M{
			"scaling":      true,
			"scale-target": 3,
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	// app2: scaling=false should become current-operation="".
	err = appColl1.Insert(bson.M{
		"_id":        ensureModelUUID(model1.UUID(), "app2"),
		"name":       "app2",
		"model-uuid": model1.UUID(),
		"provisioning-state": bson.M{
			"scaling": false,
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	// app3: empty provisioning-state should remain empty.
	err = appColl1.Insert(bson.M{
		"_id":                ensureModelUUID(model1.UUID(), "app3"),
		"name":               "app3",
		"model-uuid":         model1.UUID(),
		"provisioning-state": bson.M{},
	})
	c.Assert(err, jc.ErrorIsNil)
	// app4: missing provisioning-state should remain missing.
	err = appColl1.Insert(bson.M{
		"_id":        ensureModelUUID(model1.UUID(), "app4"),
		"name":       "app4",
		"model-uuid": model1.UUID(),
	})
	c.Assert(err, jc.ErrorIsNil)
	// app5: provisioning-state explicitly null should remain null.
	err = appColl1.Insert(bson.M{
		"_id":                ensureModelUUID(model1.UUID(), "app5"),
		"name":               "app5",
		"model-uuid":         model1.UUID(),
		"provisioning-state": nil,
	})
	c.Assert(err, jc.ErrorIsNil)
	// app6: scaling explicitly null should be unset, leaving empty provisioning-state.
	err = appColl1.Insert(bson.M{
		"_id":        ensureModelUUID(model1.UUID(), "app6"),
		"name":       "app6",
		"model-uuid": model1.UUID(),
		"provisioning-state": bson.M{
			"scaling": nil,
		},
	})
	c.Assert(err, jc.ErrorIsNil)

	appColl2, closer, err := state2.db().GetRawCollection(applicationsC)
	c.Assert(err, jc.ErrorIsNil)
	defer closer()

	// Insert apps to model2 (IAAS model)
	model2, err := state2.Model()
	c.Assert(err, jc.ErrorIsNil)

	// app7: IAAS model app should be untouched by this CAAS-only upgrade.
	err = appColl2.Insert(bson.M{
		"_id":        ensureModelUUID(model2.UUID(), "app7"),
		"name":       "app7",
		"model-uuid": model2.UUID(),
	})
	c.Assert(err, jc.ErrorIsNil)

	s.assertUpgradedData(c, ConvertScalingToCurrentOperationEnumField, nil,
		expectUpgradedData{
			coll: appColl1,
			filter: bson.D{
				{"model-uuid", model1.UUID()},
			},
			expected: []bson.M{
				{
					"_id":        ensureModelUUID(model1.UUID(), "app1"),
					"name":       "app1",
					"model-uuid": model1.UUID(),
					"provisioning-state": bson.M{
						"current-operation": "scale",
						"scale-target":      3,
					},
				},
				{
					"_id":                ensureModelUUID(model1.UUID(), "app2"),
					"name":               "app2",
					"model-uuid":         model1.UUID(),
					"provisioning-state": bson.M{},
				},
				{
					"_id":                ensureModelUUID(model1.UUID(), "app3"),
					"name":               "app3",
					"model-uuid":         model1.UUID(),
					"provisioning-state": bson.M{},
				},
				{
					"_id":        ensureModelUUID(model1.UUID(), "app4"),
					"name":       "app4",
					"model-uuid": model1.UUID(),
				},
				{
					"_id":                ensureModelUUID(model1.UUID(), "app5"),
					"name":               "app5",
					"model-uuid":         model1.UUID(),
					"provisioning-state": nil,
				},
				{
					"_id":                ensureModelUUID(model1.UUID(), "app6"),
					"name":               "app6",
					"model-uuid":         model1.UUID(),
					"provisioning-state": bson.M{},
				},
			},
		},
		expectUpgradedData{
			coll: appColl2,
			filter: bson.D{
				{"model-uuid", model2.UUID()},
			},
			expected: []bson.M{
				{
					"_id":        ensureModelUUID(model2.UUID(), "app7"),
					"name":       "app7",
					"model-uuid": model2.UUID(),
				},
			},
		},
	)
}

func (s *upgradesSuite) TestRemoveSSHProxyArtefactsDropsCollections(c *gc.C) {
	// The collections are model-scoped: documents share the underlying
	// MongoDB collection and are distinguished by a "model-uuid" field
	// (and model-UUID-prefixed _id). Create a second model and seed both
	// it and the controller model with a doc in each collection, then
	// assert the upgrade removes only the target model's documents and
	// leaves the other model's documents intact. Dropping the whole
	// collection would fail this test.
	state1 := s.makeModel(c, "m1", coretesting.Attrs{},
		ModelArgs{Type: ModelTypeIAAS})
	defer func() { _ = state1.Close() }()

	seedCollection := func(st *State, collection, docID string) {
		coll, closer, err := st.db().GetRawCollection(collection)
		c.Assert(err, jc.ErrorIsNil)
		defer closer()
		err = coll.Insert(bson.M{
			"_id":        st.docID(docID),
			"model-uuid": st.ModelUUID(),
			"data":       "unused",
		})
		c.Assert(err, jc.ErrorIsNil)
	}

	seedCollection(s.state, "virtualhostkeys", "machine-0-hostkey")
	seedCollection(state1, "virtualhostkeys", "machine-1-hostkey")
	seedCollection(s.state, "sshrequests", "conn-0")
	seedCollection(state1, "sshrequests", "conn-1")

	modelDocCount := func(st *State, name string) int {
		coll, closer, err := st.db().GetRawCollection(name)
		c.Assert(err, jc.ErrorIsNil)
		defer closer()
		n, err := coll.Find(bson.D{{Name: "model-uuid", Value: st.ModelUUID()}}).Count()
		c.Assert(err, jc.ErrorIsNil)
		return n
	}

	// Total document count across both models, to ensure the underlying
	// collection is not dropped (which would remove everything at once).
	totalDocCount := func(name string) int {
		coll, closer, err := s.state.db().GetRawCollection(name)
		c.Assert(err, jc.ErrorIsNil)
		defer closer()
		n, err := coll.Count()
		c.Assert(err, jc.ErrorIsNil)
		return n
	}

	c.Check(modelDocCount(s.state, "virtualhostkeys"), gc.Equals, 1)
	c.Check(modelDocCount(state1, "virtualhostkeys"), gc.Equals, 1)
	c.Check(modelDocCount(s.state, "sshrequests"), gc.Equals, 1)
	c.Check(modelDocCount(state1, "sshrequests"), gc.Equals, 1)

	// Two rounds to check idempotency: removing already-absent documents
	// must not error.
	for i := 0; i < 2; i++ {
		c.Logf("Run: %d", i)
		err := RemoveSSHProxyArtefacts(s.pool)
		c.Assert(err, jc.ErrorIsNil)

		c.Check(modelDocCount(s.state, "virtualhostkeys"), gc.Equals, 0)
		c.Check(modelDocCount(state1, "virtualhostkeys"), gc.Equals, 0)
		c.Check(modelDocCount(s.state, "sshrequests"), gc.Equals, 0)
		c.Check(modelDocCount(state1, "sshrequests"), gc.Equals, 0)
	}

	// The underlying collections may still exist (now empty); what
	// matters is that no ssh proxy documents remain for any model.
	c.Check(totalDocCount("virtualhostkeys"), gc.Equals, 0)
	c.Check(totalDocCount("sshrequests"), gc.Equals, 0)
}

func (s *upgradesSuite) TestRemoveSSHProxyArtefactsRemovesCleanupDocs(c *gc.C) {
	// Seed a leftover "sshConnRequests" cleanup document. The cleanup kind
	// was removed along with the rest of the feature, so the upgrade step
	// must remove any such documents or the cleanup worker would fail
	// trying to run a handler that no longer exists.
	//
	// The cleanups collection is model-scoped, so the seeded docs must
	// include the model-uuid field to be visible to the model-filtered
	// query used by removeSSHProxyCleanupDocs.
	coll, closer, err := s.state.db().GetRawCollection(cleanupsC)
	c.Assert(err, jc.ErrorIsNil)
	err = coll.Insert(bson.M{
		"_id":        s.state.docID("ssh-conn-cleanup-0"),
		"model-uuid": s.state.ModelUUID(),
		"kind":       "sshConnRequests",
		"prefix":     "some-prefix",
	})
	c.Assert(err, jc.ErrorIsNil)
	closer()

	// Also seed an unrelated cleanup doc to ensure only the ssh proxy kind
	// is removed.
	coll, closer, err = s.state.db().GetRawCollection(cleanupsC)
	c.Assert(err, jc.ErrorIsNil)
	err = coll.Insert(bson.M{
		"_id":        s.state.docID("other-cleanup-0"),
		"model-uuid": s.state.ModelUUID(),
		"kind":       "settings",
		"prefix":     "other-prefix",
	})
	c.Assert(err, jc.ErrorIsNil)
	closer()

	cleanupCount := func(kind string) int {
		coll, closer, err := s.state.db().GetRawCollection(cleanupsC)
		c.Assert(err, jc.ErrorIsNil)
		defer closer()
		n, err := coll.Find(bson.D{
			{Name: "model-uuid", Value: s.state.ModelUUID()},
			{Name: "kind", Value: kind},
		}).Count()
		c.Assert(err, jc.ErrorIsNil)
		return n
	}

	c.Check(cleanupCount("sshConnRequests"), gc.Equals, 1)
	c.Check(cleanupCount("settings"), gc.Equals, 1)

	// Idempotent: a second run finds nothing to remove but must not error.
	for i := 0; i < 2; i++ {
		c.Logf("Run: %d", i)
		err := RemoveSSHProxyArtefacts(s.pool)
		c.Assert(err, jc.ErrorIsNil)

		c.Check(cleanupCount("sshConnRequests"), gc.Equals, 0)
		c.Check(cleanupCount("settings"), gc.Equals, 1)
	}
}

func (s *upgradesSuite) TestRemoveSSHProxyArtefactsRemovesControllerConfig(c *gc.C) {
	// Seed the controller config with the orphaned ssh proxy keys. They are
	// written directly to the settings document because they are no longer
	// part of the controller config schema and so cannot be set through
	// UpdateControllerConfig; this mirrors the state left behind on a
	// controller upgraded from a version that had the feature.
	settings, err := readSettings(s.state.db(), controllersC, ControllerSettingsGlobalKey)
	c.Assert(err, jc.ErrorIsNil)
	settings.Update(map[string]interface{}{
		"ssh-server-port":                17022,
		"ssh-max-concurrent-connections": 100,
	})
	_, ops := settings.settingsUpdateOps()
	c.Assert(s.state.db().RunTransaction(ops), jc.ErrorIsNil)

	cfg, err := s.state.ControllerConfig()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(cfg["ssh-server-port"], gc.Equals, 17022)
	c.Check(cfg["ssh-max-concurrent-connections"], gc.Equals, 100)

	// Idempotent: a second run finds nothing to remove but must not error.
	for i := 0; i < 2; i++ {
		c.Logf("Run: %d", i)
		err := RemoveSSHProxyArtefacts(s.pool)
		c.Assert(err, jc.ErrorIsNil)

		cfg, err := s.state.ControllerConfig()
		c.Assert(err, jc.ErrorIsNil)
		c.Check(cfg["ssh-server-port"], gc.IsNil)
		c.Check(cfg["ssh-max-concurrent-connections"], gc.IsNil)
	}
}

func (s *upgradesSuite) TestRemoveSSHProxyArtefactsClosesControllerPort(c *gc.C) {
	// The removed enableHA path opened port 17022 (the ssh server port) on
	// every controller unit. After the feature was removed no code closes
	// it, so the firewaller keeps the security-group entry open. The upgrade
	// step must close the port range on each controller unit so the
	// firewaller reverts it.
	m0, err := s.state.AddMachine(UbuntuBase("12.10"), JobManageModel, JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)

	controllerApp := AddTestingApplication(c, s.state, "controller", AddTestingCharm(c, s.state, "wordpress"))
	u0, err := controllerApp.AddUnit(AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	err = u0.AssignToMachine(m0)
	c.Assert(err, jc.ErrorIsNil)

	// Open the ssh server port on the controller unit, mirroring what the
	// removed enableHA path did. Also open an unrelated port to ensure only
	// the ssh port is closed.
	pcp, err := u0.OpenedPortRanges()
	c.Assert(err, jc.ErrorIsNil)
	pcp.Open("", network.PortRange{
		FromPort: sshProxyServerPort,
		ToPort:   sshProxyServerPort,
		Protocol: "tcp",
	})
	pcp.Open("", network.PortRange{
		FromPort: 9999,
		ToPort:   9999,
		Protocol: "tcp",
	})
	err = s.state.ApplyOperation(pcp.Changes())
	c.Assert(err, jc.ErrorIsNil)

	portOpen := func(unitName string, port int) bool {
		u, err := s.state.Unit(unitName)
		c.Assert(err, jc.ErrorIsNil)
		ranges, err := u.OpenedPortRanges()
		c.Assert(err, jc.ErrorIsNil)
		for _, pr := range ranges.UniquePortRanges() {
			if pr.Protocol == "tcp" && port >= pr.FromPort && port <= pr.ToPort {
				return true
			}
		}
		return false
	}

	c.Check(portOpen("controller/0", sshProxyServerPort), jc.IsTrue)
	c.Check(portOpen("controller/0", 9999), jc.IsTrue)

	// Idempotent: a second run finds nothing to close but must not error.
	for i := 0; i < 2; i++ {
		c.Logf("Run: %d", i)
		err := RemoveSSHProxyArtefacts(s.pool)
		c.Assert(err, jc.ErrorIsNil)

		c.Check(portOpen("controller/0", sshProxyServerPort), jc.IsFalse)
		c.Check(portOpen("controller/0", 9999), jc.IsTrue)
	}
}

func (s *upgradesSuite) TestFixApplicationCounts(c *gc.C) {
	// negative count, no relations -> must be set to 0.
	clamp := s.makeRemoteApplication(c, "clamp")
	s.setRemoteCount(c, clamp.Name(), -1)
	// live: intact relation -> must be left completely alone.
	live := s.makeRemoteApplication(c, "live")
	ch := AddTestingCharm(c, s.state, "wordpress")
	liveRel, _ := s.mkRel(c, ch, "wp-live", live)
	_ = liveRel // keeps the relation alive with scope/settings
	// orphan: relation document dropped -> the app's count must be
	// corrected to the actual (zero) relations. Stranded scopes/settings
	// are removed by RemoveOrphanedRelationDocs, not by this step.
	orphan := s.makeRemoteApplication(c, "orphan")
	orphanRel, _ := s.mkRel(c, ch, "wp-orphan", orphan)
	relColl, closer, err := s.state.db().GetRawCollection("relations")
	c.Assert(err, jc.ErrorIsNil)
	err = relColl.RemoveId(s.state.docID(orphanRel.Tag().Id()))
	closer()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.countDocs(c, "relationscopes", s.relPrefix(orphanRel)), gc.Equals, 1)

	// Run twice to verify idempotency.
	for i := 0; i < 2; i++ {
		err := FixApplicationCounts(s.pool)
		c.Assert(err, jc.ErrorIsNil)
	}

	// negative -> 0.
	coll, closer, err := s.state.db().GetRawCollection("remoteApplications")
	c.Assert(err, jc.ErrorIsNil)
	var doc struct {
		RelationCount int `bson:"relationcount"`
	}
	err = coll.FindId(s.state.docID("clamp")).One(&doc)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(doc.RelationCount, gc.Equals, 0)
	closer()

	// live relation untouched (scope and settings still present, count 1).
	_, err = s.state.Relation(liveRel.Id())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.countDocs(c, "relationscopes", s.relPrefix(liveRel)), gc.Equals, 1)
	c.Assert(s.countDocs(c, "settings", s.relPrefix(liveRel)), gc.Equals, 3)

	// The orphan app's recorded count is corrected to the actual (zero)
	// relations. Its stranded scope is still present: removing it is the
	// job of RemoveOrphanedRelationDocs.
	c.Assert(s.countDocs(c, "relationscopes", s.relPrefix(orphanRel)), gc.Equals, 1)
	orphanDoc := struct {
		RelationCount int `bson:"relationcount"`
	}{}
	orphanColl, orphanCloser, err := s.state.db().GetRawCollection("remoteApplications")
	c.Assert(err, jc.ErrorIsNil)
	err = orphanColl.FindId(s.state.docID(orphan.Name())).One(&orphanDoc)
	orphanCloser()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(orphanDoc.RelationCount, gc.Equals, 0)

	c.Assert(s.localRelationCount(c, "wp-orphan"), gc.Equals, 0)
	c.Assert(s.localRelationCount(c, "wp-live"), gc.Equals, 1)
}

func (s *upgradesSuite) TestRemoveOrphanedRelationDocs(c *gc.C) {
	// live: intact relation -> scope/settings must be left alone.
	live := s.makeRemoteApplication(c, "live")
	ch := AddTestingCharm(c, s.state, "wordpress")
	liveRel, _ := s.mkRel(c, ch, "wp-live", live)
	_ = liveRel
	// orphan: relation document dropped, scopes/settings orphaned ->
	// must be removed.
	orphan := s.makeRemoteApplication(c, "orphan")
	orphanRel, _ := s.mkRel(c, ch, "wp-orphan", orphan)
	relColl, closer, err := s.state.db().GetRawCollection("relations")
	c.Assert(err, jc.ErrorIsNil)
	err = relColl.RemoveId(s.state.docID(orphanRel.Tag().Id()))
	closer()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.countDocs(c, "relationscopes", s.relPrefix(orphanRel)), gc.Equals, 1)
	c.Assert(s.countDocs(c, "settings", s.relPrefix(orphanRel)), gc.Equals, 3)

	// Run twice to verify idempotency.
	for i := 0; i < 2; i++ {
		err := RemoveOrphanedRelationDocs(s.pool)
		c.Assert(err, jc.ErrorIsNil)
	}

	// live relation untouched (scope and settings still present).
	_, err = s.state.Relation(liveRel.Id())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.countDocs(c, "relationscopes", s.relPrefix(liveRel)), gc.Equals, 1)
	c.Assert(s.countDocs(c, "settings", s.relPrefix(liveRel)), gc.Equals, 3)

	// orphan docs removed.
	c.Assert(s.countDocs(c, "relationscopes", s.relPrefix(orphanRel)), gc.Equals, 0)
	c.Assert(s.countDocs(c, "settings", s.relPrefix(orphanRel)), gc.Equals, 0)

	// A second model with a live relation must be left untouched, and its
	// own orphaned scopes must be cleaned - proving per-model isolation.
	state1 := s.makeModel(c, "m1", coretesting.Attrs{}, ModelArgs{Type: ModelTypeIAAS})
	defer func() { _ = state1.Close() }()
	app1, err := state1.AddRemoteApplication(AddRemoteApplicationParams{
		Name:        "other",
		SourceModel: names.NewModelTag("source-model"),
		OfferUUID:   "offer-other",
		Endpoints: []charm.Relation{{
			Interface: "mysql",
			Limit:     1,
			Name:      "db",
			Role:      charm.RoleProvider,
			Scope:     charm.ScopeGlobal,
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	wp1 := AddTestingApplication(c, state1, "wp", AddTestingCharm(c, state1, "wordpress"))
	proxyEP, err := app1.Endpoint("db")
	c.Assert(err, jc.ErrorIsNil)
	wpEP, err := wp1.Endpoint("db")
	c.Assert(err, jc.ErrorIsNil)
	rel1, err := state1.AddRelation(proxyEP, wpEP)
	c.Assert(err, jc.ErrorIsNil)
	wp1Unit, err := wp1.AddUnit(AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	ru1, err := rel1.Unit(wp1Unit)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ru1.EnterScope(nil), jc.ErrorIsNil)

	// Seed an orphan scope in the second model only (relations drop).
	relColl1, closer1, err := state1.db().GetRawCollection("relations")
	c.Assert(err, jc.ErrorIsNil)
	err = relColl1.RemoveId(state1.docID(rel1.Tag().Id()))
	closer1()
	c.Assert(err, jc.ErrorIsNil)

	err = RemoveOrphanedRelationDocs(s.pool)
	c.Assert(err, jc.ErrorIsNil)

	// Model 2's orphan scope was cleaned...
	scopes1, closer1b, err := state1.db().GetRawCollection("relationscopes")
	c.Assert(err, jc.ErrorIsNil)
	n, err := scopes1.Find(bson.M{"_id": bson.M{"$regex": "^" + state1.docID(fmt.Sprintf("r#%d#", rel1.Id()))}}).Count()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(n, gc.Equals, 0)
	closer1b()

	// ...and model 1 (the primary) still has its live relation intact.
	_, err = s.state.Relation(liveRel.Id())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.countDocs(c, "relationscopes", s.relPrefix(liveRel)), gc.Equals, 1)
}

func (s *upgradesSuite) TestFixApplicationCountsNoRecurrence(c *gc.C) {
	app, err := s.state.AddRemoteApplication(AddRemoteApplicationParams{
		Name:        "cycle",
		SourceModel: names.NewModelTag("source-model"),
		OfferUUID:   "offer-cycle",
		Endpoints: []charm.Relation{{
			Interface: "mysql",
			Limit:     1,
			Name:      "db",
			Role:      charm.RoleProvider,
			Scope:     charm.ScopeGlobal,
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	wp := AddTestingApplication(c, s.state, "wp", AddTestingCharm(c, s.state, "wordpress"))
	proxyEP, err := app.Endpoint("db")
	c.Assert(err, jc.ErrorIsNil)
	wpEP, err := wp.Endpoint("db")
	c.Assert(err, jc.ErrorIsNil)
	rel, err := s.state.AddRelation(proxyEP, wpEP)
	c.Assert(err, jc.ErrorIsNil)
	wpUnit, err := wp.AddUnit(AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	wpru, err := rel.Unit(wpUnit)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(wpru.EnterScope(nil), jc.ErrorIsNil)

	// A no-op on this healthy state.
	c.Assert(FixApplicationCounts(s.pool), jc.ErrorIsNil)

	// Tear the relation down then leave the last
	// unit. The consumer proxy is removed on its final relation.
	err = rel.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	err = rel.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	err = wpru.LeaveScope()
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.state.Relation(rel.Id())
	c.Assert(err, jc.Satisfies, errors.IsNotFound)

	// Re-run the upgrade: must be a clean no-op, no orphans, no negatives.
	c.Assert(FixApplicationCounts(s.pool), jc.ErrorIsNil)
	scopes, closer, err := s.state.db().GetRawCollection("relationscopes")
	c.Assert(err, jc.ErrorIsNil)
	n, err := scopes.Find(bson.M{"_id": bson.M{"$regex": "^" + s.state.docID("r#")}}).Count()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(n, gc.Equals, 0)
	closer()
}

func (s *upgradesSuite) TestRemoveOrphanedApplicationRelations(c *gc.C) {
	rapp, err := s.state.AddRemoteApplication(AddRemoteApplicationParams{
		Name:        "remote-dangling",
		SourceModel: names.NewModelTag("source-model"),
		OfferUUID:   "offer-dangling",
		Endpoints: []charm.Relation{{
			Interface: "mysql",
			Limit:     1,
			Name:      "db",
			Role:      charm.RoleRequirer,
			Scope:     charm.ScopeGlobal,
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	remoteEP, err := rapp.Endpoint("db")
	c.Assert(err, jc.ErrorIsNil)
	ch := AddTestingCharm(c, s.state, "mysql")
	mysql := AddTestingApplication(c, s.state, "mysql", ch)
	mysqlEP, err := mysql.Endpoint("server")
	c.Assert(err, jc.ErrorIsNil)
	rel, err := s.state.AddRelation(remoteEP, mysqlEP)
	c.Assert(err, jc.ErrorIsNil)
	mysqlUnit, err := mysql.AddUnit(AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	ru, err := rel.Unit(mysqlUnit)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ru.EnterScope(nil), jc.ErrorIsNil)

	// Persist unit relation state for the relation which will be broken.
	us := NewUnitState()
	us.SetRelationState(map[int]string{rel.Id(): "checkpoint"})
	err = mysqlUnit.SetState(us, UnitStateSizeLimits{})
	c.Assert(err, jc.ErrorIsNil)

	// The remote application document disappears while
	// the relation stays alive.
	appsColl, appsCloser, err := s.state.db().GetRawCollection("remoteApplications")
	c.Assert(err, jc.ErrorIsNil)
	err = appsColl.RemoveId(s.state.docID("remote-dangling"))
	appsCloser()
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(RemoveOrphanedApplicationRelations(s.pool), jc.ErrorIsNil)

	// With a unit in scope the relation is torn down via the deferred
	// force cleanup; run the cleanups to complete it (twice: the relation
	// removal queues further scope/settings cleanups).
	noopDeleter := func(*secrets.URI, int) error { return nil }
	for i := 0; i < 2; i++ {
		c.Assert(s.state.Cleanup(noopDeleter), jc.ErrorIsNil)
	}

	// The relation and its satellite documents are gone.
	_, err = s.state.Relation(rel.Id())
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	prefix := fmt.Sprintf("r#%d#", rel.Id())
	for _, collName := range []string{"relationscopes", "settings"} {
		coll, docCloser, err := s.state.db().GetRawCollection(collName)
		c.Assert(err, jc.ErrorIsNil)
		n, err := coll.Find(bson.M{
			"_id": bson.M{"$regex": "^" + s.state.docID(prefix)},
		}).Count()
		docCloser()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(n, gc.Equals, 0)
	}

	// The local application's relation count reflects the removal.
	appColl, appCloser, err := s.state.db().GetRawCollection("applications")
	c.Assert(err, jc.ErrorIsNil)
	var appDoc struct {
		RelationCount int `bson:"relationcount"`
	}
	err = appColl.FindId(s.state.docID("mysql")).One(&appDoc)
	appCloser()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(appDoc.RelationCount, gc.Equals, 0)

	// The persisted unit relation state for the destroyed relation is
	// cleared too, so the deferred force cleanup cannot recreate the
	// stale reference after the upgrade completes.
	uState, err := mysqlUnit.State()
	c.Assert(err, jc.ErrorIsNil)
	rst, found := uState.RelationState()
	c.Assert(found, jc.IsTrue)
	c.Assert(rst, gc.HasLen, 0)
}

func (s *upgradesSuite) TestRemoveOrphanedApplicationRelationsMalformedName(c *gc.C) {
	rapp, err := s.state.AddRemoteApplication(AddRemoteApplicationParams{
		Name:        "remote-malformed",
		SourceModel: names.NewModelTag("source-model"),
		OfferUUID:   "offer-malformed",
		Endpoints: []charm.Relation{{
			Interface: "mysql",
			Limit:     1,
			Name:      "db",
			Role:      charm.RoleRequirer,
			Scope:     charm.ScopeGlobal,
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	remoteEP, err := rapp.Endpoint("db")
	c.Assert(err, jc.ErrorIsNil)
	ch := AddTestingCharm(c, s.state, "mysql")
	mysql := AddTestingApplication(c, s.state, "mysql", ch)
	mysqlEP, err := mysql.Endpoint("server")
	c.Assert(err, jc.ErrorIsNil)
	rel, err := s.state.AddRelation(remoteEP, mysqlEP)
	c.Assert(err, jc.ErrorIsNil)

	// Corrupt one endpoint's application name into something that can
	// never be valid, directly in the DB.
	s.corruptEndpointAppName(c, rel, "mysql", "bad name!")

	// The step must remove the relation rather than error out.
	err = RemoveOrphanedApplicationRelations(s.pool)
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.state.Relation(rel.Id())
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *upgradesSuite) TestRemoveOrphanedApplicationRelationsDestroyFailed(c *gc.C) {
	rapp, err := s.state.AddRemoteApplication(AddRemoteApplicationParams{
		Name:        "remote-missing",
		SourceModel: names.NewModelTag("source-model"),
		OfferUUID:   "offer-missing",
		Endpoints: []charm.Relation{{
			Interface: "mysql",
			Limit:     1,
			Name:      "db",
			Role:      charm.RoleRequirer,
			Scope:     charm.ScopeGlobal,
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	remoteEP, err := rapp.Endpoint("db")
	c.Assert(err, jc.ErrorIsNil)
	ch := AddTestingCharm(c, s.state, "mysql")
	mysql := AddTestingApplication(c, s.state, "mysql", ch)
	mysqlEP, err := mysql.Endpoint("server")
	c.Assert(err, jc.ErrorIsNil)
	mysqlUnit, err := mysql.AddUnit(AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	rel, err := s.state.AddRelation(remoteEP, mysqlEP)
	c.Assert(err, jc.ErrorIsNil)

	// Force remove relation op to fail.
	appsColl, appsCloser, err := s.state.db().GetRawCollection("remoteApplications")
	c.Assert(err, jc.ErrorIsNil)
	err = appsColl.RemoveId(s.state.docID("remote-missing"))
	appsCloser()
	c.Assert(err, jc.ErrorIsNil)
	appColl, appCloser, err := s.state.db().GetRawCollection("applications")
	c.Assert(err, jc.ErrorIsNil)
	err = appColl.UpdateId(s.state.docID("mysql"), bson.M{"$set": bson.M{"relationcount": 0}})
	appCloser()
	c.Assert(err, jc.ErrorIsNil)

	// Persist unit relation state referencing the (still live) relation.
	unitStatesColl, closer, err := s.state.db().GetRawCollection("unitstates")
	c.Assert(err, jc.ErrorIsNil)
	err = unitStatesColl.Insert(bson.M{
		"_id":            s.state.docID(mysqlUnit.globalKey()),
		"model-uuid":     s.state.ModelUUID(),
		"relation-state": bson.M{strconv.Itoa(rel.Id()): "checkpoint"},
	})
	closer()
	c.Assert(err, jc.ErrorIsNil)

	// The step succeeds without clearing the relation's unit state; the
	// relation is still Alive and left for manual repair.
	err = RemoveOrphanedApplicationRelations(s.pool)
	c.Assert(err, jc.ErrorIsNil)
	cur, err := s.state.Relation(rel.Id())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cur.Life(), gc.Equals, Alive)
	uState, err := mysqlUnit.State()
	c.Assert(err, jc.ErrorIsNil)
	rst, found := uState.RelationState()
	c.Assert(found, jc.IsTrue)
	c.Assert(rst, gc.DeepEquals, map[int]string{
		rel.Id(): "checkpoint",
	})
}

func (s *upgradesSuite) TestRemoveOrphanedApplicationRelationsChildRecords(c *gc.C) {
	rapp, err := s.state.AddRemoteApplication(AddRemoteApplicationParams{
		Name:        "remote-dangling",
		SourceModel: names.NewModelTag("source-model"),
		OfferUUID:   "offer-dangling",
		Endpoints: []charm.Relation{{
			Interface: "mysql",
			Limit:     1,
			Name:      "db",
			Role:      charm.RoleRequirer,
			Scope:     charm.ScopeGlobal,
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	remoteEP, err := rapp.Endpoint("db")
	c.Assert(err, jc.ErrorIsNil)
	ch := AddTestingCharm(c, s.state, "mysql")
	mysql := AddTestingApplication(c, s.state, "mysql", ch)
	mysqlEP, err := mysql.Endpoint("server")
	c.Assert(err, jc.ErrorIsNil)
	rel, err := s.state.AddRelation(remoteEP, mysqlEP)
	c.Assert(err, jc.ErrorIsNil)

	relTag := names.NewRelationTag(rel.String()).String()
	statusID := s.state.docID(fmt.Sprintf("r#%d", rel.Id()))
	netID := s.state.docID(rel.String() + ":ingress:default")
	entityID := s.state.docID(relTag)
	connID := s.state.docID(strconv.Itoa(rel.Id()))
	permID := s.state.docID("perm-dangling")
	otherPermID := s.state.docID("perm-other")
	seed := func(collName, docID string, extra bson.M) {
		coll, closer, err := s.state.db().GetRawCollection(collName)
		c.Assert(err, jc.ErrorIsNil)
		// AddRelation already creates the relation's status doc, so
		// overwrite whatever is there rather than inserting.
		_ = coll.RemoveId(docID)
		doc := bson.M{"_id": docID, "model-uuid": s.state.ModelUUID()}
		for name, value := range extra {
			doc[name] = value
		}
		err = coll.Insert(doc)
		closer()
		c.Assert(err, jc.ErrorIsNil)
	}
	seed("statuses", statusID, bson.M{})
	seed("relationNetworks", netID, bson.M{"relation-key": rel.String()})
	seed("remoteEntities", entityID, bson.M{"token": "token"})
	seed("applicationOfferConnections", connID, bson.M{})
	seed("secretPermissions", permID, bson.M{
		"scope-tag":   relTag,
		"subject-tag": "unit-mysql/0",
		"role":        "read",
	})
	// A permission scoped to something else must survive the removal.
	seed("secretPermissions", otherPermID, bson.M{
		"scope-tag":   "unit-wordpress/0",
		"subject-tag": "unit-mysql/0",
		"role":        "read",
	})
	countDocs := func(collName string, sel bson.M) int {
		coll, closer, err := s.state.db().GetRawCollection(collName)
		c.Assert(err, jc.ErrorIsNil)
		defer closer()
		n, err := coll.Find(sel).Count()
		c.Assert(err, jc.ErrorIsNil)
		return n
	}
	c.Assert(countDocs("secretPermissions", bson.M{"_id": permID}), gc.Equals, 1)
	c.Assert(countDocs("secretPermissions", bson.M{"_id": otherPermID}), gc.Equals, 1)

	// The remote application document disappears while the relation
	// stays alive.
	appsColl, appsCloser, err := s.state.db().GetRawCollection("remoteApplications")
	c.Assert(err, jc.ErrorIsNil)
	err = appsColl.RemoveId(s.state.docID("remote-dangling"))
	appsCloser()
	c.Assert(err, jc.ErrorIsNil)

	// Run twice to verify idempotency.
	for i := 0; i < 2; i++ {
		err := RemoveOrphanedApplicationRelations(s.pool)
		c.Assert(err, jc.ErrorIsNil)
	}

	c.Assert(countDocs("statuses", bson.M{"_id": statusID}), gc.Equals, 0)
	c.Assert(countDocs("relationNetworks", bson.M{"_id": netID}), gc.Equals, 0)
	c.Assert(countDocs("remoteEntities", bson.M{"_id": entityID}), gc.Equals, 0)
	c.Assert(countDocs("applicationOfferConnections", bson.M{"_id": connID}), gc.Equals, 0)
	c.Assert(countDocs("secretPermissions", bson.M{"_id": permID}), gc.Equals, 0)
	// The unrelated permission remains.
	c.Assert(countDocs("secretPermissions", bson.M{"_id": otherPermID}), gc.Equals, 1)
}

func (s *upgradesSuite) TestRemoveOrphanedApplicationRelationsDyingAppCleanup(c *gc.C) {
	rapp, err := s.state.AddRemoteApplication(AddRemoteApplicationParams{
		Name:        "remote-dangling",
		SourceModel: names.NewModelTag("source-model"),
		OfferUUID:   "offer-dangling",
		Endpoints: []charm.Relation{{
			Interface: "mysql",
			Limit:     1,
			Name:      "db",
			Role:      charm.RoleRequirer,
			Scope:     charm.ScopeGlobal,
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	remoteEP, err := rapp.Endpoint("db")
	c.Assert(err, jc.ErrorIsNil)
	ch := AddTestingCharm(c, s.state, "mysql")
	mysql := AddTestingApplication(c, s.state, "mysql", ch)
	mysqlEP, err := mysql.Endpoint("server")
	c.Assert(err, jc.ErrorIsNil)
	rel, err := s.state.AddRelation(remoteEP, mysqlEP)
	c.Assert(err, jc.ErrorIsNil)

	// The surviving local application is dying with no units; the
	// relation is its only remaining reference.
	appColl, appCloser, err := s.state.db().GetRawCollection("applications")
	c.Assert(err, jc.ErrorIsNil)
	err = appColl.UpdateId(s.state.docID("mysql"), bson.M{"$set": bson.M{"life": Dying}})
	appCloser()
	c.Assert(err, jc.ErrorIsNil)

	// The remote application document disappears while the relation
	// stays alive.
	appsColl, appsCloser, err := s.state.db().GetRawCollection("remoteApplications")
	c.Assert(err, jc.ErrorIsNil)
	err = appsColl.RemoveId(s.state.docID("remote-dangling"))
	appsCloser()
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(RemoveOrphanedApplicationRelations(s.pool), jc.ErrorIsNil)

	// The relation is gone and the local application's count was
	// decremented to zero.
	_, err = s.state.Relation(rel.Id())
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	var appDoc struct {
		RelationCount int `bson:"relationcount"`
	}
	appColl, appCloser, err = s.state.db().GetRawCollection("applications")
	c.Assert(err, jc.ErrorIsNil)
	err = appColl.FindId(s.state.docID("mysql")).One(&appDoc)
	appCloser()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(appDoc.RelationCount, gc.Equals, 0)

	// Normal relation removal queues an application cleanup when
	// decrementing a dying application's references, so it is destroyed
	// once it has none left; the upgrade step must do the same.
	cleanupsColl, closer, err := s.state.db().GetRawCollection(cleanupsC)
	c.Assert(err, jc.ErrorIsNil)
	var cleanupDoc struct {
		DocID     string      `bson:"_id"`
		ModelUUID string      `bson:"model-uuid"`
		Kind      cleanupKind `bson:"kind"`
		Prefix    string      `bson:"prefix"`
	}
	err = cleanupsColl.Find(bson.M{"kind": cleanupApplication, "prefix": "mysql"}).One(&cleanupDoc)
	closer()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cleanupDoc.Kind, gc.Equals, cleanupApplication)
	c.Assert(cleanupDoc.Prefix, gc.Equals, "mysql")
	c.Assert(strings.HasPrefix(cleanupDoc.DocID, s.state.ModelUUID()+":"), jc.IsTrue)
	c.Assert(cleanupDoc.ModelUUID, gc.Equals, s.state.ModelUUID())

	// Running the cleanups destroys the dying application, whose last
	// reference (the orphaned relation) is gone.
	noopDeleter := func(*secrets.URI, int) error { return nil }
	c.Assert(s.state.Cleanup(noopDeleter), jc.ErrorIsNil)
	_, err = s.state.Application("mysql")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *upgradesSuite) TestRemoveOrphanedApplicationRelationsDyingRemoteApp(c *gc.C) {
	rapp, err := s.state.AddRemoteApplication(AddRemoteApplicationParams{
		Name:        "remote-dying",
		SourceModel: names.NewModelTag("source-model"),
		OfferUUID:   "offer-dying",
		Endpoints: []charm.Relation{{
			Interface: "mysql",
			Limit:     1,
			Name:      "db",
			Role:      charm.RoleRequirer,
			Scope:     charm.ScopeGlobal,
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	remoteEP, err := rapp.Endpoint("db")
	c.Assert(err, jc.ErrorIsNil)
	ch := AddTestingCharm(c, s.state, "mysql")
	mysql := AddTestingApplication(c, s.state, "mysql", ch)
	mysqlEP, err := mysql.Endpoint("server")
	c.Assert(err, jc.ErrorIsNil)
	rel, err := s.state.AddRelation(remoteEP, mysqlEP)
	c.Assert(err, jc.ErrorIsNil)

	// The surviving remote application is dying and this relation is
	// its last reference.
	appsColl, appsCloser, err := s.state.db().GetRawCollection("remoteApplications")
	c.Assert(err, jc.ErrorIsNil)
	err = appsColl.UpdateId(s.state.docID("remote-dying"), bson.M{"$set": bson.M{"life": Dying}})
	appsCloser()
	c.Assert(err, jc.ErrorIsNil)

	// Corrupt the local endpoint's application name.
	s.corruptEndpointAppName(c, rel, "mysql", "bad name!")

	c.Assert(RemoveOrphanedApplicationRelations(s.pool), jc.ErrorIsNil)

	// The relation is gone and the dying remote application, whose
	// last reference it was, is removed with it, mirroring normal
	// relation removal.
	_, err = s.state.Relation(rel.Id())
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	_, err = s.state.RemoteApplication("remote-dying")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *upgradesSuite) TestRemoveOrphanedApplicationRelationsConsumerProxyLastRelation(c *gc.C) {
	rapp, err := s.state.AddRemoteApplication(AddRemoteApplicationParams{
		Name:        "remote-proxy",
		SourceModel: names.NewModelTag("source-model"),
		OfferUUID:   "offer-proxy",
		Endpoints: []charm.Relation{{
			Interface: "mysql",
			Limit:     1,
			Name:      "db",
			Role:      charm.RoleRequirer,
			Scope:     charm.ScopeGlobal,
		}},
		IsConsumerProxy: true,
	})
	c.Assert(err, jc.ErrorIsNil)
	remoteEP, err := rapp.Endpoint("db")
	c.Assert(err, jc.ErrorIsNil)
	ch := AddTestingCharm(c, s.state, "mysql")
	mysql := AddTestingApplication(c, s.state, "mysql", ch)
	mysqlEP, err := mysql.Endpoint("server")
	c.Assert(err, jc.ErrorIsNil)
	rel, err := s.state.AddRelation(remoteEP, mysqlEP)
	c.Assert(err, jc.ErrorIsNil)

	// The local endpoint's application name is corrupted.
	s.corruptEndpointAppName(c, rel, "mysql", "bad name!")

	c.Assert(RemoveOrphanedApplicationRelations(s.pool), jc.ErrorIsNil)

	// The relation is gone and the consumer proxy, whose last relation
	// it was, is removed with it, mirroring normal relation removal.
	_, err = s.state.Relation(rel.Id())
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	_, err = s.state.RemoteApplication("remote-proxy")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *upgradesSuite) TestRemoveOrphanedApplicationRelationsDyingRemoteAppMultiple(c *gc.C) {
	rapp, err := s.state.AddRemoteApplication(AddRemoteApplicationParams{
		Name:        "remote-dying",
		SourceModel: names.NewModelTag("source-model"),
		OfferUUID:   "offer-dying",
		Endpoints: []charm.Relation{{
			Interface: "mysql",
			Limit:     2,
			Name:      "db",
			Role:      charm.RoleProvider,
			Scope:     charm.ScopeGlobal,
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	remoteEP, err := rapp.Endpoint("db")
	c.Assert(err, jc.ErrorIsNil)
	ch := AddTestingCharm(c, s.state, "wordpress")
	wp1 := AddTestingApplication(c, s.state, "wp1", ch)
	wp1EP, err := wp1.Endpoint("db")
	c.Assert(err, jc.ErrorIsNil)
	wp2 := AddTestingApplication(c, s.state, "wp2", ch)
	wp2EP, err := wp2.Endpoint("db")
	c.Assert(err, jc.ErrorIsNil)
	rel1, err := s.state.AddRelation(remoteEP, wp1EP)
	c.Assert(err, jc.ErrorIsNil)
	rel2, err := s.state.AddRelation(remoteEP, wp2EP)
	c.Assert(err, jc.ErrorIsNil)

	// The surviving remote application is dying and these relations
	// are its only references.
	appsColl, appsCloser, err := s.state.db().GetRawCollection("remoteApplications")
	c.Assert(err, jc.ErrorIsNil)
	err = appsColl.UpdateId(s.state.docID("remote-dying"), bson.M{"$set": bson.M{"life": Dying}})
	appsCloser()
	c.Assert(err, jc.ErrorIsNil)

	// Corrupt both local endpoints' application names so both
	// relations are orphaned.
	s.corruptEndpointAppName(c, rel1, "wp1", "bad name 1!")
	s.corruptEndpointAppName(c, rel2, "wp2", "bad name 2!")

	c.Assert(RemoveOrphanedApplicationRelations(s.pool), jc.ErrorIsNil)

	// Both relations are gone and the dying remote application, whose
	// last reference the second removal was, is removed with it.
	_, err = s.state.Relation(rel1.Id())
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	_, err = s.state.Relation(rel2.Id())
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	_, err = s.state.RemoteApplication("remote-dying")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

// corruptEndpointAppName rewrites the named endpoint application's name
// in the relation document directly in the DB.
func (s *upgradesSuite) corruptEndpointAppName(c *gc.C, rel *Relation, appName, badName string) {
	relColl, closer, err := s.state.db().GetRawCollection("relations")
	c.Assert(err, jc.ErrorIsNil)
	var relDoc struct {
		Endpoints []struct {
			ApplicationName string `bson:"applicationname"`
		} `bson:"endpoints"`
	}
	err = relColl.FindId(s.state.docID(rel.Tag().Id())).One(&relDoc)
	c.Assert(err, jc.ErrorIsNil)
	idx := -1
	for i, ep := range relDoc.Endpoints {
		if ep.ApplicationName == appName {
			idx = i
		}
	}
	c.Assert(idx >= 0, jc.IsTrue)
	err = relColl.UpdateId(s.state.docID(rel.Tag().Id()), bson.M{"$set": bson.M{
		fmt.Sprintf("endpoints.%d.applicationname", idx): badName,
	}})
	closer()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *upgradesSuite) TestRemoveOrphanedRelationDocsRecreatesApplicationSettingsNoOtherDocs(c *gc.C) {
	rapp, err := s.state.AddRemoteApplication(AddRemoteApplicationParams{
		Name:        "remote-bare",
		SourceModel: names.NewModelTag("source-model"),
		OfferUUID:   "offer-uuid",
		Endpoints: []charm.Relation{{
			Interface: "mysql",
			Limit:     1,
			Name:      "db",
			Role:      charm.RoleRequirer,
			Scope:     charm.ScopeGlobal,
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	remoteEP, err := rapp.Endpoint("db")
	c.Assert(err, jc.ErrorIsNil)
	ch := AddTestingCharm(c, s.state, "mysql")
	mysql := AddTestingApplication(c, s.state, "mysql", ch)
	mysqlEP, err := mysql.Endpoint("server")
	c.Assert(err, jc.ErrorIsNil)
	rel, err := s.state.AddRelation(remoteEP, mysqlEP)
	c.Assert(err, jc.ErrorIsNil)

	// No unit ever enters scope, so the relation's only r#-prefixed
	// documents are the two application settings docs.
	prefix := s.relPrefix(rel)
	settings := s.docIDsWithPrefix(c, "settings", prefix)
	c.Assert(settings, gc.HasLen, 2)
	settingsColl, closer, err := s.state.db().GetRawCollection("settings")
	c.Assert(err, jc.ErrorIsNil)
	for _, id := range settings {
		err = settingsColl.RemoveId(id)
		c.Assert(err, jc.ErrorIsNil)
	}
	closer()

	// Run twice to verify idempotency.
	for i := 0; i < 2; i++ {
		err = RemoveOrphanedRelationDocs(s.pool)
		c.Assert(err, jc.ErrorIsNil)
	}

	// The application settings docs for both endpoint applications are
	// recreated empty.
	c.Assert(s.countDocs(c, "settings", prefix), gc.Equals, 2)
}

func (s *upgradesSuite) TestRemoveOrphanedRelationDocsSkipsAppSettingsForDanglingApps(c *gc.C) {
	rapp, err := s.state.AddRemoteApplication(AddRemoteApplicationParams{
		Name:        "remote-dangling",
		SourceModel: names.NewModelTag("source-model"),
		OfferUUID:   "offer-uuid",
		Endpoints: []charm.Relation{{
			Interface: "mysql",
			Limit:     1,
			Name:      "db",
			Role:      charm.RoleRequirer,
			Scope:     charm.ScopeGlobal,
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	remoteEP, err := rapp.Endpoint("db")
	c.Assert(err, jc.ErrorIsNil)
	ch := AddTestingCharm(c, s.state, "mysql")
	mysql := AddTestingApplication(c, s.state, "mysql", ch)
	mysqlEP, err := mysql.Endpoint("server")
	c.Assert(err, jc.ErrorIsNil)
	rel, err := s.state.AddRelation(remoteEP, mysqlEP)
	c.Assert(err, jc.ErrorIsNil)

	// Corrupt the local endpoint name and make the relation's removal
	// fail (the remote application's relationcount is already zero), so
	// the relation survives the orphaned application repair step with an
	// orphaned endpoint.
	s.corruptEndpointAppName(c, rel, "mysql", "bad name!")
	s.setRemoteCount(c, "remote-dangling", 0)
	c.Assert(RemoveOrphanedApplicationRelations(s.pool), jc.ErrorIsNil)
	_, err = s.state.Relation(rel.Id())
	c.Assert(err, jc.ErrorIsNil)

	// Delete the relation's application settings docs; no other r#
	// docs exist.
	prefix := s.relPrefix(rel)
	settingsColl, closer, err := s.state.db().GetRawCollection("settings")
	c.Assert(err, jc.ErrorIsNil)
	for _, id := range s.docIDsWithPrefix(c, "settings", prefix) {
		err = settingsColl.RemoveId(id)
		c.Assert(err, jc.ErrorIsNil)
	}
	closer()

	c.Assert(RemoveOrphanedRelationDocs(s.pool), jc.ErrorIsNil)

	// The relation has a dangling endpoint application, so its docs are
	// left for manual repair: nothing is recreated.
	c.Assert(s.countDocs(c, "settings", prefix), gc.Equals, 0)
}

// docIDsWithPrefix returns the raw ids of documents in the named
// collection whose id starts with the model-prefixed prefix.
func (s *upgradesSuite) docIDsWithPrefix(c *gc.C, collName, prefix string) []string {
	coll, closer, err := s.state.db().GetRawCollection(collName)
	c.Assert(err, jc.ErrorIsNil)
	defer closer()
	var ids []string
	iter := coll.Find(bson.M{
		"_id": bson.M{"$regex": "^" + s.state.docID(prefix)},
	}).Select(bson.M{"_id": 1}).Iter()
	defer iter.Close()
	var doc struct {
		DocID string `bson:"_id"`
	}
	for iter.Next(&doc) {
		ids = append(ids, doc.DocID)
	}
	c.Assert(iter.Close(), jc.ErrorIsNil)
	return ids
}

func (s *upgradesSuite) TestRemoveOrphanedRelationDocsLeavesMissingUnitScopes(c *gc.C) {
	rapp, err := s.state.AddRemoteApplication(AddRemoteApplicationParams{
		Name:        "remote-wp",
		SourceModel: names.NewModelTag("source-model"),
		OfferUUID:   "offer-uuid",
		Endpoints: []charm.Relation{{
			Interface: "mysql",
			Limit:     1,
			Name:      "db",
			Role:      charm.RoleRequirer,
			Scope:     charm.ScopeGlobal,
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	rwpEP, err := rapp.Endpoint("db")
	c.Assert(err, jc.ErrorIsNil)
	ch := AddTestingCharm(c, s.state, "mysql")
	mysql := AddTestingApplication(c, s.state, "mysql", ch)
	mysqlEP, err := mysql.Endpoint("server")
	c.Assert(err, jc.ErrorIsNil)
	rel, err := s.state.AddRelation(rwpEP, mysqlEP)
	c.Assert(err, jc.ErrorIsNil)
	unit, err := mysql.AddUnit(AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	ru, err := rel.Unit(unit)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ru.EnterScope(nil), jc.ErrorIsNil)

	relPrefix := fmt.Sprintf("r#%d#", rel.Id())
	countDocs := func(collName string) int {
		coll, closer, err := s.state.db().GetRawCollection(collName)
		c.Assert(err, jc.ErrorIsNil)
		defer closer()
		n, err := coll.Find(bson.M{
			"_id": bson.M{"$regex": "^" + s.state.docID(relPrefix)},
		}).Count()
		c.Assert(err, jc.ErrorIsNil)
		return n
	}
	c.Assert(countDocs("relationscopes"), gc.Equals, 1)

	// Delete the unit's document.
	unitsColl, closer, err := s.state.db().GetRawCollection("units")
	c.Assert(err, jc.ErrorIsNil)
	err = unitsColl.RemoveId(s.state.docID(unit.Name()))
	closer()
	c.Assert(err, jc.ErrorIsNil)

	err = RemoveOrphanedRelationDocs(s.pool)
	c.Assert(err, jc.ErrorIsNil)

	// The scope, its settings, and the relation's unit count are all
	// untouched.
	c.Assert(countDocs("relationscopes"), gc.Equals, 1)
	c.Assert(countDocs("settings"), gc.Equals, 3)
	cur, err := s.state.Relation(rel.Id())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cur.UnitCount(), gc.Equals, 1)
}

func (s *upgradesSuite) TestRemoveOrphanedRelationDocsRecreatesApplicationSettings(c *gc.C) {
	rapp, err := s.state.AddRemoteApplication(AddRemoteApplicationParams{
		Name:        "remote-proxy",
		SourceModel: names.NewModelTag("source-model"),
		OfferUUID:   "offer-uuid",
		Endpoints: []charm.Relation{{
			Interface: "mysql",
			Limit:     1,
			Name:      "db",
			Role:      charm.RoleRequirer,
			Scope:     charm.ScopeGlobal,
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	rwpEP, err := rapp.Endpoint("db")
	c.Assert(err, jc.ErrorIsNil)
	ch := AddTestingCharm(c, s.state, "mysql")
	mysql := AddTestingApplication(c, s.state, "mysql", ch)
	mysqlEP, err := mysql.Endpoint("server")
	c.Assert(err, jc.ErrorIsNil)
	rel, err := s.state.AddRelation(rwpEP, mysqlEP)
	c.Assert(err, jc.ErrorIsNil)
	unit, err := mysql.AddUnit(AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	ru, err := rel.Unit(unit)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ru.EnterScope(nil), jc.ErrorIsNil)

	relPrefix := fmt.Sprintf("r#%d#", rel.Id())
	idsWithPrefix := func(collName string) []string {
		coll, closer, err := s.state.db().GetRawCollection(collName)
		c.Assert(err, jc.ErrorIsNil)
		defer closer()
		var ids []string
		iter := coll.Find(bson.M{
			"_id": bson.M{"$regex": "^" + s.state.docID(relPrefix)},
		}).Select(bson.M{"_id": 1}).Iter()
		defer iter.Close()
		var doc struct {
			DocID string `bson:"_id"`
		}
		for iter.Next(&doc) {
			ids = append(ids, doc.DocID)
		}
		return ids
	}
	// Delete all the relation's settings docs.
	settings := idsWithPrefix("settings")
	c.Assert(len(settings), gc.Equals, 3)
	settingsColl, closer, err := s.state.db().GetRawCollection("settings")
	c.Assert(err, jc.ErrorIsNil)
	for _, id := range settings {
		err = settingsColl.RemoveId(id)
		c.Assert(err, jc.ErrorIsNil)
	}
	closer()
	c.Assert(len(idsWithPrefix("relationscopes")), gc.Equals, 1)
	c.Assert(len(idsWithPrefix("settings")), gc.Equals, 0)

	// Run twice to verify idempotency.
	for i := 0; i < 2; i++ {
		err := RemoveOrphanedRelationDocs(s.pool)
		c.Assert(err, jc.ErrorIsNil)
	}

	// The scope is untouched, and all the settings docs the relation
	// units watcher relies on are recreated empty.
	c.Assert(len(idsWithPrefix("relationscopes")), gc.Equals, 1)
	c.Assert(len(idsWithPrefix("settings")), gc.Equals, 3)
}

func (s *upgradesSuite) TestRemoveOrphanedUnitStateRelations(c *gc.C) {
	// Create a relation and a unit in scope, then record persisted
	// unit state for it plus a stale (deleted) relation id.
	rwp, err := s.state.AddRemoteApplication(AddRemoteApplicationParams{
		Name:        "remote-wordpress",
		SourceModel: names.NewModelTag("source-model"),
		OfferUUID:   "offer-uuid",
		Endpoints: []charm.Relation{{
			Interface: "mysql",
			Limit:     1,
			Name:      "db",
			Role:      charm.RoleRequirer,
			Scope:     charm.ScopeGlobal,
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	rwpEP, err := rwp.Endpoint("db")
	c.Assert(err, jc.ErrorIsNil)
	ch := AddTestingCharm(c, s.state, "mysql")
	mysql := AddTestingApplication(c, s.state, "mysql", ch)
	mysqlEP, err := mysql.Endpoint("server")
	c.Assert(err, jc.ErrorIsNil)
	rel, err := s.state.AddRelation(rwpEP, mysqlEP)
	c.Assert(err, jc.ErrorIsNil)
	relID := rel.Id()
	unit, err := mysql.AddUnit(AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)

	// Persist unit state with a stale relation id that never existed.
	us := NewUnitState()
	us.SetRelationState(map[int]string{relID: "checkpoint"})
	err = unit.SetState(us, UnitStateSizeLimits{})
	c.Assert(err, jc.ErrorIsNil)

	coll, closer, err := s.state.db().GetRawCollection("unitstates")
	c.Assert(err, jc.ErrorIsNil)
	err = coll.UpdateId(s.state.docID(unit.globalKey()), bson.M{"$set": bson.M{
		"relation-state": bson.M{
			strconv.Itoa(relID): "checkpoint",
			"99999999":          "stale checkpoint",
		},
	}})
	closer()
	c.Assert(err, jc.ErrorIsNil)

	// Run the upgrade step. The stale id must be removed; the live one kept.
	err = RemoveOrphanedUnitStateRelations(s.pool)
	c.Assert(err, jc.ErrorIsNil)

	uState, err := unit.State()
	c.Assert(err, jc.ErrorIsNil)
	rst, found := uState.RelationState()
	c.Assert(found, jc.IsTrue)
	c.Assert(rst, gc.DeepEquals, map[int]string{
		relID: "checkpoint",
	})
}

func (s *upgradesSuite) TestRemoveOrphanedUnitStateRelationsMultipleStale(c *gc.C) {
	// Create a relation and a unit, then record persisted unit state
	// for it plus two stale (deleted) relation ids.
	rwp, err := s.state.AddRemoteApplication(AddRemoteApplicationParams{
		Name:        "remote-wordpress",
		SourceModel: names.NewModelTag("source-model"),
		OfferUUID:   "offer-uuid",
		Endpoints: []charm.Relation{{
			Interface: "mysql",
			Limit:     1,
			Name:      "db",
			Role:      charm.RoleRequirer,
			Scope:     charm.ScopeGlobal,
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	rwpEP, err := rwp.Endpoint("db")
	c.Assert(err, jc.ErrorIsNil)
	ch := AddTestingCharm(c, s.state, "mysql")
	mysql := AddTestingApplication(c, s.state, "mysql", ch)
	mysqlEP, err := mysql.Endpoint("server")
	c.Assert(err, jc.ErrorIsNil)
	rel, err := s.state.AddRelation(rwpEP, mysqlEP)
	c.Assert(err, jc.ErrorIsNil)
	relID := rel.Id()
	unit, err := mysql.AddUnit(AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)

	// Persist unit state with two stale relation ids that never existed.
	us := NewUnitState()
	us.SetRelationState(map[int]string{relID: "checkpoint"})
	err = unit.SetState(us, UnitStateSizeLimits{})
	c.Assert(err, jc.ErrorIsNil)

	coll, closer, err := s.state.db().GetRawCollection("unitstates")
	c.Assert(err, jc.ErrorIsNil)
	err = coll.UpdateId(s.state.docID(unit.globalKey()), bson.M{"$set": bson.M{
		"relation-state": bson.M{
			strconv.Itoa(relID): "checkpoint",
			"99999999":          "stale checkpoint",
			"99999998":          "stale checkpoint 2",
		},
	}})
	closer()
	c.Assert(err, jc.ErrorIsNil)

	// Run the upgrade step. Both stale ids must be removed; the live one
	// kept.
	err = RemoveOrphanedUnitStateRelations(s.pool)
	c.Assert(err, jc.ErrorIsNil)

	uState, err := unit.State()
	c.Assert(err, jc.ErrorIsNil)
	rst, found := uState.RelationState()
	c.Assert(found, jc.IsTrue)
	c.Assert(rst, gc.DeepEquals, map[int]string{
		relID: "checkpoint",
	})
}

func (s *upgradesSuite) TestRemoveOrphanedRelationDocsLeavesDanglingEndpointDocs(c *gc.C) {
	rapp, err := s.state.AddRemoteApplication(AddRemoteApplicationParams{
		Name:        "remote-missing",
		SourceModel: names.NewModelTag("source-model"),
		OfferUUID:   "offer-missing",
		Endpoints: []charm.Relation{{
			Interface: "mysql",
			Limit:     1,
			Name:      "db",
			Role:      charm.RoleRequirer,
			Scope:     charm.ScopeGlobal,
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	remoteEP, err := rapp.Endpoint("db")
	c.Assert(err, jc.ErrorIsNil)
	ch := AddTestingCharm(c, s.state, "mysql")
	mysql := AddTestingApplication(c, s.state, "mysql", ch)
	mysqlEP, err := mysql.Endpoint("server")
	c.Assert(err, jc.ErrorIsNil)
	rel, err := s.state.AddRelation(remoteEP, mysqlEP)
	c.Assert(err, jc.ErrorIsNil)
	mysqlUnit, err := mysql.AddUnit(AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	ru, err := rel.Unit(mysqlUnit)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ru.EnterScope(nil), jc.ErrorIsNil)
	prefix := s.relPrefix(rel)
	c.Assert(s.countDocs(c, "relationscopes", prefix), gc.Equals, 1)
	c.Assert(s.countDocs(c, "settings", prefix), gc.Equals, 3)

	// Drop the remote endpoint's application settings doc.
	settingsColl, closer, err := s.state.db().GetRawCollection("settings")
	c.Assert(err, jc.ErrorIsNil)
	err = settingsColl.RemoveId(s.state.docID(fmt.Sprintf("r#%d#remote-missing", rel.Id())))
	closer()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.countDocs(c, "settings", prefix), gc.Equals, 2)

	// The remote application document disappears, the relation is left
	// dying, and the surviving application's relationcount drifts to 0.
	relColl, closer, err := s.state.db().GetRawCollection("relations")
	c.Assert(err, jc.ErrorIsNil)
	err = relColl.UpdateId(s.state.docID(rel.Tag().Id()), bson.M{"$set": bson.M{"life": Dying}})
	closer()
	c.Assert(err, jc.ErrorIsNil)
	appsColl, appsCloser, err := s.state.db().GetRawCollection("remoteApplications")
	c.Assert(err, jc.ErrorIsNil)
	err = appsColl.RemoveId(s.state.docID("remote-missing"))
	appsCloser()
	c.Assert(err, jc.ErrorIsNil)
	appColl, appCloser, err := s.state.db().GetRawCollection("applications")
	c.Assert(err, jc.ErrorIsNil)
	err = appColl.UpdateId(s.state.docID("mysql"), bson.M{"$set": bson.M{"relationcount": 0}})
	appCloser()
	c.Assert(err, jc.ErrorIsNil)

	// The orphaned relation's removal aborts on the count assertion; the
	// relation and its docs are left for manual repair.
	err = RemoveOrphanedApplicationRelations(s.pool)
	c.Assert(err, jc.ErrorIsNil)
	cur, err := s.state.Relation(rel.Id())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cur.Life(), gc.Equals, Dying)
	c.Assert(s.countDocs(c, "relationscopes", prefix), gc.Equals, 1)
	c.Assert(s.countDocs(c, "settings", prefix), gc.Equals, 2)

	// The orphaned endpoint warning path must leave the docs untouched,
	// including the still-missing remote application settings doc.
	err = RemoveOrphanedRelationDocs(s.pool)
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.state.Relation(rel.Id())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.countDocs(c, "relationscopes", prefix), gc.Equals, 1)
	c.Assert(s.countDocs(c, "settings", prefix), gc.Equals, 2)
}

func (s *upgradesSuite) TestUpgradeRepairsBrokenDB(c *gc.C) {
	ch := AddTestingCharm(c, s.state, "wordpress")
	removeIds := func(collName string, ids ...string) {
		coll, closer, err := s.state.db().GetRawCollection(collName)
		c.Assert(err, jc.ErrorIsNil)
		defer closer()
		for _, id := range ids {
			err = coll.RemoveId(s.state.docID(id))
			c.Assert(err, jc.ErrorIsNil)
		}
	}
	idsWithPrefix := func(collName, prefix string) []string {
		coll, closer, err := s.state.db().GetRawCollection(collName)
		c.Assert(err, jc.ErrorIsNil)
		defer closer()
		var ids []string
		iter := coll.Find(bson.M{"_id": bson.M{"$regex": "^" + s.state.docID(prefix)}}).Select(bson.M{"_id": 1}).Iter()
		defer iter.Close()
		var doc struct {
			DocID string `bson:"_id"`
		}
		for iter.Next(&doc) {
			ids = append(ids, doc.DocID)
		}
		return ids
	}

	// remote application relationcount drift.
	counter := s.makeRemoteApplication(c, "counter")
	s.setRemoteCount(c, counter.Name(), -4)
	counterLive := s.makeRemoteApplication(c, "counter-live")
	counterRel, _ := s.mkRel(c, ch, "counter-wp", counterLive)

	// orphaned relation scopes/settings docs, parent
	// relation deleted.
	orphan := s.makeRemoteApplication(c, "orphan")
	orphanRel, _ := s.mkRel(c, ch, "orphan-wp", orphan)
	relColl, closer, err := s.state.db().GetRawCollection("relations")
	c.Assert(err, jc.ErrorIsNil)
	err = relColl.RemoveId(s.state.docID(orphanRel.Tag().Id()))
	closer()
	c.Assert(err, jc.ErrorIsNil)
	settingsColl, closer, err := s.state.db().GetRawCollection("settings")
	c.Assert(err, jc.ErrorIsNil)
	err = settingsColl.Insert(bson.M{
		"_id":        s.state.docID("r#990001#charm"),
		"model-uuid": s.state.ModelUUID(),
		"settings":   bson.M{},
	})
	c.Assert(err, jc.ErrorIsNil)
	closer()
	scopesColl, closer, err := s.state.db().GetRawCollection("relationscopes")
	c.Assert(err, jc.ErrorIsNil)
	err = scopesColl.Insert(bson.M{
		"_id":        s.state.docID("r#990002#global#u#ghost/0"),
		"model-uuid": s.state.ModelUUID(),
	})
	c.Assert(err, jc.ErrorIsNil)
	closer()
	c.Assert(s.countDocs(c, "relationscopes", s.relPrefix(orphanRel)), gc.Equals, 1)
	c.Assert(s.countDocs(c, "settings", s.relPrefix(orphanRel)), gc.Equals, 3)

	// relation scope without its settings document.
	watcher := s.makeRemoteApplication(c, "watcher-proxy")
	watcherRel, watcherUnit := s.mkRel(c, ch, "watcher", watcher)
	watcherSettings := idsWithPrefix("settings", s.relPrefix(watcherRel))
	c.Assert(len(watcherSettings), gc.Equals, 3)
	removeIds("settings", watcherSettings...)
	c.Assert(s.countDocs(c, "relationscopes", s.relPrefix(watcherRel)), gc.Equals, 1)
	c.Assert(s.countDocs(c, "settings", s.relPrefix(watcherRel)), gc.Equals, 0)

	// unitstates relation-state referencing a removed relation.
	us := NewUnitState()
	us.SetRelationState(map[int]string{watcherRel.Id(): "checkpoint"})
	err = watcherUnit.SetState(us, UnitStateSizeLimits{})
	c.Assert(err, jc.ErrorIsNil)
	unitstatesColl, closer, err := s.state.db().GetRawCollection("unitstates")
	c.Assert(err, jc.ErrorIsNil)
	err = unitstatesColl.UpdateId(s.state.docID(watcherUnit.globalKey()), bson.M{"$set": bson.M{
		"relation-state": bson.M{
			strconv.Itoa(watcherRel.Id()): "checkpoint",
			"99999998":                    "stale checkpoint",
		},
	}})
	closer()
	c.Assert(err, jc.ErrorIsNil)

	// relation whose remote application document is gone
	// while the relation stayed alive.
	dangling := s.makeRemoteApplication(c, "dangling")
	danglingRel, danglingUnit := s.mkRel(c, ch, "dangling-wp", dangling)
	// The bad relation's unit also has persisted relation state; it
	// must be cleared along with the relation itself, even though the
	// relation's removal is deferred to the force destroy cleanup.
	danglingUS := NewUnitState()
	danglingUS.SetRelationState(map[int]string{danglingRel.Id(): "checkpoint"})
	err = danglingUnit.SetState(danglingUS, UnitStateSizeLimits{})
	c.Assert(err, jc.ErrorIsNil)
	appsColl, appsCloser, err := s.state.db().GetRawCollection("remoteApplications")
	c.Assert(err, jc.ErrorIsNil)
	err = appsColl.RemoveId(s.state.docID("dangling"))
	appsCloser()
	c.Assert(err, jc.ErrorIsNil)

	// local application relationcount drift.
	appColl, appCloser, err := s.state.db().GetRawCollection("applications")
	c.Assert(err, jc.ErrorIsNil)
	err = appColl.UpdateId(s.state.docID("watcher"), bson.M{"$set": bson.M{"relationcount": 9}})
	appCloser()
	c.Assert(err, jc.ErrorIsNil)

	for i := 0; i < 2; i++ {
		c.Assert(FixApplicationCounts(s.pool), jc.ErrorIsNil)
		c.Assert(RemoveOrphanedApplicationRelations(s.pool), jc.ErrorIsNil)
		c.Assert(RemoveOrphanedRelationDocs(s.pool), jc.ErrorIsNil)
		c.Assert(RemoveOrphanedUnitStateRelations(s.pool), jc.ErrorIsNil)
	}
	// The dangling relation is force-destroyed; complete the deferred
	// scope/settings cleanups (twice: the removal queues further
	// cleanups).
	noopDeleter := func(*secrets.URI, int) error { return nil }
	for i := 0; i < 2; i++ {
		c.Assert(s.state.Cleanup(noopDeleter), jc.ErrorIsNil)
	}

	checkCount := func(collName, name string) int {
		coll, closer, err := s.state.db().GetRawCollection(collName)
		c.Assert(err, jc.ErrorIsNil)
		defer closer()
		var doc struct {
			RelationCount int `bson:"relationcount"`
		}
		err = coll.FindId(s.state.docID(name)).One(&doc)
		c.Assert(err, jc.ErrorIsNil)
		return doc.RelationCount
	}

	// Counts corrected (negative set to 0).
	c.Assert(checkCount("remoteApplications", "counter"), gc.Equals, 0)
	c.Assert(checkCount("remoteApplications", "counter-live"), gc.Equals, 1)
	// Orphaned scope/settings removed.
	c.Assert(s.countDocs(c, "relationscopes", s.relPrefix(orphanRel)), gc.Equals, 0)
	c.Assert(s.countDocs(c, "settings", s.relPrefix(orphanRel)), gc.Equals, 0)
	c.Assert(s.countDocs(c, "relationscopes", "r#990002#"), gc.Equals, 0)
	c.Assert(s.countDocs(c, "settings", "r#990001#"), gc.Equals, 0)
	// Scope is left on its live relation.
	c.Assert(s.countDocs(c, "relationscopes", s.relPrefix(watcherRel)), gc.Equals, 1)
	// The relation's missing settings docs are recreated, including
	// the unit-scoped settings paired with the scope and the
	// application-level settings for both endpoint applications that
	// the relation units watcher watches.
	c.Assert(s.countDocs(c, "settings", s.relPrefix(watcherRel)), gc.Equals, 3)
	c.Assert(s.countDocs(c, "relationscopes", s.relPrefix(counterRel)), gc.Equals, 1)
	c.Assert(s.countDocs(c, "settings", s.relPrefix(counterRel)), gc.Equals, 3)
	// The stale unitstates key is gone, the live one kept.
	uState, err := watcherUnit.State()
	c.Assert(err, jc.ErrorIsNil)
	rst, found := uState.RelationState()
	c.Assert(found, jc.IsTrue)
	c.Assert(rst, gc.DeepEquals, map[int]string{
		watcherRel.Id(): "checkpoint",
	})
	// The dangling relation and its satellites are destroyed and the
	// local application's count reflects the removal.
	_, err = s.state.Relation(danglingRel.Id())
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	c.Assert(s.countDocs(c, "relationscopes", s.relPrefix(danglingRel)), gc.Equals, 0)
	c.Assert(s.countDocs(c, "settings", s.relPrefix(danglingRel)), gc.Equals, 0)
	c.Assert(checkCount("applications", "dangling-wp"), gc.Equals, 0)
	// The dangling relation's unit state is cleared as well; the
	// deferred force cleanup runs only after the upgrade, so the step
	// itself must remove the stale reference.
	danglingUState, err := danglingUnit.State()
	c.Assert(err, jc.ErrorIsNil)
	drst, dfound := danglingUState.RelationState()
	c.Assert(dfound, jc.IsTrue)
	c.Assert(drst, gc.HasLen, 0)
	// The drifted local application count is repaired to the actual
	// number of relations.
	c.Assert(checkCount("applications", "watcher"), gc.Equals, 1)
}

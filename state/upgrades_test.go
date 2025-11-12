// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"slices"
	"sort"

	"github.com/juju/errors"
	"github.com/juju/mgo/v3"
	"github.com/juju/mgo/v3/bson"
	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v3"
	"github.com/kr/pretty"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/storage/provider"
	"github.com/juju/juju/testing"
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

// TestUpgradeAddVirtualHostKeys tests that after an upgrade,
// machines and CAAS units have a virtual host key.
func (s *upgradesSuite) TestUpgradeAddVirtualHostKeys(c *gc.C) {
	machineModel := s.makeModel(c, "model-1", coretesting.Attrs{}, ModelArgs{Type: ModelTypeIAAS})
	k8sModel := s.makeModel(c, "model-2", coretesting.Attrs{}, ModelArgs{Type: ModelTypeCAAS})
	defer func() {
		_ = machineModel.Close()
		_ = k8sModel.Close()
	}()

	machinesColl, machinesCloser := s.state.db().GetRawCollection(machinesC)
	defer machinesCloser()

	err := machinesColl.Insert(bson.M{
		"_id":        ensureModelUUID(machineModel.ModelUUID(), "1"),
		"machineid":  "1",
		"model-uuid": machineModel.ModelUUID(),
	})
	c.Assert(err, jc.ErrorIsNil)

	unitsColl, unitsCloser := s.state.db().GetRawCollection(unitsC)
	defer unitsCloser()

	// The first unit is on a machine model and the second on a k8s model.
	// The first unit is not expected to have a key while the second is.
	err = unitsColl.Insert(
		bson.M{
			"_id":        ensureModelUUID(machineModel.ModelUUID(), "machineunit/1"),
			"name":       "machineunit/1",
			"model-uuid": machineModel.ModelUUID(),
			"machineid":  "1",
		}, bson.M{
			"_id":        ensureModelUUID(k8sModel.ModelUUID(), "k8sunit/1"),
			"name":       "k8sunit/1",
			"model-uuid": k8sModel.ModelUUID(),
		})
	c.Assert(err, jc.ErrorIsNil)

	virtualHostKeysColl, vhkCloser := s.state.db().GetRawCollection(virtualHostKeysC)
	defer vhkCloser()

	// The hostkey values below are ignored by the checker but must still exist for deepEquals to work.
	expectedVirtualHostKeys := []bson.M{
		{
			"_id":     fmt.Sprintf("%s:machine-1-hostkey", machineModel.ModelUUID()),
			"hostkey": []byte("placeholder"),
		}, {
			"_id":     fmt.Sprintf("%s:unit-k8sunit/1-hostkey", k8sModel.ModelUUID()),
			"hostkey": []byte("placeholder"),
		}}

	// Sort the values since the model UUIDs are random and assertUpgradedData fetches
	// the actual data in sorted order.
	sort.Slice(expectedVirtualHostKeys, func(i, j int) bool {
		return expectedVirtualHostKeys[i]["_id"].(string) < expectedVirtualHostKeys[j]["_id"].(string)
	})

	mc := jc.NewMultiChecker()
	mc.AddExpr(`_[_]["hostkey"]`, testing.BytesToStringMatch, `-----BEGIN OPENSSH PRIVATE KEY-----.*`)
	s.assertUpgradedData(c, AddVirtualHostKeys, mc,
		upgradedData(virtualHostKeysColl, expectedVirtualHostKeys),
	)
}

func (s *upgradesSuite) TestSplitMigrationStatusMessages(c *gc.C) {
	model := s.makeModel(c, "m", coretesting.Attrs{}, ModelArgs{Type: ModelTypeIAAS})
	defer func() { _ = model.Close() }()

	migStatus, closer := s.state.db().GetRawCollection(migrationsStatusC)
	defer closer()

	migStatusMessage, closer2 := s.state.db().GetRawCollection(migrationsStatusMessageC)
	defer closer2()

	err := migStatus.Insert(bson.M{
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

func (s *upgradesSuite) TestPopulateApplicationStorageUniqueID(c *gc.C) {
	state1 := s.makeModel(c, "m1", coretesting.Attrs{}, ModelArgs{Type: ModelTypeCAAS})
	state2 := s.makeModel(c, "m2", coretesting.Attrs{}, ModelArgs{Type: ModelTypeCAAS})
	defer func() {
		_ = state1.Close()
		_ = state2.Close()
	}()

	appColl1, closer := state1.db().GetRawCollection(applicationsC)
	defer closer()

	model1, err := state1.Model()
	c.Assert(err, gc.IsNil)

	err = appColl1.Insert(bson.M{
		"_id":        ensureModelUUID(model1.UUID(), "app1"),
		"name":       "app1",
		"model-uuid": model1.UUID(),
	})
	c.Assert(err, gc.IsNil)
	err = appColl1.Insert(bson.M{
		"_id":        ensureModelUUID(model1.UUID(), "app2"),
		"name":       "app2",
		"model-uuid": model1.UUID(),
	})
	c.Assert(err, gc.IsNil)
	// app3 does not get backfilled because its storage unique ID is already
	// populated.
	err = appColl1.Insert(bson.M{
		"_id":               ensureModelUUID(model1.UUID(), "app3"),
		"name":              "app3",
		"model-uuid":        model1.UUID(),
		"storage-unique-id": "uniqueid3",
	})
	c.Assert(err, gc.IsNil)

	model2, err := state2.Model()
	c.Assert(err, gc.IsNil)

	appColl2, closer := state2.db().GetRawCollection(applicationsC)
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
				{
					appName:  "app3",
					uniqueID: "uniqueid3",
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
}

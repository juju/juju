// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"fmt"
	v1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
	"sort"

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

type newK8sFunc func(model *Model) (kubernetes.Interface, *rest.Config, error)

func newFakeK8sClient(_ *Model) (kubernetes.Interface, *rest.Config, error) {
	k8sClient := fake.NewSimpleClientset()
	return k8sClient, nil, nil
}

func (s *upgradesSuite) TestPopulateApplicationStorageUniqueID(c *gc.C) {
	state1 := s.makeModel(c, "m1", coretesting.Attrs{}, ModelArgs{Type: ModelTypeCAAS})
	state2 := s.makeModel(c, "m2", coretesting.Attrs{}, ModelArgs{Type: ModelTypeCAAS})
	defer func() {
		_ = state1.Close()
		_ = state2.Close()
	}()

	coll1, closer := state1.db().GetRawCollection(applicationsC)
	defer closer()

	model1, err := state1.Model()
	c.Assert(err, gc.IsNil)

	err = coll1.Insert(bson.M{
		"_id":        ensureModelUUID(model1.UUID(), "app1"),
		"name":       "app1",
		"model-uuid": model1.UUID(),
	})
	c.Assert(err, gc.IsNil)
	err = coll1.Insert(bson.M{
		"_id":        ensureModelUUID(model1.UUID(), "app2"),
		"name":       "app2",
		"model-uuid": model1.UUID(),
	})
	c.Assert(err, gc.IsNil)
	err = coll1.Insert(bson.M{
		"_id":               ensureModelUUID(model1.UUID(), "app3"),
		"name":              "app3",
		"model-uuid":        model1.UUID(),
		"storage-unique-id": "uniqueid3",
	})
	c.Assert(err, gc.IsNil)

	model2, err := state2.Model()
	c.Assert(err, gc.IsNil)

	coll2, closer := state2.db().GetRawCollection(applicationsC)
	defer closer()

	err = coll2.Insert(bson.M{
		"_id":        ensureModelUUID(model2.UUID(), "app4"),
		"name":       "app4",
		"model-uuid": model2.UUID(),
	})
	c.Assert(err, gc.IsNil)
	err = coll2.Insert(bson.M{
		"_id":        ensureModelUUID(model2.UUID(), "app5"),
		"name":       "app5",
		"model-uuid": model2.UUID(),
	})
	c.Assert(err, gc.IsNil)

	getStorageUniqueID := func(newK8sClient newK8sFunc) func(appName string, _ *Model) (string, error) {
		k8sClient, _, err := newK8sClient(model1)

		stsApp1 := v1.StatefulSet{
			ObjectMeta: metav1.ObjectMeta{
				Name: "app1",
				Annotations: map[string]string{
					"app.juju.is/uuid": "uniqueid1",
				},
			},
		}
		stsApp2 := v1.StatefulSet{
			ObjectMeta: metav1.ObjectMeta{
				Name: "app2",
				Annotations: map[string]string{
					"app.juju.is/uuid": "uniqueid2",
				},
			},
		}
		stsApp3 := v1.StatefulSet{
			ObjectMeta: metav1.ObjectMeta{
				Name: "app3",
				Annotations: map[string]string{
					"app.juju.is/uuid": "uniqueid3",
				},
			},
		}
		_, err = k8sClient.AppsV1().StatefulSets(model1.Name()).Create(context.Background(), &stsApp1, metav1.CreateOptions{})
		c.Assert(err, gc.IsNil)
		_, err = k8sClient.AppsV1().StatefulSets(model1.Name()).Create(context.Background(), &stsApp2, metav1.CreateOptions{})
		c.Assert(err, gc.IsNil)
		_, err = k8sClient.AppsV1().StatefulSets(model1.Name()).Create(context.Background(), &stsApp3, metav1.CreateOptions{})
		c.Assert(err, gc.IsNil)

		return func(appName string, model *Model) (string, error) {
			sts, err := k8sClient.AppsV1().StatefulSets(model.Name()).Get(context.Background(), appName, metav1.GetOptions{})
			if err != nil {
				return "", err
			}
			annotations := sts.Annotations
			v, ok := annotations["app.juju.is/uuid"]
			c.Assert(ok, gc.Equals, true)
			return v, nil
		}
	}

	err = PopulateApplicationStorageUniqueID(s.pool, getStorageUniqueID(newFakeK8sClient))
	logger.Infof("err is %s", err)
	c.Assert(err, gc.IsNil)
}

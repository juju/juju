// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/mgo/v3/bson"
	"github.com/juju/mgo/v3/txn"

	"github.com/juju/juju/pki/ssh"
)

// Until we add 3.0 upgrade steps, keep static analysis happy.
var _ = func() {
	_ = applyToAllModelSettings(nil, nil)
}

// AppAndStorageID represents an application with its id, name, and storage id.
// It is used for backfilling an application's storage id during a controller upgrade.
type AppAndStorageID struct {
	Id              string
	Name            string
	StorageUniqueID string
}

// runForAllModelStates will run runner function for every model passing a state
// for that model.
//
//nolint:unused
func runForAllModelStates(pool *StatePool, runner func(st *State) error) error {
	st, err := pool.SystemState()
	if err != nil {
		return errors.Trace(err)
	}
	models, closer := st.db().GetCollection(modelsC)
	defer closer()

	var modelDocs []bson.M
	err = models.Find(nil).Select(bson.M{"_id": 1}).All(&modelDocs)
	if err != nil {
		return errors.Annotate(err, "failed to read models")
	}

	for _, modelDoc := range modelDocs {
		modelUUID := modelDoc["_id"].(string)
		model, err := pool.Get(modelUUID)
		if err != nil {
			return errors.Annotatef(err, "failed to open model %q", modelUUID)
		}
		defer func() {
			model.Release()
		}()
		if err := runner(model.State); err != nil {
			return errors.Annotatef(err, "model UUID %q", modelUUID)
		}
	}
	return nil
}

// applyToAllModelSettings iterates the model settings documents and applies the
// passed in function to them.  If the function returns 'true' it indicates the
// settings have been modified, and they should be written back to the
// database.
// Note that if there are any problems with updating settings, then none of the
// changes will be applied, as they are all updated in a single transaction.
func applyToAllModelSettings(st *State, change func(*settingsDoc) (bool, error)) error {
	uuids, err := st.AllModelUUIDs()
	if err != nil {
		return errors.Trace(err)
	}

	coll, closer := st.db().GetRawCollection(settingsC)
	defer closer()

	var ids []string
	for _, uuid := range uuids {
		ids = append(ids, uuid+":e")
	}

	iter := coll.Find(bson.M{"_id": bson.M{"$in": ids}}).Iter()
	defer iter.Close()

	var ops []txn.Op
	var doc settingsDoc
	for iter.Next(&doc) {
		settingsChanged, err := change(&doc)
		if err != nil {
			return errors.Trace(err)
		}
		if settingsChanged {
			ops = append(ops, txn.Op{
				C:      settingsC,
				Id:     doc.DocID,
				Assert: txn.DocExists,
				Update: bson.M{"$set": bson.M{"settings": doc.Settings}},
			})
		}
	}
	if err := iter.Close(); err != nil {
		return errors.Trace(err)
	}
	if len(ops) > 0 {
		return errors.Trace(st.runRawTransaction(ops))
	}
	return nil
}

// AddVirtualHostKeys creates virtual host keys for CAAS units and machines.
func AddVirtualHostKeys(pool *StatePool) error {
	st, err := pool.SystemState()
	if err != nil {
		return errors.Trace(err)
	}

	virtualHostKeysCollection, vhkCloser := st.db().GetRawCollection(virtualHostKeysC)
	defer vhkCloser()
	virtualHostKeys := []virtualHostKeyDoc{}
	err = virtualHostKeysCollection.Find(nil).All(&virtualHostKeys)
	if err != nil {
		return errors.Annotatef(err, "cannot get all virtual host keys")
	}

	hostKeyMap := map[string]struct{}{}
	for _, virtualHostKey := range virtualHostKeys {
		hostKeyMap[virtualHostKey.DocId] = struct{}{}
	}

	machinesCollection, machineCloser := st.db().GetRawCollection(machinesC)
	defer machineCloser()
	mdocs := machineDocSlice{}
	err = machinesCollection.Find(nil).All(&mdocs)
	if err != nil {
		return errors.Annotatef(err, "cannot get all machines")
	}

	var ops []txn.Op
	for _, doc := range mdocs {
		machineLookup := ensureModelUUID(doc.ModelUUID, machineHostKeyID(doc.Id))
		if _, ok := hostKeyMap[machineLookup]; ok {
			continue
		}
		key, err := ssh.NewMarshalledED25519()
		if err != nil {
			return errors.Trace(err)
		}
		addOps, err := newMachineVirtualHostKeysOps(doc.ModelUUID, doc.Id, key)
		if err != nil {
			return errors.Trace(err)
		}
		ops = append(ops, addOps...)
	}

	err = runForAllModelStates(pool, func(st *State) error {
		model, err := st.Model()
		if err != nil {
			return errors.Trace(err)
		}

		if model.Type() == ModelTypeCAAS {
			// add host keys for CaaS units.
			units, err := st.allUnits()
			if err != nil {
				return errors.Trace(err)
			}
			for _, unit := range units {
				unitLookup := ensureModelUUID(st.ModelUUID(), unitHostKeyID(unit.Tag().Id()))
				if _, ok := hostKeyMap[unitLookup]; ok {
					continue
				}
				key, err := ssh.NewMarshalledED25519()
				if err != nil {
					return errors.Trace(err)
				}
				addOps, err := newUnitVirtualHostKeysOps(st.ModelUUID(), unit.Tag().Id(), key)
				if err != nil {
					return errors.Trace(err)
				}
				ops = append(ops, addOps...)
			}
		}
		return nil
	})

	if err != nil {
		return errors.Trace(err)
	}
	return st.runRawTransaction(ops)
}
func SplitMigrationStatusMessages(pool *StatePool) error {
	type legacyModelMigStatusDoc struct {
		// These are the same as the ids as migrationsC.
		// "uuid:sequence".
		Id string `bson:"_id"`

		// StartTime holds the time the migration started (stored as per
		// UnixNano).
		StartTime int64 `bson:"start-time"`

		// StartTime holds the time the migration reached the SUCCESS
		// phase (stored as per UnixNano).
		SuccessTime int64 `bson:"success-time"`

		// EndTime holds the time the migration reached a terminal (end)
		// phase (stored as per UnixNano).
		EndTime int64 `bson:"end-time"`

		// Phase holds the current migration phase. This should be one of
		// the string representations of the core/migrations.Phase
		// constants.
		Phase string `bson:"phase"`

		// PhaseChangedTime holds the time that Phase last changed (stored
		// as per UnixNano).
		PhaseChangedTime int64 `bson:"phase-changed-time"`

		// StatusMessage holds a human readable message about the
		// progress of the migration.
		StatusMessage string `bson:"status-message"`
	}

	st, err := pool.SystemState()
	if err != nil {
		return errors.Trace(err)
	}

	migStatus, closer := st.db().GetCollection(migrationsStatusC)
	defer closer()

	iter := migStatus.Find(nil).Iter()
	defer iter.Close()

	var ops []txn.Op
	var legacyStatusDoc legacyModelMigStatusDoc
	for iter.Next(&legacyStatusDoc) {
		if legacyStatusDoc.StatusMessage == "" {
			continue
		}

		id := legacyStatusDoc.Id

		messageDoc := modelMigStatusMessageDoc{
			Id:            id,
			StatusMessage: legacyStatusDoc.StatusMessage,
		}

		ops = append(ops, txn.Op{
			C:      migrationsStatusMessageC,
			Id:     id,
			Assert: txn.DocMissing,
			Insert: messageDoc,
		}, txn.Op{
			C:      migrationsStatusC,
			Id:     id,
			Assert: txn.DocExists,
			Update: bson.D{{"$unset", bson.D{{"status-message", nil}}}},
		})
	}
	return st.runRawTransaction(ops)
}

// PopulateApplicationStorageUniqueID has the responsibility of populating the
// `storage-unique-id` field in the application document.
func PopulateApplicationStorageUniqueID(
	pool *StatePool,
	getStorageUniqueIDs func(
		ctx context.Context,
		applications []AppAndStorageID,
		model *Model,
	) ([]AppAndStorageID, error),
) error {
	// Run for each model because we want to backfill for every application.
	return runForAllModelStates(pool, func(st *State) error {
		model, err := st.Model()
		if err != nil {
			return errors.Trace(err)
		}
		logger.Debugf("trying to populate storage unique ID for apps in model %q", model.Name())

		if model.Type() != ModelTypeCAAS {
			logger.Debugf("skipping because model %q is not a k8s model", model.Name())
			return nil
		}

		applicationsColl, closer := st.db().GetCollection(applicationsC)
		defer closer()

		// Fetch the list of applications with an empty storage unique ID.
		// This ensures we don't repeat the upgrade for applications that have
		// been populated with a storage unique ID.
		query := bson.M{"storage-unique-id": bson.M{"$exists": false}}
		fields := bson.M{"_id": 1, "name": 1}
		iter := applicationsColl.Find(query).Select(fields).Iter()
		defer iter.Close()

		apps := make([]AppAndStorageID, 0)

		var app bson.M
		for iter.Next(&app) {
			apps = append(apps, AppAndStorageID{
				Id:   app["_id"].(string),
				Name: app["name"].(string),
			})
		}

		logger.Debugf("have %d apps to populate storage unique IDs", len(apps))

		appsWithStorageUniqueIDs, err := getStorageUniqueIDs(context.Background(), apps, model)
		if err != nil {
			return errors.Annotate(err, "getting storage unique IDs")
		}

		var ops []txn.Op
		for _, a := range appsWithStorageUniqueIDs {
			ops = append(ops, txn.Op{
				C:      applicationsC,
				Id:     a.Id,
				Assert: txn.DocExists,
				Update: bson.D{{"$set", bson.D{
					{"storage-unique-id", a.StorageUniqueID},
				}}},
			})
		}
		return st.runRawTransaction(ops)
	})
}

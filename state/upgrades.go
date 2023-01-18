// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/errors"
	"github.com/juju/mgo/v3/bson"
	"github.com/juju/mgo/v3/txn"
)

// Until we add 3.0 upgrade steps, keep static analysis happy.
var _ = func() {
	_ = runForAllModelStates(nil, nil)
	_ = applyToAllModelSettings(nil, nil)
}

// runForAllModelStates will run runner function for every model passing a state
// for that model.
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

// CorrectCharmOriginsMultiAppSingleCharm corrects application charm origins
// where the charm has been downloaded, however the origin was not updated
// with ID and Hash data. Per LP 1999060, usually seen with multi application
// using same charm bundles.
func CorrectCharmOriginsMultiAppSingleCharm(pool *StatePool) error {
	return errors.Trace(runForAllModelStates(pool, func(st *State) error {
		col, closer := st.db().GetCollection(applicationsC)
		defer closer()

		var docsWithID, docsWithoutID []applicationDoc
		if err := col.Find(bson.D{
			{"charm-origin.id", bson.M{"$eq": ""}},
			{"charm-origin.source", bson.M{"$eq": "charm-hub"}},
		}).All(&docsWithoutID); err != nil {
			return errors.Trace(err)
		}
		if len(docsWithoutID) == 0 {
			return nil // nothing to do
		}
		if err := col.Find(bson.D{
			{"charm-origin.id", bson.M{"$ne": ""}},
			{"charm-origin.source", bson.M{"$eq": "charm-hub"}},
		}).All(&docsWithID); err != nil {
			return errors.Trace(err)
		}

		charmOrigins := make(map[string]CharmOrigin, len(docsWithID))
		for _, doc := range docsWithID {
			if doc.CharmURL == nil {
				return errors.Errorf("application %q missing charm url", doc.Name)
			}
			charmOrigins[*doc.CharmURL] = doc.CharmOrigin
		}

		var ops []txn.Op
		for _, doc := range docsWithoutID {
			if doc.CharmURL == nil {
				return errors.Errorf("application %q missing charm url", doc.Name)
			}
			if origin, ok := charmOrigins[*doc.CharmURL]; ok {
				ops = append(ops, txn.Op{
					C:      applicationsC,
					Id:     doc.DocID,
					Assert: txn.DocExists,
					Update: bson.D{{"$set", bson.D{{"charm-origin.id", origin.ID},
						{"charm-origin.hash", origin.Hash}}}},
				})
			} else {
				return errors.Errorf("application %q missing charm origin for %q", doc.Name, *doc.CharmURL)
			}
		}

		if len(ops) > 0 {
			return errors.Trace(st.runRawTransaction(ops))
		}
		return nil
	}))
}

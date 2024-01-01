// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"strings"

	"github.com/juju/errors"
	"github.com/juju/mgo/v3/bson"
	"github.com/juju/mgo/v3/txn"
	"github.com/juju/names/v5"
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

// ConvertApplicationOfferTokenKeys updates application offer remote entity
// keys to have the offer uuid rather than the name.
func ConvertApplicationOfferTokenKeys(pool *StatePool) error {
	return errors.Trace(runForAllModelStates(pool, func(st *State) error {
		remoteEntities, closer := st.db().GetCollection(remoteEntitiesC)
		defer closer()

		var docs []bson.M
		if err := remoteEntities.Find(
			bson.D{{"_id", bson.D{{"$regex", names.ApplicationOfferTagKind + "-.*"}}}},
		).All(&docs); err != nil {
			return errors.Annotate(err, "failed to read remote entity docs")
		}

		appOffers := NewApplicationOffers(st)
		var ops []txn.Op
		for _, doc := range docs {
			oldID := doc["_id"].(string)
			offerName := strings.TrimPrefix(st.localID(oldID), names.ApplicationOfferTagKind+"-")
			offer, err := appOffers.ApplicationOffer(offerName)
			if errors.Is(err, errors.NotFound) {
				// Already been updated.
				continue
			}
			if err != nil {
				return errors.Trace(err)
			}
			newID := st.docID(names.NewApplicationOfferTag(offer.OfferUUID).String())
			doc["_id"] = newID
			ops = append(ops, txn.Op{
				C:      remoteEntitiesC,
				Id:     oldID,
				Remove: true,
			}, txn.Op{
				C:      remoteEntitiesC,
				Id:     newID,
				Insert: doc,
			})
		}
		if len(ops) > 0 {
			return errors.Trace(st.runRawTransaction(ops))
		}
		return nil
	}))
}

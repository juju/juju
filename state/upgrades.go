// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/errors"
	"github.com/juju/mgo/v3/bson"
	"github.com/juju/mgo/v3/txn"

	"github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/container"
)

// Until we add 3.0 upgrade steps, keep static analysis happy.
var _ = func() {
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

func FillInEmptyCharmhubTracks(pool *StatePool) error {
	return errors.Trace(runForAllModelStates(pool, func(st *State) error {
		applications, closer := st.db().GetCollection(applicationsC)
		defer closer()

		var docs []bson.M
		if err := applications.Find(bson.D{}).All(&docs); err != nil {
			return errors.Annotatef(err, "failed to read applications")
		}

		var ops []txn.Op
		for _, doc := range docs {
			charmOrigin := doc["charm-origin"].(bson.M)
			if charmOrigin["source"] != charm.CharmHub.String() {
				continue
			}

			channel := charmOrigin["channel"].(bson.M)
			if _, ok := channel["track"]; ok {
				continue
			}

			ops = append(ops, txn.Op{
				C:      applicationsC,
				Id:     doc["_id"],
				Assert: txn.DocExists,
				Update: bson.M{"$set": bson.M{"charm-origin.channel.track": "latest"}},
			})
		}
		if len(ops) > 0 {
			return errors.Trace(st.runRawTransaction(ops))
		}
		return nil
	}))
}

// AssignArchToContainers assigns an architecture to container
// instance data based on their host.
func AssignArchToContainers(pool *StatePool) error {
	return errors.Trace(runForAllModelStates(pool, func(st *State) error {
		instData, closer := st.db().GetCollection(instanceDataC)
		defer closer()

		var containers []instanceData
		if err := instData.Find(bson.M{"machineid": bson.M{"$regex": "[0-9]+/[a-z]+/[0-9]+"}}).All(&containers); err != nil {
			return errors.Annotatef(err, "failed to read container instance data")
		}
		// Nothing to do if no containers in the model.
		if len(containers) == 0 {
			return nil
		}

		var machines []instanceData
		if err := instData.Find(bson.D{{"machineid", bson.D{{"$regex", "^[0-9]+$"}}}}).All(&machines); err != nil {
			return errors.Annotatef(err, "failed to read machine instance data")
		}

		machineArch := make(map[string]string, len(machines))

		for _, machine := range machines {
			if machine.Arch == nil {
				logger.Errorf("no architecture for machine %q", machine.MachineId)
			}
			machineArch[machine.MachineId] = *machine.Arch
		}

		var ops []txn.Op
		for _, cont := range containers {
			if cont.Arch != nil {
				continue
			}
			mID := container.ParentId(cont.MachineId)
			a, ok := machineArch[mID]
			if !ok {
				logger.Errorf("no instance data for machine %q, but has container %q", mID, cont.MachineId)
				continue
			}
			ops = append(ops, txn.Op{
				C:      instanceDataC,
				Id:     cont.DocID,
				Assert: txn.DocExists,
				Update: bson.M{"$set": bson.M{"arch": &a}},
			})
		}

		if len(ops) > 0 {
			return errors.Trace(st.runRawTransaction(ops))
		}
		return nil
	}))
}

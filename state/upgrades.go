// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"gopkg.in/juju/names.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/status"
)

var upgradesLogger = loggo.GetLogger("juju.state.upgrade")

func AddPreferredAddressesToMachines(st *State) error {
	machines, err := st.AllMachines()
	if err != nil {
		return errors.Trace(err)
	}

	for _, machine := range machines {
		if machine.Life() == Dead {
			continue
		}
		// Setting the addresses is enough to trigger setting the preferred
		// addresses.
		err = machine.SetMachineAddresses(machine.MachineAddresses()...)
		if err != nil {
			return errors.Trace(err)
		}
		err := machine.SetProviderAddresses(machine.ProviderAddresses()...)
		if err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

// runForAllEnvStates will run runner function for every env passing a state
// for that env.
func runForAllEnvStates(st *State, runner func(st *State) error) error {
	environments, closer := st.getCollection(modelsC)
	defer closer()

	var envDocs []bson.M
	err := environments.Find(nil).Select(bson.M{"_id": 1}).All(&envDocs)
	if err != nil {
		return errors.Annotate(err, "failed to read models")
	}

	for _, envDoc := range envDocs {
		modelUUID := envDoc["_id"].(string)
		envSt, err := st.ForModel(names.NewModelTag(modelUUID))
		if err != nil {
			return errors.Annotatef(err, "failed to open model %q", modelUUID)
		}
		defer envSt.Close()
		if err := runner(envSt); err != nil {
			return errors.Annotatef(err, "model UUID %q", modelUUID)
		}
	}
	return nil
}

// AddFilesystemStatus ensures each filesystem has a status doc.
func AddFilesystemStatus(st *State) error {
	return runForAllEnvStates(st, func(st *State) error {
		filesystems, err := st.AllFilesystems()
		if err != nil {
			return errors.Trace(err)
		}
		var ops []txn.Op
		for _, filesystem := range filesystems {
			_, err := filesystem.Status()
			if err == nil {
				continue
			}
			if !errors.IsNotFound(err) {
				return errors.Annotate(err, "getting status")
			}
			status, err := upgradingFilesystemStatus(st, filesystem)
			if err != nil {
				return errors.Annotate(err, "deciding filesystem status")
			}
			ops = append(ops, createStatusOp(st, filesystem.globalKey(), statusDoc{
				Status:  status,
				Updated: st.clock.Now().UnixNano(),
			}))
		}
		if len(ops) > 0 {
			return errors.Trace(st.runTransaction(ops))
		}
		return nil
	})
}

// If the filesystem has not been provisioned, then it should be Pending;
// if it has been provisioned, but there is an unprovisioned attachment, then
// it should be Attaching; otherwise it is Attached.
func upgradingFilesystemStatus(st *State, filesystem Filesystem) (status.Status, error) {
	if _, err := filesystem.Info(); errors.IsNotProvisioned(err) {
		return status.Pending, nil
	}
	attachments, err := st.FilesystemAttachments(filesystem.FilesystemTag())
	if err != nil {
		return "", errors.Trace(err)
	}
	for _, attachment := range attachments {
		_, err := attachment.Info()
		if errors.IsNotProvisioned(err) {
			return status.Attaching, nil
		}
	}
	return status.Attached, nil
}

// MigrateSettingsSchema migrates the schema of the settings collection,
// moving non-reserved keys at the top-level into a subdoc, and introducing
// a top-level "version" field with the initial value matching txn-revno.
//
// This migration takes place both before and after model-uuid migration,
// to get the correct txn-revno value.
func MigrateSettingsSchema(st *State) error {
	coll, closer := st.getRawCollection(settingsC)
	defer closer()

	upgradesLogger.Debugf("migrating schema of the %s collection", settingsC)
	iter := coll.Find(nil).Iter()
	defer iter.Close()

	var ops []txn.Op
	var doc bson.M
	for iter.Next(&doc) {
		if !settingsDocNeedsMigration(doc) {
			continue
		}

		id := doc["_id"]
		txnRevno := doc["txn-revno"].(int64)

		// Remove reserved attributes; we'll move the remaining
		// ones to the "settings" subdoc.
		delete(doc, "model-uuid")
		delete(doc, "_id")
		delete(doc, "txn-revno")
		delete(doc, "txn-queue")

		// If there exists a setting by the name "settings",
		// we must remove it first, or it will collide with
		// the dotted-notation $sets.
		if _, ok := doc["settings"]; ok {
			ops = append(ops, txn.Op{
				C:      settingsC,
				Id:     id,
				Assert: txn.DocExists,
				Update: bson.D{{"$unset", bson.D{{"settings", 1}}}},
			})
		}

		var update bson.D
		for key, value := range doc {
			if key != "settings" && key != "version" {
				// Don't try to unset these fields,
				// as we've unset "settings" above
				// already, and we'll overwrite
				// "version" below.
				update = append(update, bson.DocElem{
					"$unset", bson.D{{key, 1}},
				})
			}
			update = append(update, bson.DocElem{
				"$set", bson.D{{"settings." + key, value}},
			})
		}
		if len(update) == 0 {
			// If there are no settings, then we need
			// to add an empty "settings" map so we
			// can tell for next time that migration
			// is complete, and don't move the "version"
			// field we add.
			update = bson.D{{
				"$set", bson.D{{"settings", bson.M{}}},
			}}
		}
		update = append(update, bson.DocElem{
			"$set", bson.D{{"version", txnRevno}},
		})

		ops = append(ops, txn.Op{
			C:      settingsC,
			Id:     id,
			Assert: txn.DocExists,
			Update: update,
		})
	}
	if err := iter.Err(); err != nil {
		return errors.Trace(err)
	}
	return st.runRawTransaction(ops)
}

func settingsDocNeedsMigration(doc bson.M) bool {
	// It is not possible for there to exist a settings value
	// with type bson.M, so we know that it is the new settings
	// field and not just a setting with the name "settings".
	if _, ok := doc["settings"].(bson.M); ok {
		return false
	}
	return true
}

func addDefaultBindingsToServices(st *State) error {
	applications, err := st.AllApplications()
	if err != nil {
		return errors.Trace(err)
	}

	upgradesLogger.Debugf("adding default endpoint bindings to applications (where missing)")
	ops := make([]txn.Op, 0, len(applications))
	for _, application := range applications {
		ch, _, err := application.Charm()
		if err != nil {
			return errors.Annotatef(err, "cannot get charm for application %q", application.Name())
		}
		if _, err := application.EndpointBindings(); err == nil {
			upgradesLogger.Debugf("application %q already has bindings (skipping)", application.Name())
			continue
		} else if !errors.IsNotFound(err) {
			return errors.Annotatef(err, "checking application %q for existing bindings", application.Name())
		}
		// Passing nil for the bindings map will use the defaults.
		createOp, err := createEndpointBindingsOp(st, application.globalKey(), nil, ch.Meta())
		if err != nil {
			return errors.Annotatef(err, "setting default endpoint bindings for application %q", application.Name())
		}
		ops = append(ops, createOp)
	}
	return st.runTransaction(ops)
}

// AddDefaultEndpointBindingsToServices adds default endpoint bindings for each
// service. As long as the service has a charm URL set, each charm endpoint will
// be bound to the default space.
func AddDefaultEndpointBindingsToServices(st *State) error {
	return runForAllEnvStates(st, addDefaultBindingsToServices)
}

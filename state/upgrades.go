// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/mgo/v3"
	"github.com/juju/mgo/v3/bson"
	"github.com/juju/mgo/v3/txn"

	"github.com/juju/juju/core/application"
	"github.com/juju/juju/core/network"
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
	models, closer, err := st.db().GetCollection(modelsC)
	if err != nil {
		return errors.Trace(err)
	}
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

	coll, closer, err := st.db().GetRawCollection(settingsC)
	if err != nil {
		return errors.Trace(err)
	}
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

	migStatus, closer, err := st.db().GetCollection(migrationsStatusC)
	if err != nil {
		return errors.Trace(err)
	}
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

// OpenControllerAPIPort runs an upgrade to open the controller api port
// on the controller units.
func OpenControllerAPIPort(pool *StatePool) error {
	st, err := pool.SystemState()
	if err != nil {
		return errors.Trace(err)
	}

	controllerCfg, err := st.ControllerConfig()
	if err != nil {
		return errors.Trace(err)
	}
	apiPort := controllerCfg.APIPort()

	unitsColl, closer, err := st.db().GetRawCollection(unitsC)
	if err != nil {
		return errors.Trace(err)
	}
	defer closer()

	var controllerUnits []unitDoc
	err = unitsColl.Find(bson.M{"application": controllerAppName}).Select(bson.M{"name": 1}).All(&controllerUnits)
	if err != nil {
		return errors.Annotatef(err, "cannot get controller units")
	}
nextUnit:
	for _, unitDoc := range controllerUnits {
		// Ideally we'd want to do this work using bson maps to avoid
		// using state objects but the logic is complicated enough that
		// it's viable at the moment to use the existing state code to
		// manipulate the port ranges.
		controllerUnit, err := st.Unit(unitDoc.Name)
		if err != nil {
			return errors.Trace(err)
		}

		pcp, err := controllerUnit.OpenedPortRanges()
		if err != nil {
			return errors.Trace(err)
		}
		for _, pr := range pcp.UniquePortRanges() {
			if pr.Protocol != "tcp" {
				continue
			}
			if apiPort >= pr.FromPort && apiPort <= pr.ToPort {
				continue nextUnit
			}
		}
		pcp.Open("", network.PortRange{
			FromPort: apiPort,
			ToPort:   apiPort,
			Protocol: "tcp",
		})

		if err = st.ApplyOperation(pcp.Changes()); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

// ExposeControllerApplication runs an upgrade to expose the
// controller application.
func ExposeControllerApplication(pool *StatePool) error {
	st, err := pool.SystemState()
	if err != nil {
		return errors.Trace(err)
	}

	appsColl, closer, err := st.db().GetCollection(applicationsC)
	if err != nil {
		return errors.Trace(err)
	}
	defer closer()
	var appData bson.M
	err = appsColl.Find(bson.M{"_id": controllerAppName}).Select(bson.M{"exposed": 1}).One(&appData)
	if err == mgo.ErrNotFound {
		// If there was a problem deploying the controller charm at bootstrap
		// then there won't be a record to update.
		logger.Warningf("controller application not found, skipping expose controller application upgrade")
		return nil
	}
	if err != nil {
		return errors.Trace(err)
	}
	exposed, _ := appData["exposed"].(bool)
	if exposed {
		return nil
	}

	ops := []txn.Op{{
		C:      applicationsC,
		Id:     st.docID(controllerAppName),
		Assert: txn.DocExists,
		Update: bson.D{{"$set", bson.D{
			{"exposed", true},
		}}},
	}}
	if err = st.runRawTransaction(ops); err != nil {
		return errors.Trace(err)
	}
	return nil
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

		applicationsColl, closer, err := st.db().GetCollection(applicationsC)
		if err != nil {
			return errors.Trace(err)
		}
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

// ConvertScalingToCurrentOperationEnumField has the responsibility of converting
// the "provisioning-state.scaling" field to "provisioning-state.current-operation"
// enum field. It also removes "provisioning-state.scaling" which is no longer
// used.
func ConvertScalingToCurrentOperationEnumField(pool *StatePool) error {
	return runForAllModelStates(pool, func(st *State) error {
		model, err := st.Model()
		if err != nil {
			return errors.Trace(err)
		}
		logger.Debugf("trying to convert scaling to current-operation field for apps in model %q",
			model.Name())

		if model.Type() != ModelTypeCAAS {
			logger.Debugf("skipping because model %q is not a k8s model",
				model.Name())
			return nil
		}

		applicationsColl, closer, err := st.db().GetCollection(applicationsC)
		if err != nil {
			return errors.Trace(err)
		}
		defer closer()

		query := bson.M{"provisioning-state.scaling": bson.M{"$exists": true}}
		fields := bson.M{"_id": 1, "provisioning-state.scaling": 1}

		iter := applicationsColl.Find(query).Select(fields).Iter()
		defer iter.Close()

		var app bson.M
		var ops []txn.Op
		for iter.Next(&app) {
			id, ok := app["_id"].(string)
			if !ok {
				return errors.Errorf("expected app id string, got \"%v\" (%T)", id, id)
			}

			psRaw, ok := app["provisioning-state"]
			if !ok || psRaw == nil {
				continue
			}

			ps, ok := psRaw.(bson.M)
			if !ok {
				return errors.Errorf("expected app provisioning-state bson.M, got \"%v\" (%T)", psRaw, psRaw)
			}

			scalingRaw, ok := ps["scaling"]
			if !ok || scalingRaw == nil {
				ops = append(ops, txn.Op{
					C:      applicationsC,
					Id:     id,
					Assert: txn.DocExists,
					Update: bson.D{{"$unset", bson.D{
						{"provisioning-state.scaling", ""},
					}}},
				})
				continue
			}

			scaling, ok := scalingRaw.(bool)
			if !ok {
				return errors.Errorf("expected app scaling bool, got \"%v\" (%T)", scalingRaw, scalingRaw)
			}

			update := bson.D{
				{"$unset", bson.D{{"provisioning-state.scaling", ""}}},
			}

			if scaling {
				update = append(update, bson.DocElem{
					"$set", bson.D{
						{"provisioning-state.current-operation",
							application.ScaleOperation},
					},
				})
			}

			ops = append(ops, txn.Op{
				C:      applicationsC,
				Id:     id,
				Assert: txn.DocExists,
				Update: update,
			})
		}
		return st.runRawTransaction(ops)
	})
}

// sshProxyCollectionNames holds the names of the MongoDB collections that
// were used by the controller-proxied SSH feature. The feature was removed
// from the 3.6 line but the collections were left behind in upgraded
// controllers' model databases. RemoveSSHProxyArtefacts removes the
// orphaned documents from every model so no dead data remains.
//
// The collection constants are defined locally here rather than in
// allcollections.go because the collections are no longer registered as
// known collections; the upgrade step only needs the names to remove the
// leftover documents.
//
// These collections were model-scoped (non-global): documents share the
// single juju database and are distinguished by a "model-uuid" field and
// model-UUID-prefixed _id. The upgrade step therefore removes documents
// per-model using a model-uuid filter rather than dropping the underlying
// MongoDB collection, which would destroy every model's documents at
// once.
var sshProxyCollectionNames = []string{
	"virtualhostkeys",
	"sshrequests",
}

// sshProxyCleanupKind is the cleanupKind value that was used for expired
// SSH connection requests. The cleanup handler was removed along with the
// rest of the feature, so any leftover cleanup documents of this kind
// would fail (or attempt to access a dropped collection) when processed.
const sshProxyCleanupKind cleanupKind = "sshConnRequests"

// sshProxyControllerConfigKeys are the controller configuration keys that
// were added for the controller-proxied SSH feature. They are no longer
// part of the controller config schema, so any values left behind in the
// controller settings document by an upgraded controller are orphaned and
// would be surfaced to clients as unexpected config entries.
var sshProxyControllerConfigKeys = []string{
	"ssh-server-port",
	"ssh-max-concurrent-connections",
}

// sshProxyServerPort is the default port that the removed controller-proxied
// SSH server listened on. It was opened on every controller unit by the
// enableHA path (see state/enableha.go addControllerUnitOps) and persisted in
// the unit's opened port ranges. After the feature was removed the port range
// is orphaned: no code closes it, so the firewaller keeps it open. The upgrade
// step closes it on every controller unit so the firewaller reverts the
// security-group change.
const sshProxyServerPort = 17022

// RemoveSSHProxyArtefacts removes all state left behind by the removed
// controller-proxied SSH feature from the controller:
//
//   - the virtualhostkeys and sshrequests documents are removed from every
//     model in the pool,
//   - any leftover "sshConnRequests" cleanup documents are removed so they
//     cannot trigger a handler that no longer exists,
//   - the orphaned ssh-server-port and ssh-max-concurrent-connections
//     controller config keys are removed from the controller settings
//     document, and
//   - the ssh server port (17022) is closed on every controller unit so the
//     firewaller reverts the security-group opening made by the removed
//     enableHA path.
//
// Each operation is a no-op where the underlying data does not exist
// (removing absent documents or settings keys, or closing an already-closed
// port range, is harmless), so the step is idempotent and safe to run on
// controllers that never had the feature.
//
// The virtualhostkeys and sshrequests collections were model-scoped, so
// documents are removed per-model with a model-uuid filter rather than
// dropping the underlying MongoDB collection, which is shared across all
// models in the single-juju-database layout used since 3.6.
func RemoveSSHProxyArtefacts(pool *StatePool) error {
	if err := dropSSHProxyCollections(pool); err != nil {
		return errors.Trace(err)
	}
	if err := closeSSHProxyControllerPort(pool); err != nil {
		return errors.Trace(err)
	}
	return errors.Trace(removeSSHProxyControllerConfig(pool))
}

// dropSSHProxyCollections removes the orphaned virtualhostkeys and
// sshrequests documents from every model in the pool and removes any
// leftover "sshConnRequests" cleanup documents so they cannot trigger a
// handler that no longer exists.
func dropSSHProxyCollections(pool *StatePool) error {
	return runForAllModelStates(pool, func(st *State) error {
		for _, name := range sshProxyCollectionNames {
			coll, closer, err := st.db().GetRawCollection(name)
			if err != nil {
				return errors.Trace(err)
			}
			// Remove only this model's documents. The collection is
			// shared across all models in the single-juju-database
			// layout, so dropping the whole collection would destroy
			// other models' data.
			if _, err := coll.RemoveAll(bson.D{{Name: "model-uuid", Value: st.ModelUUID()}}); err != nil {
				closer()
				return errors.Annotatef(err, "removing %q documents for model %q", name, st.ModelUUID())
			}
			closer()
		}
		if err := st.removeSSHProxyCleanupDocs(); err != nil {
			return errors.Trace(err)
		}
		return nil
	})
}

// closeSSHProxyControllerPort closes the ssh server port (17022) on every
// controller unit. The removed enableHA path opened this port on each
// controller unit's opened port ranges; with the feature gone no code
// closes it, so the firewaller keeps the security-group entry open. Closing
// the range here lets the firewaller revert it.
//
// Only the controller (system) model has controller units, so this only
// needs to run against the system state.
func closeSSHProxyControllerPort(pool *StatePool) error {
	st, err := pool.SystemState()
	if err != nil {
		return errors.Trace(err)
	}
	unitsColl, closer, err := st.db().GetRawCollection(unitsC)
	if err != nil {
		return errors.Trace(err)
	}
	defer closer()
	var controllerUnits []unitDoc
	if err := unitsColl.Find(bson.M{"application": controllerAppName}).Select(bson.M{"name": 1}).All(&controllerUnits); err != nil {
		return errors.Annotatef(err, "cannot get controller units")
	}
	sshPortRange := network.PortRange{
		FromPort: sshProxyServerPort,
		ToPort:   sshProxyServerPort,
		Protocol: "tcp",
	}
	for _, unitDoc := range controllerUnits {
		controllerUnit, err := st.Unit(unitDoc.Name)
		if err != nil {
			return errors.Trace(err)
		}
		pcp, err := controllerUnit.OpenedPortRanges()
		if err != nil {
			return errors.Trace(err)
		}
		// Close is a no-op if the range is not open, so this is safe to
		// run repeatedly and on controllers that never had the feature.
		pcp.Close("", sshPortRange)
		if err := st.ApplyOperation(pcp.Changes()); err != nil {
			return errors.Annotatef(err, "closing ssh proxy port on unit %q", unitDoc.Name)
		}
	}
	return nil
}

// removeSSHProxyControllerConfig removes the orphaned controller-proxied
// SSH configuration keys from the controller settings document. It only
// needs to run once against the system (controller) state, as controller
// config is global rather than per-model.
func removeSSHProxyControllerConfig(pool *StatePool) error {
	st, err := pool.SystemState()
	if err != nil {
		return errors.Trace(err)
	}
	settings, err := readSettings(st.db(), controllersC, ControllerSettingsGlobalKey)
	if err != nil {
		return errors.Annotatef(err, "controller %q", st.ControllerUUID())
	}
	for _, key := range sshProxyControllerConfigKeys {
		settings.Delete(key)
	}
	_, ops := settings.settingsUpdateOps()
	if len(ops) == 0 {
		return nil
	}
	return errors.Trace(settings.write(ops))
}

// removeSSHProxyCleanupDocs removes any leftover cleanup documents for the
// removed "sshConnRequests" cleanup kind. Without this, the cleanup worker
// could pick up such a document and fail trying to run a handler that no
// longer exists against a collection that has been dropped.
func (st *State) removeSSHProxyCleanupDocs() error {
	coll, closer, err := st.db().GetCollection(cleanupsC)
	if err != nil {
		return errors.Trace(err)
	}
	defer closer()
	var docs []cleanupDoc
	if err := coll.Find(bson.D{{Name: "kind", Value: sshProxyCleanupKind}}).All(&docs); err != nil {
		return errors.Annotate(err, "cannot read ssh proxy cleanup documents")
	}
	if len(docs) == 0 {
		return nil
	}
	ops := make([]txn.Op, 0, len(docs))
	for _, doc := range docs {
		ops = append(ops, txn.Op{
			C:      cleanupsC,
			Id:     doc.DocID,
			Remove: true,
		})
	}
	return errors.Trace(st.runRawTransaction(ops))
}

// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"slices"
	"strconv"
	"strings"

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

// FixApplicationCounts repairs remote and local application
// relationcount drift caused by force removing cross model relations. It
// resets relationcount to the number of relations whose endpoints
// reference the application. Local application counts are repaired too:
// removing an orphaned relation asserts on the relationcount of the
// endpoint application, whichever collection it belongs to.
func FixApplicationCounts(pool *StatePool) error {
	return runForAllModelStates(pool, func(st *State) error {
		var repaired, failed []string
		for _, collName := range []string{remoteApplicationsC, applicationsC} {
			apps, closer, err := st.db().GetCollection(collName)
			if err != nil {
				return errors.Trace(err)
			}
			var docs []struct {
				DocID         string `bson:"_id"`
				Name          string `bson:"name"`
				RelationCount int    `bson:"relationcount"`
			}
			err = apps.Find(nil).Select(bson.M{"_id": 1, "name": 1, "relationcount": 1}).All(&docs)
			closer()
			if err != nil {
				return errors.Trace(err)
			}
			// The fix-up is best effort: each application is updated
			// in its own transaction, logging a warning on failure,
			// so a bad document cannot abort the whole upgrade step.
			for _, doc := range docs {
				refCount, err := countRelationsForApplication(st, doc.Name)
				if err != nil {
					return errors.Trace(err)
				}
				if doc.RelationCount == refCount {
					continue
				}
				logger.Debugf("repairing application %q: relationcount %d != %d actual relations", doc.Name, doc.RelationCount, refCount)
				err = st.runRawTransaction([]txn.Op{{
					C:      collName,
					Id:     doc.DocID,
					Assert: txn.DocExists,
					Update: bson.D{{"$set", bson.D{{"relationcount", refCount}}}},
				}})
				if err != nil {
					logger.Warningf("cannot repair relationcount for application %q: %v (manual repair required)", doc.Name, err)
					failed = append(failed, doc.Name)
					continue
				}
				repaired = append(repaired, doc.Name)
			}
		}
		if len(repaired) > 0 {
			logger.Infof("repaired relationcount for %d application(s) in model %q: %v",
				len(repaired), st.ModelUUID(), repaired)
		}
		if len(failed) > 0 {
			logger.Warningf("%d application(s) left for manual repair in model %q: %v",
				len(failed), st.ModelUUID(), failed)
		}
		return nil
	})
}

// RemoveOrphanedApplicationRelations destroys relations when an endpoint
// remote application has no document in the model.
// Only the remote end of a cross-model relation can be left dangling.
// Note: this is best effort and does not guarantee that all orphaned
// relations will be removed. Nor does it guarantee that all child data is
// removed. The key thing is that the relation itself is removed.
//
// NB: the removal operations are bespoke rather than taken from
// Relation.removeOps: those assert on life and unitcount values that
// corrupted relations violate, and their endpoint lookups fail on the
// missing application documents.
func RemoveOrphanedApplicationRelations(pool *StatePool) error {
	return runForAllModelStates(pool, func(st *State) error {
		// collectIDs returns the document ids matched by selector, which
		// runs model-scoped so only this model's documents are considered.
		collectIDs := func(collName string, sel bson.M) ([]string, error) {
			coll, closer, err := st.db().GetCollection(collName)
			if err != nil {
				return nil, errors.Trace(err)
			}
			defer closer()
			var ids []string
			var doc struct {
				DocID string `bson:"_id"`
			}
			iter := coll.Find(sel).Select(bson.M{"_id": 1}).Iter()
			defer iter.Close()
			for iter.Next(&doc) {
				ids = append(ids, doc.DocID)
			}
			if err := iter.Close(); err != nil {
				return nil, errors.Trace(err)
			}
			return ids, nil
		}

		// appendRemoveOps appends to ops a Remove op for every
		// document matching sel in the named collection.
		appendRemoveOps := func(ops []txn.Op, collName string, sel bson.M) ([]txn.Op, error) {
			ids, err := collectIDs(collName, sel)
			if err != nil {
				return nil, errors.Trace(err)
			}
			for _, id := range ids {
				ops = append(ops, txn.Op{C: collName, Id: id, Remove: true})
			}
			return ops, nil
		}

		// apps holds every endpoint application that currently has a document.
		type endpointApp struct {
			coll  string
			docID string
			life  Life
			// relationCount, isConsumerProxy and sourceControllerUUID
			// are only read for remote applications.
			relationCount        int
			isConsumerProxy      bool
			sourceControllerUUID string
		}
		apps := make(map[string]endpointApp)
		for _, collName := range []string{remoteApplicationsC, applicationsC} {
			coll, closer, err := st.db().GetCollection(collName)
			if err != nil {
				return errors.Trace(err)
			}
			var doc struct {
				Name                 string `bson:"name"`
				DocID                string `bson:"_id"`
				Life                 Life   `bson:"life"`
				RelationCount        int    `bson:"relationcount"`
				IsConsumerProxy      bool   `bson:"is-consumer-proxy"`
				SourceControllerUUID string `bson:"source-controller-uuid"`
			}
			iter := coll.Find(nil).Select(bson.M{
				"_id": 1, "name": 1, "life": 1, "relationcount": 1,
				"is-consumer-proxy": 1, "source-controller-uuid": 1,
			}).Iter()
			for iter.Next(&doc) {
				if _, seen := apps[doc.Name]; seen {
					continue
				}
				apps[doc.Name] = endpointApp{
					coll:                 collName,
					docID:                doc.DocID,
					life:                 doc.Life,
					relationCount:        doc.RelationCount,
					isConsumerProxy:      doc.IsConsumerProxy,
					sourceControllerUUID: doc.SourceControllerUUID,
				}
			}
			err = iter.Close()
			closer()
			if err != nil {
				return errors.Trace(err)
			}
		}

		relations, closer, err := st.db().GetCollection(relationsC)
		if err != nil {
			return errors.Trace(err)
		}
		defer closer()

		var doc struct {
			DocID     string `bson:"_id"`
			Key       string `bson:"key"`
			Id        int    `bson:"id"`
			Endpoints []struct {
				ApplicationName string `bson:"applicationname"`
			} `bson:"endpoints"`
		}
		iter := relations.Find(nil).
			Select(bson.M{"_id": 1, "key": 1, "id": 1, "endpoints": 1}).
			Iter()
		defer iter.Close()
		var removed, failed []int
		// decremented tracks, per application, the relation count
		// decrements this step has already applied, so that the count
		// a later orphaned relation sees reflects the removals before
		// it: several orphaned relations can reference the same dying
		// remote application, and only the last one may remove it.
		decremented := make(map[string]int)
	nextRelation:
		for iter.Next(&doc) {
			orphaned := false
			for _, ep := range doc.Endpoints {
				if _, ok := apps[ep.ApplicationName]; !ok {
					orphaned = true
					break
				}
			}
			if !orphaned {
				continue
			}

			var ops []txn.Op
			for _, ep := range doc.Endpoints {
				app, ok := apps[ep.ApplicationName]
				if !ok {
					continue
				}
				if app.coll == remoteApplicationsC &&
					(app.life != Alive || app.isConsumerProxy) &&
					app.relationCount-decremented[ep.ApplicationName] == 1 {
					// Mirror normal relation removal: a dying remote
					// application (or a consumer proxy) whose last
					// relation is removed must itself be removed; unlike
					// local applications there is no cleanup mechanism
					// to do it later.
					remoteApp := &RemoteApplication{st: st, doc: remoteApplicationDoc{
						DocID:                app.docID,
						Name:                 ep.ApplicationName,
						SourceControllerUUID: app.sourceControllerUUID,
					}}
					removeAppOps, err := remoteApp.removeOps(bson.D{{"relationcount", 1}})
					if errors.IsNotFound(err) {
						// A missing external controller refcount must not
						// block the upgrade. Leave the entire relation for
						// manual repair rather than apply incomplete ops.
						logger.Warningf("cannot remove orphaned relation %d: %v (manual repair required)", doc.Id, err)
						failed = append(failed, doc.Id)
						continue nextRelation
					}
					if err != nil {
						return errors.Trace(err)
					}
					ops = append(ops, removeAppOps...)
					continue
				}
				// The endpoint application's relation count must
				// reflect the removed relation. The assertion
				// means a count that is already below one aborts the
				// whole removal below, leaving the relation for manual
				// repair.
				ops = append(ops, txn.Op{
					C:      app.coll,
					Id:     app.docID,
					Assert: bson.D{{"relationcount", bson.D{{"$gt", 0}}}},
					Update: bson.D{{"$inc", bson.D{{"relationcount", -1}}}},
				})
				if app.coll == applicationsC && app.life != Alive {
					// Mirror normal relation removal: a dying
					// application whose last reference is removed
					// must have a cleanup queued. The op is model
					// scoped for the raw transaction below.
					cleanupOp, err := st.newModelScopedCleanupOp(
						cleanupApplication,
						ep.ApplicationName,
						false, // destroyStorage
						false, // force
					)
					if err != nil {
						return errors.Trace(err)
					}
					ops = append(ops, cleanupOp)
				}
			}
			// Removal of the relation and all the documents it owns:
			// scope and settings docs, persisted unit relation state,
			// and any queued cleanups that reference it.
			prefix := "r#" + strconv.Itoa(doc.Id) + "#"
			prefixSel := bson.M{"_id": bson.M{"$regex": "^" + st.docID(prefix)}}
			ops, err := appendRemoveOps(ops, relationScopesC, prefixSel)
			if err != nil {
				return errors.Trace(err)
			}
			ops, err = appendRemoveOps(ops, settingsC, prefixSel)
			if err != nil {
				return errors.Trace(err)
			}
			relState := "relation-state." + strconv.Itoa(doc.Id)
			ids, err := collectIDs(unitStatesC, bson.M{relState: bson.M{"$exists": true}})
			if err != nil {
				return errors.Trace(err)
			}
			for _, id := range ids {
				ops = append(ops, txn.Op{C: unitStatesC, Id: id, Update: bson.D{{"$unset", bson.D{{relState, ""}}}}})
			}
			ops, err = appendRemoveOps(ops, cleanupsC, bson.M{"$or": []bson.M{{
				"kind":   cleanupForceDestroyedRelation,
				"prefix": strconv.Itoa(doc.Id),
			}, {
				"kind":   cleanupRelationSettings,
				"prefix": prefix,
			}}})
			if err != nil {
				return errors.Trace(err)
			}

			// Also clears the relation's status, relation network,
			// remote entity, offer connection, and secret
			// permission records.
			removeChildRecordOps, err := removeRelationChildRecordOps(st, doc.Id, doc.Key)
			if err != nil {
				return errors.Trace(err)
			}
			ops = append(ops, removeChildRecordOps...)

			ops = append(ops, txn.Op{
				C:      relationsC,
				Id:     doc.DocID,
				Assert: txn.DocExists,
				Remove: true,
			})
			logger.Debugf("removing orphaned relation %v: an endpoint application has no document", doc.Id)
			if err := st.runRawTransaction(ops); err != nil {
				logger.Warningf("cannot remove orphaned relation %d: %v (manual repair required)", doc.Id, err)
				failed = append(failed, doc.Id)
			} else {
				removed = append(removed, doc.Id)
				for _, ep := range doc.Endpoints {
					decremented[ep.ApplicationName]++
				}
			}
		}
		if err := iter.Close(); err != nil {
			return errors.Trace(err)
		}
		if len(removed) > 0 {
			logger.Infof("removed %d orphaned relation(s) in model %q: %v",
				len(removed), st.ModelUUID(), removed)
		}
		if len(failed) > 0 {
			logger.Warningf("%d orphaned relation(s) left for manual repair in model %q: %v",
				len(failed), st.ModelUUID(), failed)
		}
		return nil
	})
}

// RemoveOrphanedRelationDocs removes relation scope and settings documents
// whose parent relation no longer exists, and repairs a scope whose
// settings document is missing: the codebase treats a scope doc as a
// guarantee that a settings doc with the same key exists and the relation
// units watcher fails on such a scope. The relation units watcher also
// watches an application settings doc for every endpoint application of
// every relation, so missing ones are recreated wherever their endpoint
// applications still exist.
func RemoveOrphanedRelationDocs(pool *StatePool) error {
	return runForAllModelStates(pool, func(st *State) error {
		type relationOrphanDoc struct {
			Coll  string
			DocID string
		}

		// Relation scope and settings docs have "r#" prefixed local ids.
		// Index them by their relation id so docs whose parent relation
		// still exists can be told apart from orphaned ones.
		orphans := make(map[int][]relationOrphanDoc)
		sel := bson.M{"_id": bson.M{"$regex": "^" + st.docID("r#")}}
		for _, collName := range []string{relationScopesC, settingsC} {
			coll, closer, err := st.db().GetCollection(collName)
			if err != nil {
				return errors.Trace(err)
			}
			var ids []struct {
				DocID string `bson:"_id"`
			}
			if err := coll.Find(sel).Select(bson.M{"_id": 1}).All(&ids); err != nil {
				closer()
				return errors.Trace(err)
			}
			closer()
			for _, id := range ids {
				local := st.localID(id.DocID)
				rest := local[2:]
				idx := strings.Index(rest, "#")
				if idx <= 0 {
					continue
				}
				relID, err := strconv.Atoi(rest[:idx])
				if err != nil {
					continue
				}
				orphans[relID] = append(orphans[relID], relationOrphanDoc{Coll: collName, DocID: id.DocID})
			}
		}

		// Collect the endpoint applications of every relation. Relation
		// ids absent from the map no longer exist, and the applications
		// are needed both to decide whether orphaned docs may be
		// repaired and to recreate missing application settings docs
		// below.
		relApps := make(map[int][]string)
		relations, closer, err := st.db().GetCollection(relationsC)
		if err != nil {
			return errors.Trace(err)
		}
		var relDoc struct {
			Id        int `bson:"id"`
			Endpoints []struct {
				ApplicationName string `bson:"applicationname"`
			} `bson:"endpoints"`
		}
		iter := relations.Find(nil).
			Select(bson.M{"id": 1, "endpoints.applicationname": 1}).
			Iter()
		for iter.Next(&relDoc) {
			var apps []string
			for _, ep := range relDoc.Endpoints {
				apps = append(apps, ep.ApplicationName)
			}
			relApps[relDoc.Id] = apps
		}
		if err := iter.Close(); err != nil {
			closer()
			return errors.Trace(err)
		}
		closer()

		// appExists records which endpoint applications still have a
		// document, in either the remote or the local applications
		// collection. Duplicate names in the $in query are harmless.
		appExists := make(map[string]bool)
		var appNames []string
		for _, apps := range relApps {
			appNames = append(appNames, apps...)
		}
		if len(appNames) > 0 {
			for _, collName := range []string{remoteApplicationsC, applicationsC} {
				coll, closer, err := st.db().GetCollection(collName)
				if err != nil {
					return errors.Trace(err)
				}
				var docs []struct {
					Name string `bson:"name"`
				}
				err = coll.Find(bson.M{"name": bson.M{"$in": appNames}}).
					Select(bson.M{"name": 1}).All(&docs)
				closer()
				if err != nil {
					return errors.Trace(err)
				}
				for _, doc := range docs {
					appExists[doc.Name] = true
				}
			}
		}
		appsExist := func(apps []string) bool {
			for _, appName := range apps {
				if !appExists[appName] {
					return false
				}
			}
			return true
		}

		// Removal and repair are best effort: each relation's ops run
		// in their own transaction, logging a warning on failure, so a
		// bad relation cannot abort the repair of the others.
		newSettingsOp := func(docID string) txn.Op {
			return txn.Op{
				C:      settingsC,
				Id:     docID,
				Assert: txn.DocMissing,
				Insert: &settingsDoc{
					DocID:     docID,
					ModelUUID: st.ModelUUID(),
					Settings:  make(settingsMap),
				},
			}
		}
		// A relation can be repaired both by the loop below and by the
		// application settings work, so the ids are recorded in maps:
		// each relation is counted once per list. A relation left in
		// both lists has some documents repaired and some left for
		// manual repair.
		repaired := make(map[int]bool)
		failed := make(map[int]bool)
		sortedIDs := func(ids map[int]bool) []int {
			sorted := make([]int, 0, len(ids))
			for id := range ids {
				sorted = append(sorted, id)
			}
			slices.Sort(sorted)
			return sorted
		}
		for id := range orphans {
			var ops []txn.Op
			if _, exists := relApps[id]; !exists {
				// The relation is gone; remove all its docs.
				logger.Debugf("removing orphaned relation docs for relation %d", id)
				for _, doc := range orphans[id] {
					ops = append(ops, txn.Op{
						C:      doc.Coll,
						Id:     doc.DocID,
						Remove: true,
					})
				}
			} else {
				// The relation still exists; its docs must form
				// complete pairs. But if an endpoint application no
				// longer has a document, the relation should have been
				// destroyed by RemoveOrphanedApplicationRelations and
				// its child records must not be recreated.
				if !appsExist(relApps[id]) {
					logger.Warningf("relation %d has dangling endpoint applications, leaving its docs for manual repair", id)
					failed[id] = true
					continue
				}
				settingsAt := make(map[string]bool)
				for _, doc := range orphans[id] {
					if doc.Coll == settingsC {
						settingsAt[doc.DocID] = true
					}
				}
				for _, doc := range orphans[id] {
					if doc.Coll != relationScopesC {
						continue
					}
					if !settingsAt[doc.DocID] {
						// The scope documents a unit in the relation, so
						// the settings doc it guarantees must exist;
						// recreate it empty to keep the watcher alive.
						logger.Debugf("recreating missing settings doc for relation %d scope %v", id, doc.DocID)
						ops = append(ops, newSettingsOp(doc.DocID))
					}
				}
			}
			if len(ops) == 0 {
				continue
			}
			if err := st.runRawTransaction(ops); err != nil {
				logger.Warningf("cannot repair relation %d docs: %v (manual repair required)", id, err)
				failed[id] = true
			} else {
				repaired[id] = true
			}
		}

		// Every relation whose endpoint applications still exist must
		// have an application settings doc per endpoint: the relation
		// units watcher watches these docs too and fails when one is
		// missing. A relation whose application settings docs are all
		// missing has no r#-prefixed documents at all, so it is not
		// visited by the loop above; scan all of them. One query
		// collects the docs that already exist.
		type appSettingsRef struct {
			relID   int
			appName string
		}
		missing := make(map[string]appSettingsRef)
		for id, apps := range relApps {
			if !appsExist(apps) {
				continue
			}
			for _, appName := range apps {
				settingsID := st.docID(relationApplicationSettingsKey(id, appName))
				missing[settingsID] = appSettingsRef{relID: id, appName: appName}
			}
		}
		if len(missing) > 0 {
			settingsIDs := make([]string, 0, len(missing))
			for settingsID := range missing {
				settingsIDs = append(settingsIDs, settingsID)
			}
			settings, closer, err := st.db().GetCollection(settingsC)
			if err != nil {
				return errors.Trace(err)
			}
			var docs []struct {
				DocID string `bson:"_id"`
			}
			err = settings.Find(bson.M{"_id": bson.M{"$in": settingsIDs}}).
				Select(bson.M{"_id": 1}).All(&docs)
			closer()
			if err != nil {
				return errors.Trace(err)
			}
			for _, doc := range docs {
				delete(missing, doc.DocID)
			}
		}
		perRelation := make(map[int][]appSettingsRef)
		for _, ref := range missing {
			perRelation[ref.relID] = append(perRelation[ref.relID], ref)
		}
		for id, absent := range perRelation {
			var ops []txn.Op
			for _, ref := range absent {
				logger.Debugf("recreating missing application settings doc for relation %d application %q", id, ref.appName)
				ops = append(ops, newSettingsOp(st.docID(relationApplicationSettingsKey(id, ref.appName))))
			}
			if err := st.runRawTransaction(ops); err != nil {
				logger.Warningf("cannot repair relation %d docs: %v (manual repair required)", id, err)
				failed[id] = true
			} else {
				repaired[id] = true
			}
		}
		if len(repaired) > 0 {
			logger.Infof("repaired relation documents for %d relation(s) in model %q: %v",
				len(repaired), st.ModelUUID(), sortedIDs(repaired))
		}
		if len(failed) > 0 {
			logger.Warningf("%d relation(s) left for manual repair in model %q: %v",
				len(failed), st.ModelUUID(), sortedIDs(failed))
		}
		return nil
	})
}

// RemoveOrphanedUnitStateRelations clears relation ids that have been
// deleted from the relation-state maps persisted in unitstates documents.
func RemoveOrphanedUnitStateRelations(pool *StatePool) error {
	return runForAllModelStates(pool, func(st *State) error {
		states, closer, err := st.db().GetCollection(unitStatesC)
		if err != nil {
			return errors.Trace(err)
		}
		type unitStateDoc struct {
			DocID         string            `bson:"_id"`
			RelationState map[string]string `bson:"relation-state"`
		}
		var docs []unitStateDoc
		var doc unitStateDoc
		iter := states.Find(nil).Select(bson.M{"relation-state": 1}).Iter()
		for iter.Next(&doc) {
			docs = append(docs, doc)
		}
		if err := iter.Close(); err != nil {
			closer()
			return errors.Trace(err)
		}
		closer()

		// Collect the distinct relation ids referenced by any unit's
		// relation state, and drop the ones that no longer exist.
		relIDs := make(map[int]bool)
		for _, doc := range docs {
			for k := range doc.RelationState {
				if id, err := strconv.Atoi(k); err == nil {
					relIDs[id] = true
				}
			}
		}
		if len(relIDs) == 0 {
			return nil
		}

		var referenced []int
		for id := range relIDs {
			referenced = append(referenced, id)
		}
		relations, closer, err := st.db().GetCollection(relationsC)
		if err != nil {
			return errors.Trace(err)
		}
		var relDocs []struct {
			Id int `bson:"id"`
		}
		if err := relations.Find(bson.D{{"id", bson.D{{"$in", referenced}}}}).Select(bson.M{"id": 1}).All(&relDocs); err != nil {
			closer()
			return errors.Trace(err)
		}
		closer()
		existing := make(map[int]bool)
		for _, rel := range relDocs {
			existing[rel.Id] = true
		}

		// The cleanup is best effort: each unit state document is
		// updated in its own transaction, logging a warning on
		// failure, so a stale document cannot abort the whole upgrade
		// step.
		var repaired, failed []string
		for _, unitDoc := range docs {
			var unset bson.D
			for k := range unitDoc.RelationState {
				id, err := strconv.Atoi(k)
				if err != nil || existing[id] {
					continue
				}
				unset = append(unset, bson.DocElem{Name: "relation-state." + k, Value: ""})
			}
			if len(unset) == 0 {
				continue
			}
			if err := st.runRawTransaction([]txn.Op{{
				C:      unitStatesC,
				Id:     unitDoc.DocID,
				Update: bson.D{{"$unset", unset}},
			}}); err != nil {
				logger.Warningf("cannot clear orphaned relation states for %v: %v (manual repair required)", unitDoc.DocID, err)
				failed = append(failed, unitDoc.DocID)
			} else {
				repaired = append(repaired, unitDoc.DocID)
			}
		}
		if len(repaired) > 0 {
			logger.Infof("cleared orphaned relation states for %d unit state document(s) in model %q: %v",
				len(repaired), st.ModelUUID(), repaired)
		}
		if len(failed) > 0 {
			logger.Warningf("%d unit state document(s) left for manual repair in model %q: %v",
				len(failed), st.ModelUUID(), failed)
		}
		return nil
	})
}

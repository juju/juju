// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/replicaset"
	"gopkg.in/juju/charm.v6"
	"gopkg.in/juju/names.v2"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/caas"
	k8s "github.com/juju/juju/caas/kubernetes/provider"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/controller"
	corelease "github.com/juju/juju/core/lease"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/mongo/utils"
	"github.com/juju/juju/permission"
	"github.com/juju/juju/state/globalclock"
	"github.com/juju/juju/state/lease"
	"github.com/juju/juju/storage/provider"
)

var upgradesLogger = loggo.GetLogger("juju.state.upgrade")

// runForAllModelStates will run runner function for every model passing a state
// for that model.
func runForAllModelStates(pool *StatePool, runner func(st *State) error) error {
	st := pool.SystemState()
	models, closer := st.db().GetCollection(modelsC)
	defer closer()

	var modelDocs []bson.M
	err := models.Find(nil).Select(bson.M{"_id": 1}).All(&modelDocs)
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

// readBsonDField returns the value of a given field in a bson.D.
func readBsonDField(d bson.D, name string) (interface{}, bool) {
	for i := range d {
		field := &d[i]
		if field.Name == name {
			return field.Value, true
		}
	}
	return nil, false
}

// replaceBsonDField replaces a field in bson.D.
func replaceBsonDField(d bson.D, name string, value interface{}) error {
	for i, field := range d {
		if field.Name == name {
			newField := field
			newField.Value = value
			d[i] = newField
			return nil
		}
	}
	return errors.NotFoundf("field %q", name)
}

// RenameAddModelPermission renames any permissions called addmodel to add-model.
func RenameAddModelPermission(pool *StatePool) error {
	st := pool.SystemState()
	coll, closer := st.db().GetRawCollection(permissionsC)
	defer closer()
	upgradesLogger.Infof("migrating addmodel permission")

	iter := coll.Find(bson.M{"access": "addmodel"}).Iter()
	defer iter.Close()
	var ops []txn.Op
	var doc bson.M
	for iter.Next(&doc) {
		id, ok := doc["_id"]
		if !ok {
			return errors.New("no id found in permission doc")
		}

		ops = append(ops, txn.Op{
			C:      permissionsC,
			Id:     id,
			Assert: txn.DocExists,
			Update: bson.D{{"$set", bson.D{{"access", "add-model"}}}},
		})
	}
	if err := iter.Close(); err != nil {
		return errors.Trace(err)
	}
	return st.runRawTransaction(ops)
}

// StripLocalUserDomain removes any @local suffix from any relevant document field values.
func StripLocalUserDomain(pool *StatePool) error {
	st := pool.SystemState()
	var ops []txn.Op
	more, err := stripLocalFromFields(st, cloudCredentialsC, "_id", "owner")
	if err != nil {
		return err
	}
	ops = append(ops, more...)

	more, err = stripLocalFromFields(st, modelsC, "owner", "cloud-credential")
	if err != nil {
		return err
	}
	ops = append(ops, more...)

	more, err = stripLocalFromFields(st, usermodelnameC, "_id")
	if err != nil {
		return err
	}
	ops = append(ops, more...)

	more, err = stripLocalFromFields(st, controllerUsersC, "_id", "user", "createdby")
	if err != nil {
		return err
	}
	ops = append(ops, more...)

	more, err = stripLocalFromFields(st, modelUsersC, "_id", "user", "createdby")
	if err != nil {
		return err
	}
	ops = append(ops, more...)

	more, err = stripLocalFromFields(st, permissionsC, "_id", "subject-global-key")
	if err != nil {
		return err
	}
	ops = append(ops, more...)

	more, err = stripLocalFromFields(st, modelUserLastConnectionC, "_id", "user")
	if err != nil {
		return err
	}
	ops = append(ops, more...)
	return st.runRawTransaction(ops)
}

func stripLocalFromFields(st *State, collName string, fields ...string) ([]txn.Op, error) {
	coll, closer := st.db().GetRawCollection(collName)
	defer closer()
	upgradesLogger.Infof("migrating document fields of the %s collection", collName)

	iter := coll.Find(nil).Iter()
	defer iter.Close()
	var ops []txn.Op
	var doc bson.D
	for iter.Next(&doc) {
		// Get a copy of the current doc id so we can see if it has changed.
		var newId interface{}
		id, ok := readBsonDField(doc, "_id")
		if ok {
			newId = id
		}

		// Take a copy of the current doc fields.
		newDoc := make(bson.D, len(doc))
		for i, f := range doc {
			newDoc[i] = f
		}

		// Iterate over the fields that need to be updated and
		// record any updates to be made.
		var update bson.D
		for _, field := range fields {
			isId := field == "_id"
			fieldVal, ok := readBsonDField(doc, field)
			if !ok {
				continue
			}
			updatedVal := strings.Replace(fieldVal.(string), "@local", "", -1)
			if err := replaceBsonDField(newDoc, field, updatedVal); err != nil {
				return nil, err
			}
			if isId {
				newId = updatedVal
			} else {
				if fieldVal != updatedVal {
					update = append(update, bson.DocElem{
						"$set", bson.D{{field, updatedVal}},
					})
				}
			}
		}

		// For documents where the id has not changed, we can
		// use an update operation.
		if newId == id {
			if len(update) > 0 {
				ops = append(ops, txn.Op{
					C:      collName,
					Id:     id,
					Assert: txn.DocExists,
					Update: update,
				})
			}
		} else {
			// Where the id has changed, we need to remove the old and
			// insert the new document.
			ops = append(ops, []txn.Op{{
				C:      collName,
				Id:     id,
				Assert: txn.DocExists,
				Remove: true,
			}, {
				C:      collName,
				Id:     newId,
				Assert: txn.DocMissing,
				Insert: newDoc,
			}}...)
		}
	}
	if err := iter.Close(); err != nil {
		return nil, errors.Trace(err)
	}
	return ops, nil
}

// AddMigrationAttempt adds an "attempt" field to migration documents
// which are missing one.
func AddMigrationAttempt(pool *StatePool) error {
	st := pool.SystemState()
	coll, closer := st.db().GetRawCollection(migrationsC)
	defer closer()

	query := coll.Find(bson.M{"attempt": bson.M{"$exists": false}})
	query = query.Select(bson.M{"_id": 1})
	iter := query.Iter()
	defer iter.Close()
	var ops []txn.Op
	var doc bson.M
	for iter.Next(&doc) {
		id := doc["_id"]
		attempt, err := extractMigrationAttempt(id)
		if err != nil {
			upgradesLogger.Warningf("%s (skipping)", err)
			continue
		}

		ops = append(ops, txn.Op{
			C:      migrationsC,
			Id:     id,
			Assert: txn.DocExists,
			Update: bson.D{{"$set", bson.D{{"attempt", attempt}}}},
		})
	}
	if err := iter.Close(); err != nil {
		return errors.Annotate(err, "iterating migrations")
	}

	return errors.Trace(st.runRawTransaction(ops))
}

func extractMigrationAttempt(id interface{}) (int, error) {
	idStr, ok := id.(string)
	if !ok {
		return 0, errors.Errorf("invalid migration doc id type: %v", id)
	}

	_, attemptStr, ok := splitDocID(idStr)
	if !ok {
		return 0, errors.Errorf("invalid migration doc id: %v", id)
	}

	attempt, err := strconv.Atoi(attemptStr)
	if err != nil {
		return 0, errors.Errorf("invalid migration attempt number: %v", id)
	}

	return attempt, nil
}

// AddLocalCharmSequences creates any missing sequences in the
// database for tracking already used local charm revisions.
func AddLocalCharmSequences(pool *StatePool) error {
	st := pool.SystemState()
	charmsColl, closer := st.db().GetRawCollection(charmsC)
	defer closer()

	query := bson.M{
		"url": bson.M{"$regex": "^local:"},
	}
	var docs []bson.M
	err := charmsColl.Find(query).Select(bson.M{
		"_id":  1,
		"life": 1,
	}).All(&docs)
	if err != nil {
		return errors.Trace(err)
	}

	// model UUID -> charm URL base -> max revision
	maxRevs := make(map[string]map[string]int)
	var deadIds []string
	for _, doc := range docs {
		id, ok := doc["_id"].(string)
		if !ok {
			upgradesLogger.Errorf("invalid charm id: %v", doc["_id"])
			continue
		}
		modelUUID, urlStr, ok := splitDocID(id)
		if !ok {
			upgradesLogger.Errorf("unable to split charm _id: %v", id)
			continue
		}
		url, err := charm.ParseURL(urlStr)
		if err != nil {
			upgradesLogger.Errorf("unable to parse charm URL: %v", err)
			continue
		}

		if _, exists := maxRevs[modelUUID]; !exists {
			maxRevs[modelUUID] = make(map[string]int)
		}

		baseURL := url.WithRevision(-1).String()
		curRev := maxRevs[modelUUID][baseURL]
		if url.Revision > curRev {
			maxRevs[modelUUID][baseURL] = url.Revision
		}

		if life, ok := doc["life"].(int); !ok {
			upgradesLogger.Errorf("invalid life for charm: %s", id)
			continue
		} else if life == int(Dead) {
			deadIds = append(deadIds, id)
		}

	}

	sequences, closer := st.db().GetRawCollection(sequenceC)
	defer closer()
	for modelUUID, modelRevs := range maxRevs {
		for baseURL, maxRevision := range modelRevs {
			name := charmRevSeqName(baseURL)
			updater := newDbSeqUpdater(sequences, modelUUID, name)
			err := updater.ensure(maxRevision + 1)
			if err != nil {
				return errors.Annotatef(err, "setting sequence %s", name)
			}
		}

	}

	// Remove dead charm documents
	var ops []txn.Op
	for _, id := range deadIds {
		ops = append(ops, txn.Op{
			C:      charmsC,
			Id:     id,
			Remove: true,
		})
	}
	err = st.runRawTransaction(ops)
	return errors.Annotate(err, "removing dead charms")
}

// UpdateLegacyLXDCloudCredentials updates the cloud credentials for the
// LXD-based controller, and updates the cloud endpoint with the given
// value.
func UpdateLegacyLXDCloudCredentials(
	st *State,
	endpoint string,
	credential cloud.Credential,
) error {
	cloudOps, err := updateLegacyLXDCloudsOps(st, endpoint)
	if err != nil {
		return errors.Trace(err)
	}
	credOps, err := updateLegacyLXDCredentialsOps(st, credential)
	if err != nil {
		return errors.Trace(err)
	}
	return st.db().RunTransaction(append(cloudOps, credOps...))
}

func updateLegacyLXDCloudsOps(st *State, endpoint string) ([]txn.Op, error) {
	clouds, err := st.Clouds()
	if err != nil {
		return nil, errors.Trace(err)
	}
	var ops []txn.Op
	for _, c := range clouds {
		if c.Type != "lxd" {
			continue
		}
		authTypes := []string{string(cloud.CertificateAuthType)}
		set := bson.D{{"auth-types", authTypes}}
		if c.Endpoint == "" {
			set = append(set, bson.DocElem{"endpoint", endpoint})
		}
		for _, region := range c.Regions {
			if region.Endpoint == "" {
				set = append(set, bson.DocElem{
					"regions." + utils.EscapeKey(region.Name) + ".endpoint",
					endpoint,
				})
			}
		}
		upgradesLogger.Infof("updating cloud %q: %v", c.Name, set)
		ops = append(ops, txn.Op{
			C:      cloudsC,
			Id:     c.Name,
			Assert: txn.DocExists,
			Update: bson.D{{"$set", set}},
		})
	}
	return ops, nil
}

func updateLegacyLXDCredentialsOps(st *State, cred cloud.Credential) ([]txn.Op, error) {
	var ops []txn.Op
	coll, closer := st.db().GetRawCollection(cloudCredentialsC)
	defer closer()
	iter := coll.Find(bson.M{"auth-type": "empty"}).Iter()
	defer iter.Close()
	var doc cloudCredentialDoc
	for iter.Next(&doc) {
		cloudCredentialTag, err := doc.cloudCredentialTag()
		if err != nil {
			upgradesLogger.Debugf("%v", err)
			continue
		}
		c, err := st.Cloud(doc.Cloud)
		if err != nil {
			return nil, errors.Trace(err)
		}
		if c.Type != "lxd" {
			continue
		}
		op := updateCloudCredentialOp(cloudCredentialTag, cred)
		upgradesLogger.Infof("updating credential %q: %v", cloudCredentialTag, op)
		ops = append(ops, op)
	}
	if err := iter.Close(); err != nil {
		return nil, errors.Trace(err)
	}
	return ops, nil
}

func upgradeNoProxy(np string) string {
	if np == "" {
		return "127.0.0.1,localhost,::1"
	}
	nps := set.NewStrings("127.0.0.1", "localhost", "::1")
	for _, i := range strings.Split(np, ",") {
		nps.Add(i)
	}
	// sorting is not a big overhead in this case and eases testing.
	return strings.Join(nps.SortedValues(), ",")
}

// UpgradeNoProxyDefaults changes the default values of no_proxy
// to hold localhost values as defaults.
func UpgradeNoProxyDefaults(pool *StatePool) error {
	st := pool.SystemState()
	var ops []txn.Op
	coll, closer := st.db().GetRawCollection(settingsC)
	defer closer()
	iter := coll.Find(bson.D{}).Iter()
	defer iter.Close()
	var doc settingsDoc
	for iter.Next(&doc) {
		noProxyVal := doc.Settings[config.NoProxyKey]
		noProxy, ok := noProxyVal.(string)
		if !ok {
			continue
		}
		noProxy = upgradeNoProxy(noProxy)
		doc.Settings[config.NoProxyKey] = noProxy
		ops = append(ops, txn.Op{
			C:      settingsC,
			Id:     doc.DocID,
			Assert: txn.DocExists,
			Update: bson.M{"$set": bson.M{"settings": doc.Settings}},
		})
	}
	if err := iter.Close(); err != nil {
		return errors.Trace(err)
	}
	if len(ops) > 0 {
		return errors.Trace(st.runRawTransaction(ops))
	}
	return nil
}

// AddNonDetachableStorageMachineId sets the "machineid" field on
// volume and filesystem docs that are inherently bound to that
// machine.
func AddNonDetachableStorageMachineId(pool *StatePool) error {
	return runForAllModelStates(pool, addNonDetachableStorageMachineId)
}

func addNonDetachableStorageMachineId(st *State) error {
	sb, err := NewStorageBackend(st)
	if err != nil {
		return errors.Trace(err)
	}

	getPool := func(d bson.M) string {
		var pool string
		info, _ := d["info"].(bson.M)
		params, _ := d["params"].(bson.M)
		if info != nil {
			pool = info["pool"].(string)
		} else if params != nil {
			pool = params["pool"].(string)
		}
		return pool
	}

	var needsUpgradeTerm = bson.D{
		{"machineid", bson.D{{"$exists", false}}},
		{"hostid", bson.D{{"$exists", false}}},
	}
	var ops []txn.Op

	volumeColl, cleanup := st.db().GetCollection(volumesC)
	defer cleanup()

	var volData []bson.M
	err = volumeColl.Find(needsUpgradeTerm).All(&volData)
	if err != nil && err != mgo.ErrNotFound {
		return errors.Trace(err)
	}

	volumeAttachColl, cleanup := st.db().GetCollection(volumeAttachmentsC)
	defer cleanup()

	var volAttachData []bson.M
	err = volumeAttachColl.Find(nil).All(&volAttachData)
	if err != nil && err != mgo.ErrNotFound {
		return errors.Trace(err)
	}
	attachDataForVolumes := make(map[string][]bson.M)
	for _, vad := range volAttachData {
		volId := vad["volumeid"].(string)
		data := attachDataForVolumes[volId]
		data = append(data, vad)
		attachDataForVolumes[volId] = data
	}

	for _, v := range volData {
		detachable, err := isDetachableVolumePool(sb, getPool(v))
		if err != nil {
			return errors.Trace(err)
		}
		if detachable {
			continue
		}

		attachInfo := attachDataForVolumes[v["name"].(string)]
		if len(attachInfo) != 1 {
			// There should be exactly one attachment since the
			// filesystem is non-detachable, but be defensive
			// and leave the document alone if our expectations
			// are not met.
			continue
		}
		machineId := attachInfo[0]["machineid"]
		if machineId == "" {
			machineId = attachInfo[0]["hostid"]
		}
		ops = append(ops, txn.Op{
			C:      volumesC,
			Id:     v["name"],
			Assert: txn.DocExists,
			Update: bson.D{{"$set", bson.D{
				{"machineid", machineId},
			}}},
		})
	}

	filesystemColl, cleanup := st.db().GetCollection(filesystemsC)
	defer cleanup()

	var fsData []bson.M
	err = filesystemColl.Find(needsUpgradeTerm).All(&fsData)
	if err != nil && err != mgo.ErrNotFound {
		return errors.Trace(err)
	}
	filesystemAttachColl, cleanup := st.db().GetCollection(filesystemAttachmentsC)
	defer cleanup()

	var filesystemAttachData []bson.M
	err = filesystemAttachColl.Find(nil).All(&filesystemAttachData)
	if err != nil && err != mgo.ErrNotFound {
		return errors.Trace(err)
	}
	attachDataForFilesystems := make(map[string][]bson.M)
	for _, fad := range filesystemAttachData {
		filesystemId := fad["filesystemid"].(string)
		data := attachDataForFilesystems[filesystemId]
		data = append(data, fad)
		attachDataForFilesystems[filesystemId] = data
	}

	for _, f := range fsData {
		if detachable, err := isDetachableFilesystemPool(sb, getPool(f)); err != nil {
			return errors.Trace(err)
		} else if detachable {
			continue
		}

		attachInfo := attachDataForFilesystems[f["filesystemid"].(string)]
		if len(attachInfo) != 1 {
			// There should be exactly one attachment since the
			// filesystem is non-detachable, but be defensive
			// and leave the document alone if our expectations
			// are not met.
			continue
		}
		machineId := attachInfo[0]["machineid"]
		if machineId == "" {
			machineId = attachInfo[0]["hostid"]
		}
		ops = append(ops, txn.Op{
			C:      filesystemsC,
			Id:     f["filesystemid"],
			Assert: txn.DocExists,
			Update: bson.D{{"$set", bson.D{
				{"machineid", machineId},
			}}},
		})
	}
	if len(ops) > 0 {
		return errors.Trace(st.db().RunTransaction(ops))
	}
	return nil
}

// RemoveNilValueApplicationSettings removes any application setting
// key-value pairs from "settings" where value is nil.
func RemoveNilValueApplicationSettings(pool *StatePool) error {
	st := pool.SystemState()
	coll, closer := st.db().GetRawCollection(settingsC)
	defer closer()
	iter := coll.Find(bson.M{"_id": bson.M{"$regex": "^.*:a#.*"}}).Iter()
	defer iter.Close()
	var ops []txn.Op
	var doc settingsDoc
	for iter.Next(&doc) {
		settingsChanged := false
		for key, value := range doc.Settings {
			if value != nil {
				continue
			}
			settingsChanged = true
			delete(doc.Settings, key)
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

// AddControllerLogCollectionsSizeSettings adds the controller
// settings to control log pruning and txn log size if they are missing.
func AddControllerLogCollectionsSizeSettings(pool *StatePool) error {
	st := pool.SystemState()
	coll, closer := st.db().GetRawCollection(controllersC)
	defer closer()
	var doc settingsDoc
	if err := coll.FindId(controllerSettingsGlobalKey).One(&doc); err != nil {
		if err == mgo.ErrNotFound {
			return nil
		}
		return errors.Trace(err)
	}

	var ops []txn.Op
	// Logs settings removed here because they are now no longer necessary.
	settingsChanged :=
		maybeUpdateSettings(doc.Settings, controller.MaxTxnLogSize, fmt.Sprintf("%vM", controller.DefaultMaxTxnLogCollectionMB))
	if settingsChanged {
		ops = append(ops, txn.Op{
			C:      controllersC,
			Id:     doc.DocID,
			Assert: txn.DocExists,
			Update: bson.M{"$set": bson.M{"settings": doc.Settings}},
		})
	}
	if len(ops) > 0 {
		return errors.Trace(st.runRawTransaction(ops))
	}
	return nil
}

func maybeUpdateSettings(settings map[string]interface{}, key string, value interface{}) bool {
	if _, ok := settings[key]; !ok {
		settings[key] = value
		return true
	}
	return false
}

// applyToAllModelSettings iterates the model settings documents and applys the
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

// AddStatusHistoryPruneSettings adds the model settings
// to control log pruning if they are missing.
func AddStatusHistoryPruneSettings(pool *StatePool) error {
	st := pool.SystemState()
	err := applyToAllModelSettings(st, func(doc *settingsDoc) (bool, error) {
		settingsChanged :=
			maybeUpdateSettings(doc.Settings, config.MaxStatusHistoryAge, config.DefaultStatusHistoryAge)
		settingsChanged =
			maybeUpdateSettings(doc.Settings, config.MaxStatusHistorySize, config.DefaultStatusHistorySize) || settingsChanged
		return settingsChanged, nil
	})
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}

// AddActionPruneSettings adds the model settings
// to control log pruning if they are missing.
func AddActionPruneSettings(pool *StatePool) error {
	st := pool.SystemState()
	err := applyToAllModelSettings(st, func(doc *settingsDoc) (bool, error) {
		settingsChanged :=
			maybeUpdateSettings(doc.Settings, config.MaxActionResultsAge, config.DefaultActionResultsAge)
		settingsChanged =
			maybeUpdateSettings(doc.Settings, config.MaxActionResultsSize, config.DefaultActionResultsSize) || settingsChanged
		return settingsChanged, nil
	})
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}

// AddUpdateStatusHookSettings adds the model settings
// to control how often to run the update-status hook
// if they are missing.
func AddUpdateStatusHookSettings(pool *StatePool) error {
	st := pool.SystemState()
	err := applyToAllModelSettings(st, func(doc *settingsDoc) (bool, error) {
		settingsChanged :=
			maybeUpdateSettings(doc.Settings, config.UpdateStatusHookInterval, config.DefaultUpdateStatusHookInterval)
		return settingsChanged, nil
	})
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}

// AddStorageInstanceConstraints sets the "constraints" field on
// storage instance docs.
func AddStorageInstanceConstraints(pool *StatePool) error {
	return runForAllModelStates(pool, addStorageInstanceConstraints)
}

func addStorageInstanceConstraints(st *State) error {
	sb, err := NewStorageBackend(st)
	if err != nil {
		return errors.Trace(err)
	}
	storageInstances, err := sb.storageInstances(bson.D{
		{"constraints", bson.D{{"$exists", false}}},
	})
	if err != nil {
		return errors.Trace(err)
	}
	var ops []txn.Op
	for _, s := range storageInstances {
		var siCons storageInstanceConstraints
		var defaultPool string
		switch s.Kind() {
		case StorageKindBlock:
			v, err := sb.storageInstanceVolume(s.StorageTag())
			if err == nil {
				if v.doc.Info != nil {
					siCons.Pool = v.doc.Info.Pool
					siCons.Size = v.doc.Info.Size
				} else if v.doc.Params != nil {
					siCons.Pool = v.doc.Params.Pool
					siCons.Size = v.doc.Params.Size
				}
			} else if errors.IsNotFound(err) {
				defaultPool = string(provider.LoopProviderType)
			} else {
				return errors.Trace(err)
			}
		case StorageKindFilesystem:
			f, err := sb.storageInstanceFilesystem(s.StorageTag())
			if err == nil {
				if f.doc.Info != nil {
					siCons.Pool = f.doc.Info.Pool
					siCons.Size = f.doc.Info.Size
				} else if f.doc.Params != nil {
					siCons.Pool = f.doc.Params.Pool
					siCons.Size = f.doc.Params.Size
				}
			} else if errors.IsNotFound(err) {
				defaultPool = string(provider.RootfsProviderType)
			} else {
				return errors.Trace(err)
			}
		default:
			// Unknown storage kind, ignore.
			continue
		}
		if siCons.Pool == "" {
			// There's no associated volume or filesystem, so
			// take constraints from the application storage
			// constraints. This could be wrong, but we've got
			// nothing else to go on, and this will match the
			// old broken behaviour at least.
			//
			// If there's no owner, just use the defaults.
			siCons.Pool = defaultPool
			siCons.Size = 1024
			if ownerTag := s.maybeOwner(); ownerTag != nil {
				type withStorageConstraints interface {
					StorageConstraints() (map[string]StorageConstraints, error)
				}
				owner, err := st.FindEntity(ownerTag)
				if err != nil {
					return errors.Trace(err)
				}
				if owner, ok := owner.(withStorageConstraints); ok {
					allCons, err := owner.StorageConstraints()
					if err != nil {
						return errors.Trace(err)
					}
					if cons, ok := allCons[s.StorageName()]; ok {
						siCons.Pool = cons.Pool
						siCons.Size = cons.Size
					}
				}
			}
			logger.Warningf(
				"no volume or filesystem found, using application storage constraints for %s",
				names.ReadableString(s.Tag()),
			)
		}
		ops = append(ops, txn.Op{
			C:      storageInstancesC,
			Id:     s.doc.Id,
			Assert: txn.DocExists,
			Update: bson.D{{"$set", bson.D{
				{"constraints", siCons},
			}}},
		})
	}
	if len(ops) > 0 {
		return errors.Trace(st.db().RunTransaction(ops))
	}
	return nil
}

// SplitLogCollections moves log entries from the old single log collection
// to the log collection per model.
func SplitLogCollections(pool *StatePool) error {
	st := pool.SystemState()
	session := st.MongoSession()
	db := session.DB(logsDB)
	oldLogs := db.C("logs")

	// If we haven't seen any particular model, we need to initialise
	// the logs collection with the right indices.
	seen := set.NewStrings()

	iter := oldLogs.Find(nil).Iter()
	defer iter.Close()

	var doc bson.M
	for iter.Next(&doc) {
		modelUUID := doc["e"].(string)
		newCollName := logCollectionName(modelUUID)
		newLogs := db.C(newCollName)

		if !seen.Contains(newCollName) {
			// There is no setting for the size, so use the default.
			if err := InitDbLogsForModel(session, modelUUID, controller.DefaultModelLogsSizeMB); err != nil {
				return errors.Annotatef(err, "failed to init new logs collection %q", newCollName)
			}
			seen.Add(newCollName)
		}

		delete(doc, "e") // old model uuid

		if err := newLogs.Insert(doc); err != nil {
			// In the case of a restart, we may have already moved
			// some of these rows, in which case we'd get a duplicate
			// id error (this is OK).
			if !mgo.IsDup(err) {
				return errors.Annotate(err, "failed to insert log record")
			}
		}
		doc = nil
	}
	if err := iter.Close(); err != nil {
		return errors.Trace(err)
	}

	// drop the old collection
	if err := oldLogs.DropCollection(); err != nil {
		// If the namespace is already missing, that's fine.
		if isMgoNamespaceNotFound(err) {
			return nil
		}
		return errors.Annotate(err, "failed to drop old logs collection")
	}
	return nil
}

func isMgoNamespaceNotFound(err error) bool {
	// Check for &mgo.QueryError{Code:26, Message:"ns not found"}
	if qerr, ok := err.(*mgo.QueryError); ok {
		if qerr.Code == 26 {
			return true
		}
		// For older mongodb's Code isn't set. Use the message
		// instead.
		if qerr.Message == "ns not found" {
			return true
		}
	}
	return false
}

type relationUnitCountInfo struct {
	docId     string
	endpoints set.Strings
	unitCount int
}

func (i *relationUnitCountInfo) otherEnd(appName string) (string, error) {
	for _, name := range i.endpoints.Values() {
		// TODO(babbageclunk): can a non-peer relation have one app for both endpoints?
		if name != appName {
			return name, nil
		}
	}
	return "", errors.Errorf("couldn't find other end of %q for %q", i.docId, appName)
}

// CorrectRelationUnitCounts ensures that there aren't any rows in
// relationscopes for applications that shouldn't be there. Fix for
// https://bugs.launchpad.net/juju/+bug/1699050
func CorrectRelationUnitCounts(pool *StatePool) error {
	st := pool.SystemState()
	applicationsColl, aCloser := st.db().GetRawCollection(applicationsC)
	defer aCloser()

	relationsColl, rCloser := st.db().GetRawCollection(relationsC)
	defer rCloser()

	scopesColl, sCloser := st.db().GetRawCollection(relationScopesC)
	defer sCloser()

	applications, err := collectApplicationInfo(applicationsColl)
	if err != nil {
		return errors.Trace(err)
	}
	relations, err := collectRelationInfo(relationsColl)
	if err != nil {
		return errors.Trace(err)
	}

	var ops []txn.Op
	var scope struct {
		DocId     string `bson:"_id"`
		Key       string `bson:"key"`
		ModelUUID string `bson:"model-uuid"`
	}
	relationsToUpdate := set.NewStrings()
	iter := scopesColl.Find(nil).Iter()
	defer iter.Close()

	for iter.Next(&scope) {
		// Scope key looks like: r#<relation id>#[<principal unit for container scope>#]<role>#<unit>
		keyParts := strings.Split(scope.Key, "#")
		if len(keyParts) < 4 {
			upgradesLogger.Errorf("malformed scope key %q", scope.Key)
			continue
		}

		principalApp, found := extractPrincipalUnitApp(keyParts)
		if !found {
			// No change needed - this isn't a container scope.
			continue
		}
		relationKey := scope.ModelUUID + ":" + keyParts[1]
		relation, ok := relations[relationKey]
		if !ok {
			upgradesLogger.Errorf("orphaned relation scope %q", scope.DocId)
			continue
		}

		if relation.endpoints.Contains(principalApp) {
			// This scope record is fine - it's for an app that's in the relation.
			continue
		}

		unit := keyParts[len(keyParts)-1]
		subordinate, err := otherEndIsSubordinate(relation, unit, scope.ModelUUID, applications)
		if err != nil {
			return errors.Trace(err)
		}
		if subordinate {
			// The other end for this unit is for a subordinate
			// application, allow those.
			continue
		}

		// This scope record needs to be removed and the unit count updated.
		relation.unitCount--
		relationsToUpdate.Add(relationKey)
		ops = append(ops, txn.Op{
			C:      relationScopesC,
			Id:     scope.DocId,
			Assert: txn.DocExists,
			Remove: true,
		})
	}
	if err := iter.Close(); err != nil {
		return errors.Trace(err)
	}

	// Add in the updated unit counts.
	for _, key := range relationsToUpdate.Values() {
		relation := relations[key]
		ops = append(ops, txn.Op{
			C:      relationsC,
			Id:     relation.docId,
			Assert: txn.DocExists,
			Update: bson.M{"$set": bson.M{"unitcount": relation.unitCount}},
		})
	}
	if len(ops) > 0 {
		return errors.Trace(st.runRawTransaction(ops))
	}
	return nil
}

func collectApplicationInfo(coll *mgo.Collection) (map[string]bool, error) {
	results := make(map[string]bool)
	var doc struct {
		Id          string `bson:"_id"`
		Subordinate bool   `bson:"subordinate"`
	}
	iter := coll.Find(nil).Iter()
	for iter.Next(&doc) {
		results[doc.Id] = doc.Subordinate
	}
	if err := iter.Close(); err != nil {
		return nil, errors.Trace(err)
	}
	return results, nil
}

func collectRelationInfo(coll *mgo.Collection) (map[string]*relationUnitCountInfo, error) {
	relations := make(map[string]*relationUnitCountInfo)
	var doc struct {
		DocId     string   `bson:"_id"`
		ModelUUID string   `bson:"model-uuid"`
		Id        int      `bson:"id"`
		UnitCount int      `bson:"unitcount"`
		Endpoints []bson.M `bson:"endpoints"`
	}

	iter := coll.Find(nil).Iter()
	defer iter.Close()
	for iter.Next(&doc) {
		endpoints := set.NewStrings()
		for _, epDoc := range doc.Endpoints {
			appName, ok := epDoc["applicationname"].(string)
			if !ok {
				return nil, errors.Errorf("invalid application name: %v", epDoc["applicationname"])
			}
			endpoints.Add(appName)
		}
		key := fmt.Sprintf("%s:%d", doc.ModelUUID, doc.Id)
		relations[key] = &relationUnitCountInfo{
			docId:     doc.DocId,
			endpoints: endpoints,
			unitCount: doc.UnitCount,
		}
	}
	if err := iter.Close(); err != nil {
		return nil, errors.Trace(err)
	}
	return relations, nil
}

// unitAppName returns the name of the Application, given a Units name.
func unitAppName(unitName string) string {
	unitParts := strings.Split(unitName, "/")
	return unitParts[0]
}

func extractPrincipalUnitApp(scopeKeyParts []string) (string, bool) {
	if len(scopeKeyParts) < 5 {
		return "", false
	}
	return unitAppName(scopeKeyParts[2]), true
}

func otherEndIsSubordinate(relation *relationUnitCountInfo, unitName, modelUUID string, applications map[string]bool) (bool, error) {
	app, err := relation.otherEnd(unitAppName(unitName))
	if err != nil {
		return false, errors.Trace(err)
	}
	appKey := fmt.Sprintf("%s:%s", modelUUID, app)
	res, ok := applications[appKey]
	if !ok {
		return false, errors.Errorf("can't determine whether %q is subordinate", appKey)
	}
	return res, nil
}

// AddModelEnvironVersion ensures that all model docs have an environ-version
// field. For those that do not have one, they are seeded with version zero.
// This will force all environ upgrade steps to be run; there are only two
// providers (azure and vsphere) that had upgrade steps at the time, and the
// upgrade steps are required to be idempotent anyway.
func AddModelEnvironVersion(pool *StatePool) error {
	st := pool.SystemState()
	coll, closer := st.db().GetCollection(modelsC)
	defer closer()

	var doc struct {
		UUID           string `bson:"_id"`
		Cloud          string `bson:"cloud"`
		EnvironVersion *int   `bson:"environ-version,omitempty"`
	}

	var ops []txn.Op
	iter := coll.Find(nil).Iter()
	defer iter.Close()
	for iter.Next(&doc) {
		if doc.EnvironVersion != nil {
			continue
		}
		ops = append(ops, txn.Op{
			C:      modelsC,
			Id:     doc.UUID,
			Assert: txn.DocExists,
			Update: bson.D{{"$set", bson.D{{"environ-version", 0}}}},
		})
	}
	if err := iter.Close(); err != nil {
		return errors.Trace(err)
	}
	return st.db().RunTransaction(ops)
}

// AddModelType adds a "type" field to model documents which don't
// have one. The "iaas" type is used.
func AddModelType(pool *StatePool) error {
	st := pool.SystemState()
	coll, closer := st.db().GetCollection(modelsC)
	defer closer()

	var doc struct {
		UUID string `bson:"_id"`
		Type string `bson:"type"`
	}

	var ops []txn.Op
	iter := coll.Find(nil).Iter()
	defer iter.Close()
	for iter.Next(&doc) {
		if doc.Type != "" {
			continue
		}
		ops = append(ops, txn.Op{
			C:      modelsC,
			Id:     doc.UUID,
			Assert: txn.DocExists,
			Update: bson.D{{"$set", bson.D{{"type", "iaas"}}}},
		})
	}
	if err := iter.Close(); err != nil {
		return errors.Trace(err)
	}
	return st.db().RunTransaction(ops)
}

// MigrateLeasesToGlobalTime removes old (<2.3-beta2) lease/clock-skew
// documents, replacing the lease documents with new ones for the
// existing lease holders.
func MigrateLeasesToGlobalTime(pool *StatePool) error {
	return runForAllModelStates(pool, migrateModelLeasesToGlobalTime)
}

const InitialLeaderClaimTime = time.Minute

func migrateModelLeasesToGlobalTime(st *State) error {
	coll, closer := st.db().GetCollection(leasesC)
	defer closer()

	// Find all old lease/clock-skew documents, remove them
	// and create replacement lease docs in the new format.
	//
	// Replacement leases are created with a duration of a
	// minute, relative to the global time epoch.
	err := st.db().Run(func(int) ([]txn.Op, error) {
		var doc struct {
			DocID     string `bson:"_id"`
			Type      string `bson:"type"`
			Namespace string `bson:"namespace"`
			Name      string `bson:"name"`
			Holder    string `bson:"holder"`
			Expiry    int64  `bson:"expiry"`
			Writer    string `bson:"writer"`
		}

		var ops []txn.Op
		iter := coll.Find(bson.D{{"type", bson.D{{"$exists", true}}}}).Iter()
		defer iter.Close()
		for iter.Next(&doc) {
			ops = append(ops, txn.Op{
				C:      coll.Name(),
				Id:     st.localID(doc.DocID),
				Assert: txn.DocExists,
				Remove: true,
			})
			if doc.Type != "lease" {
				upgradesLogger.Tracef("deleting old lease doc %q", doc.DocID)
				continue
			}
			// Check if the target exists
			if _, err := lease.LookupLease(coll, doc.Namespace, doc.Name); err == nil {
				// target already exists, it takes precedence over an old doc, which we still want to delete
				upgradesLogger.Infof("new lease %q %q already exists, simply deleting old lease %q",
					doc.Namespace, doc.Name, doc.DocID)
				continue
			} else if err != mgo.ErrNotFound {
				// We got an unknown error looking up this doc, don't suppress it
				return nil, err
			}
			upgradesLogger.Tracef("migrating lease %q to new lease structure", doc.DocID)
			claimOps, err := lease.ClaimLeaseOps(
				doc.Namespace,
				doc.Name,
				doc.Holder,
				doc.Writer,
				coll.Name(),
				globalclock.GlobalEpoch(),
				InitialLeaderClaimTime,
			)
			if err != nil {
				return nil, errors.Trace(err)
			}
			ops = append(ops, claimOps...)
		}
		if err := iter.Close(); err != nil {
			return nil, errors.Trace(err)
		}
		return ops, nil
	})
	return errors.Annotate(err, "upgrading legacy lease documents")
}

// AddRelationStatus sets the initial status for existing relations
// without a status.
func AddRelationStatus(pool *StatePool) error {
	return runForAllModelStates(pool, addRelationStatus)
}

func addRelationStatus(st *State) error {
	// Newly created relations will have a status doc,
	// so it suffices to just get the collection once
	// up front.
	relations, err := st.AllRelations()
	if err != nil {
		return errors.Trace(err)
	}
	now := st.clock().Now()
	err = st.db().Run(func(int) ([]txn.Op, error) {
		var ops []txn.Op
		for _, rel := range relations {
			_, err := rel.Status()
			if err == nil {
				continue
			}
			if !errors.IsNotFound(err) {
				return nil, err
			}
			// Relations are marked as either
			// joining or joined, depending
			// on whether there are any units
			// in scope.
			relStatus := status.Joining
			if rel.doc.UnitCount > 0 {
				relStatus = status.Joined
			}
			relationStatusDoc := statusDoc{
				Status:    relStatus,
				ModelUUID: st.ModelUUID(),
				Updated:   now.UnixNano(),
			}
			ops = append(ops, createStatusOp(
				st, relationGlobalScope(rel.Id()),
				relationStatusDoc,
			))
		}
		return ops, nil
	})
	return errors.Annotate(err, "adding relation status")
}

// MoveOldAuditLog renames the no-longer-needed audit.log collection
// to old-audit.log if it has any rows - if it's empty it deletes it.
func MoveOldAuditLog(pool *StatePool) error {
	st := pool.SystemState()
	names, err := st.MongoSession().DB("juju").CollectionNames()
	if err != nil {
		return errors.Trace(err)
	}
	if !set.NewStrings(names...).Contains("audit.log") {
		// No audit log collection to move.
		return nil
	}

	coll, closer := st.db().GetRawCollection("audit.log")
	defer closer()

	rows, err := coll.Count()
	if err != nil {
		return errors.Trace(err)
	}
	if rows == 0 {
		return errors.Trace(coll.DropCollection())
	}
	session := st.MongoSession()
	renameCommand := bson.D{
		{"renameCollection", "juju.audit.log"},
		{"to", "juju.old-audit.log"},
	}
	return errors.Trace(session.Run(renameCommand, nil))
}

// DeleteCloudImageMetadata deletes any non-custom cloud
// image metadata records from the cloudimagemetadata collection.
func DeleteCloudImageMetadata(pool *StatePool) error {
	st := pool.SystemState()
	coll, closer := st.db().GetRawCollection(cloudimagemetadataC)
	defer closer()

	bulk := coll.Bulk()
	bulk.Unordered()
	bulk.RemoveAll(bson.D{{"source", bson.D{{"$ne", "custom"}}}})
	_, err := bulk.Run()
	return errors.Annotate(err, "deleting cloud image metadata records")
}

// CopyMongoSpaceToHASpaceConfig copies the Mongo space name from
// ControllerInfo to the HA space name in ControllerConfig.
// This only happens if the Mongo space state is valid, it is not empty,
// and if there is no value already set for the HA space name.
// The old keys are then deleted from ControllerInfo.
func MoveMongoSpaceToHASpaceConfig(pool *StatePool) error {
	st := pool.SystemState()
	// Holds Mongo space fields removed from controllersDoc.
	type controllersUpgradeDoc struct {
		MongoSpaceName  string `bson:"mongo-space-name"`
		MongoSpaceState string `bson:"mongo-space-state"`
	}
	var doc controllersUpgradeDoc

	controllerColl, controllerCloser := st.db().GetRawCollection(controllersC)
	defer controllerCloser()
	err := controllerColl.Find(bson.D{{"_id", modelGlobalKey}}).One(&doc)
	if err != nil {
		return errors.Annotate(err, "retrieving controller info doc")
	}

	mongoSpace := doc.MongoSpaceName
	if doc.MongoSpaceState == "valid" && mongoSpace != "" {
		settings, err := readSettings(st.db(), controllersC, controllerSettingsGlobalKey)
		if err != nil {
			return errors.Annotate(err, "cannot get controller config")
		}

		// In the unlikely event that there is already a juju-ha-space
		// configuration setting, we do not copy over it with the old Mongo
		// space name.
		if haSpace, ok := settings.Get(controller.JujuHASpace); ok {
			upgradesLogger.Debugf("not copying mongo-space-name %q to juju-ha-space - already set to %q",
				mongoSpace, haSpace)
		} else {
			settings.Set(controller.JujuHASpace, mongoSpace)
			if _, err = settings.Write(); err != nil {
				return errors.Annotate(err, "writing controller info")
			}
		}
	}

	err = controllerColl.UpdateId(modelGlobalKey, bson.M{"$unset": bson.M{
		"mongo-space-name":  1,
		"mongo-space-state": 1,
	}})
	return errors.Annotate(err, "removing mongo-space-state and mongo-space-name")
}

// CreateMissingApplicationConfig ensures that all models have an application config in the db.
func CreateMissingApplicationConfig(pool *StatePool) error {
	st := pool.SystemState()
	settingsColl, settingsCloser := st.db().GetRawCollection(settingsC)
	defer settingsCloser()

	var applicationConfigIDs []struct {
		ID string `bson:"_id"`
	}
	settingsColl.Find(bson.M{
		"_id": bson.M{"$regex": bson.RegEx{"#application$", ""}}}).All(&applicationConfigIDs)

	allIDs := set.NewStrings()
	for _, id := range applicationConfigIDs {
		allIDs.Add(id.ID)
	}

	appsColl, appsCloser := st.db().GetRawCollection(applicationsC)
	defer appsCloser()

	var applicationNames []struct {
		Name      string `bson:"name"`
		ModelUUID string `bson:"model-uuid"`
	}
	appsColl.Find(nil).All(&applicationNames)

	var newAppConfigOps []txn.Op
	emptySettings := make(map[string]interface{})
	for _, app := range applicationNames {
		appConfID := fmt.Sprintf("%s:%s", app.ModelUUID, applicationConfigKey(app.Name))
		if !allIDs.Contains(appConfID) {
			newOp := createSettingsOp(settingsC, appConfID, emptySettings)
			// createSettingsOp assumes you're using a model-specific state, which will auto-inject the ModelUUID
			// since we're doing this globally, cast it to the underlying type and add it.
			newOp.Insert.(*settingsDoc).ModelUUID = app.ModelUUID
			newAppConfigOps = append(newAppConfigOps, newOp)
		}
	}
	err := st.db().RunRawTransaction(newAppConfigOps)
	if err != nil {
		return errors.Annotate(err, "writing application configs")
	}
	return nil
}

// RemoveVotingMachineIds ensures that the 'votingmachineids' field on controller info has been removed
func RemoveVotingMachineIds(pool *StatePool) error {
	st := pool.SystemState()
	controllerColl, controllerCloser := st.db().GetRawCollection(controllersC)
	defer controllerCloser()
	// The votingmachineids field is just a denormalization of Machine.WantsVote() so we can just
	// remove it as being redundant
	err := controllerColl.UpdateId(modelGlobalKey, bson.M{"$unset": bson.M{"votingmachineids": 1}})
	if err != nil {
		return errors.Annotate(err, "removing votingmachineids")
	}
	return nil
}

// AddCloudModelCounts updates cloud docs to ensure the model count field is set.
func AddCloudModelCounts(pool *StatePool) error {
	st := pool.SystemState()
	cloudsColl, closer := st.db().GetCollection(cloudsC)
	defer closer()

	var clouds []cloudDoc
	err := cloudsColl.Find(nil).All(&clouds)
	if err != nil {
		return errors.Trace(err)
	}

	modelsColl, closer := st.db().GetCollection(modelsC)
	defer closer()
	refCountColl, closer := st.db().GetCollection(globalRefcountsC)
	defer closer()

	var updateOps []txn.Op
	for _, c := range clouds {
		n, err := modelsColl.Find(bson.D{{"cloud", c.Name}}).Count()
		if err != nil {
			return errors.Trace(err)
		}
		_, currentCount, err := countCloudModelRefOp(st, c.Name)
		if err != nil {
			return errors.Trace(err)
		}
		if n != currentCount {
			op, err := nsRefcounts.CreateOrIncRefOp(refCountColl, cloudModelRefCountKey(c.Name), n-currentCount)
			if err != nil {
				return errors.Trace(err)
			}
			updateOps = append(updateOps, op)
		}
	}
	return st.db().RunTransaction(updateOps)
}

// UpgradeDefaultContainerImageStreamConfig ensures that the config value for
// container-image-stream is set to its default value, "released".
func UpgradeContainerImageStreamDefault(pool *StatePool) error {
	st := pool.SystemState()
	err := applyToAllModelSettings(st, func(doc *settingsDoc) (bool, error) {
		ciStreamVal, keySet := doc.Settings[config.ContainerImageStreamKey]
		if keySet {
			if ciStream, _ := ciStreamVal.(string); ciStream != "" {
				return false, nil
			}
		}
		doc.Settings[config.ContainerImageStreamKey] = "released"
		return true, nil
	})
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}

// RemoveContainerImageStreamFromNonModelSettings
// In 2.3.6 we accidentally had an upgrade step that added
// "container-image-stream": "released" to all settings documents, not just the
// ones relating to Model data.
// This removes it from all the ones that aren't model docs if it is exactly
// what we would have added in 2.3.6
func RemoveContainerImageStreamFromNonModelSettings(pool *StatePool) error {
	st := pool.SystemState()
	uuids, err := st.AllModelUUIDs()
	if err != nil {
		return errors.Trace(err)
	}

	modelDocIDs := set.NewStrings()
	for _, uuid := range uuids {
		modelDocIDs.Add(uuid + ":e")
	}
	coll, closer := st.db().GetRawCollection(settingsC)
	defer closer()

	iter := coll.Find(nil).Iter()
	defer iter.Close()

	// This is the key for the field that was accidentally added in 2.3.6
	// settings.container-image-stream
	const dbSettingsKey = "settings." + config.ContainerImageStreamKey

	var ops []txn.Op
	var doc settingsDoc
	for iter.Next(&doc) {
		if modelDocIDs.Contains(doc.DocID) {
			// this is a model document, whatever was set here should stay
			continue
		}
		if stream, ok := doc.Settings[config.ContainerImageStreamKey]; !ok {
			// doesn't contain ContainerImageStreamKey
			continue
		} else if stream != "released" {
			// definitely wasn't set by the 2.3.6 upgrade step
			continue
		}
		// We just unset the one field that we accidentally set before, so we
		// don't have to worry about serialization of the other keys in the
		// document.
		ops = append(ops, txn.Op{
			C:      settingsC,
			Id:     doc.DocID,
			Assert: txn.DocExists,
			Update: bson.M{"$unset": bson.M{dbSettingsKey: 1}},
		})
	}
	if err := iter.Close(); err != nil {
		return errors.Trace(err)
	}
	if len(ops) > 0 {
		return errors.Trace(st.runRawTransaction(ops))
	}
	return nil
}

// ReplicaSetMembers gets the members of the current Mongo replica
// set. These are needed to bootstrap the raft cluster in an upgrade
// and using MongoSession directly from an upgrade steps would make
// testing difficult.
func ReplicaSetMembers(pool *StatePool) ([]replicaset.Member, error) {
	return replicaset.CurrentMembers(pool.SystemState().MongoSession())
}

// MigrateStorageMachineIdFields updates the various storage collections
// to copy any machineid field value across to hostid.
func MigrateStorageMachineIdFields(pool *StatePool) error {
	return runForAllModelStates(pool, migrateStorageMachineIds)
}

func migrateStorageMachineIds(st *State) error {
	var needsUpgradeTerm = bson.D{
		{"machineid", bson.D{{"$exists", true}}},
		{"hostid", bson.D{{"$exists", false}}},
	}

	var ops []txn.Op
	addUpgradeOps := func(collName string) error {
		storageColl, cleanup := st.db().GetCollection(collName)
		defer cleanup()

		var storageData []bson.M
		err := storageColl.Find(needsUpgradeTerm).All(&storageData)
		if err != nil && err != mgo.ErrNotFound {
			return errors.Trace(err)
		}

		for _, data := range storageData {
			machineId := data["machineid"]
			ops = append(ops, txn.Op{
				C:      collName,
				Id:     data["_id"],
				Assert: txn.DocExists,
				Update: bson.D{
					{"$set", bson.D{{"hostid", machineId}}},
					{"$unset", bson.D{{"machineid", nil}}},
				},
			})
		}
		return nil
	}

	for _, collName := range []string{volumesC, filesystemsC, volumeAttachmentsC, filesystemAttachmentsC} {
		if err := addUpgradeOps(collName); err != nil {
			return errors.Trace(err)
		}
	}
	if len(ops) > 0 {
		return errors.Trace(st.db().RunTransaction(ops))
	}
	return nil
}

// LegacyLeases returns information about all of the leases in the
// state-based lease store.
func LegacyLeases(pool *StatePool, localTime time.Time) (map[corelease.Key]corelease.Info, error) {
	st := pool.SystemState()
	reader, err := globalclock.NewReader(globalclock.ReaderConfig{
		Config: globalclock.Config{
			Collection: globalClockC,
			Mongo:      &environMongo{state: st},
		},
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	globalTime, err := reader.Now()

	// This needs to be the raw collection so we see all leases across
	// models.
	leaseCollection, closer := st.db().GetRawCollection(leasesC)
	defer closer()
	iter := leaseCollection.Find(nil).Iter()
	results := make(map[corelease.Key]corelease.Info)

	var doc struct {
		Namespace string        `bson:"namespace"`
		ModelUUID string        `bson:"model-uuid"`
		Name      string        `bson:"name"`
		Holder    string        `bson:"holder"`
		Start     int64         `bson:"start"`
		Duration  time.Duration `bson:"duration"`
	}

	for iter.Next(&doc) {
		startTime := time.Unix(0, doc.Start)
		globalExpiry := startTime.Add(doc.Duration)
		remaining := globalExpiry.Sub(globalTime)
		localExpiry := localTime.Add(remaining)
		key := corelease.Key{
			Namespace: doc.Namespace,
			ModelUUID: doc.ModelUUID,
			Lease:     doc.Name,
		}
		results[key] = corelease.Info{
			Holder:   doc.Holder,
			Expiry:   localExpiry,
			Trapdoor: nil,
		}
	}
	if err := iter.Close(); err != nil {
		return nil, errors.Trace(err)
	}
	return results, nil
}

// MigrateAddModelPermissions converts add-model permissions on the controller
// to add-model permissions on the controller cloud.
func MigrateAddModelPermissions(pool *StatePool) error {
	st := pool.SystemState()
	controllerInfo, err := st.ControllerInfo()
	if err != nil {
		return errors.Trace(err)
	}
	coll, closer := st.db().GetRawCollection(permissionsC)
	defer closer()

	query := bson.M{
		"_id":    bson.M{"$regex": "^" + controllerKey(st.ControllerUUID()) + "#us#.*"},
		"access": "add-model",
	}
	iter := coll.Find(query).Iter()

	var doc struct {
		DocId            string `bson:"_id"`
		ObjectGlobalKey  string `bson:"object-global-key"`
		SubjectGlobalKey string `bson:"subject-global-key"`
		Access           string `bson:"access"`
	}
	var ops []txn.Op

	// Set all the existng controller add-model permissions back to login.
	// Create a new cloud permission for add-model.
	for iter.Next(&doc) {
		ops = append(ops, txn.Op{
			C:      permissionsC,
			Id:     doc.DocId,
			Assert: txn.DocExists,
			Update: bson.M{"$set": bson.M{"access": "login"}},
		})
		ops = append(ops,
			createPermissionOp(cloudGlobalKey(controllerInfo.CloudName), doc.SubjectGlobalKey, permission.AddModelAccess))
	}

	if err := iter.Close(); err != nil {
		return errors.Trace(err)
	}
	if len(ops) > 0 {
		return errors.Trace(st.runRawTransaction(ops))
	}
	return nil
}

// SetEnableDiskUUIDOnVsphere updates the settings for all vsphere
// models to have enable-disk-uuid=false. The new default is true, but
// this maintains the previous behaviour for upgraded models.
func SetEnableDiskUUIDOnVsphere(pool *StatePool) error {
	return errors.Trace(applyToAllModelSettings(pool.SystemState(), func(doc *settingsDoc) (bool, error) {
		typeVal, found := doc.Settings["type"]
		if !found {
			return false, nil
		}
		typeStr, ok := typeVal.(string)
		if !ok || typeStr != "vsphere" {
			return false, nil
		}
		_, found = doc.Settings["enable-disk-uuid"]
		if found {
			// If the config option's already been set don't change
			// it.
			return false, nil
		}
		doc.Settings["enable-disk-uuid"] = false
		return true, nil
	}))
}

// UpdateInheritedControllerConfig migrates the existing global
// settings doc keyed on "controller" to be keyed on the cloud name.
func UpdateInheritedControllerConfig(pool *StatePool) error {
	st := pool.SystemState()
	model, err := st.Model()
	if err != nil {
		return errors.Trace(err)
	}
	key := cloudGlobalKey(model.Cloud())

	var ops []txn.Op
	coll, closer := st.db().GetRawCollection(globalSettingsC)
	defer closer()
	iter := coll.FindId("controller").Iter()
	defer iter.Close()
	var doc settingsDoc
	for iter.Next(&doc) {
		ops = append(ops, txn.Op{
			C:      globalSettingsC,
			Id:     doc.DocID,
			Remove: true,
			Assert: txn.DocExists,
		})
		doc.DocID = key
		ops = append(ops, txn.Op{
			C:      globalSettingsC,
			Id:     key,
			Insert: doc,
			Assert: txn.DocMissing,
		})
	}
	if err := iter.Close(); err != nil {
		return errors.Trace(err)
	}
	if len(ops) > 0 {
		err = errors.Trace(st.runRawTransaction(ops))
		return err
	}
	return nil
}

// UpdateKubernetesStorageConfig sets default storage classes
// for operator and workload storage.
func UpdateKubernetesStorageConfig(pool *StatePool) error {
	return runForAllModelStates(pool, updateKubernetesStorageConfig)
}

func cloudSpec(
	st *State,
	cloudName, regionName string,
	credentialTag names.CloudCredentialTag,
) (environs.CloudSpec, error) {
	modelCloud, err := st.Cloud(cloudName)
	if err != nil {
		return environs.CloudSpec{}, errors.Trace(err)
	}

	var credential *cloud.Credential
	if credentialTag != (names.CloudCredentialTag{}) {
		credentialValue, err := st.CloudCredential(credentialTag)
		if err != nil {
			return environs.CloudSpec{}, errors.Trace(err)
		}
		cloudCredential := cloud.NewNamedCredential(credentialValue.Name,
			cloud.AuthType(credentialValue.AuthType),
			credentialValue.Attributes,
			credentialValue.Revoked,
		)
		credential = &cloudCredential
	}

	return environs.MakeCloudSpec(modelCloud, regionName, credential)
}

// NewBroker returns a CAAS broker.
// Override for testing.
var NewBroker caas.NewContainerBrokerFunc = caas.New

func updateKubernetesStorageConfig(st *State) error {
	model, err := st.Model()
	if err != nil || model.Type() == ModelTypeIAAS {
		return errors.Trace(err)
	}
	if model.Life() != Alive {
		// No need to update models that are going away; they may no
		// longer have settings to update.
		return nil
	}
	cred, ok := model.CloudCredential()
	if !ok {
		return nil
	}
	cfg, err := model.Config()
	if err != nil {
		return errors.Trace(err)
	}

	defaults, err := st.controllerInheritedConfig(model.Cloud())()
	if err != nil {
		return errors.Annotate(err, "getting cloud config")
	}
	operatorStorage, haveDefaultOperatorStorage := defaults[k8s.OperatorStorageKey]
	if !haveDefaultOperatorStorage {
		cloudSpec, err := cloudSpec(st, model.Cloud(), model.CloudRegion(), cred)
		if err != nil {
			return errors.Trace(err)
		}
		broker, err := NewBroker(environs.OpenParams{Cloud: cloudSpec, Config: cfg})
		if err != nil {
			return errors.Trace(err)
		}
		metadata, err := broker.GetClusterMetadata("")
		if err != nil {
			return errors.Trace(err)
		}
		if metadata.NominatedStorageClass == nil {
			return nil
		}
		operatorStorage = metadata.NominatedStorageClass.Name
		err = st.updateConfigDefaults(model.Cloud(), cloud.Attrs{
			k8s.OperatorStorageKey: operatorStorage,
			k8s.WorkloadStorageKey: operatorStorage, // use same storage for both
		}, nil)
		if err != nil {
			return errors.Trace(err)
		}
	}

	attrs := make(map[string]interface{})
	if _, ok := cfg.AllAttrs()[k8s.OperatorStorageKey]; !ok {
		attrs[k8s.OperatorStorageKey] = operatorStorage
	}
	if _, ok := cfg.AllAttrs()[k8s.WorkloadStorageKey]; !ok {
		attrs[k8s.WorkloadStorageKey] = operatorStorage

	}

	if len(attrs) == 0 {
		return nil
	}

	return model.UpdateModelConfig(attrs, nil)
}

// EnsureDefaultModificationStatus ensures that there is a modification status
// document for every machine in the statuses.
func EnsureDefaultModificationStatus(pool *StatePool) error {
	st := pool.SystemState()
	db := st.db()

	machineCol, machineCloser := db.GetRawCollection(machinesC)
	defer machineCloser()
	machineIter := machineCol.Find(nil).Iter()
	defer machineIter.Close()

	statusCol, statusCloser := db.GetRawCollection(statusesC)
	defer statusCloser()

	var ops []txn.Op
	var machine machineDoc
	updatedTime := st.clock().Now().UnixNano()
	for machineIter.Next(&machine) {
		// Since we are using a raw collection, we need to manually
		// ensure that we prefix the IDs with the model-uuid.
		localID := machineGlobalModificationKey(machine.Id)
		key := ensureModelUUID(machine.ModelUUID, localID)

		// We only need to migrate machines that don't have a modification
		// status document. So we need to first check if there is one, before
		// creating a txn.Op for the missing document.
		var doc statusDoc
		err := statusCol.Find(bson.D{{"_id", key}}).Select(bson.D{{"_id", 1}}).One(&doc)
		if err == nil {
			continue
		} else if err != mgo.ErrNotFound {
			return errors.Trace(err)
		}

		rawDoc := statusDoc{
			ModelUUID: machine.ModelUUID,
			Status:    status.Idle,
			Updated:   updatedTime,
		}
		ops = append(ops, txn.Op{
			C:      statusesC,
			Id:     key,
			Assert: txn.DocMissing,
			Insert: rawDoc,
		})
	}
	if err := machineIter.Close(); err != nil {
		return errors.Trace(err)
	}
	if len(ops) > 0 {
		return errors.Trace(st.runRawTransaction(ops))
	}
	return nil
}

// EnsureApplicationDeviceConstraints ensures that there is a device
// constraints document for every application.
func EnsureApplicationDeviceConstraints(pool *StatePool) error {
	st := pool.SystemState()
	db := st.db()

	applicationCol, applicationCloser := db.GetRawCollection(applicationsC)
	defer applicationCloser()
	applicationIter := applicationCol.Find(nil).Iter()
	defer applicationIter.Close()

	constraintsCol, constraintsCloser := db.GetRawCollection(deviceConstraintsC)
	defer constraintsCloser()

	var ops []txn.Op
	var application applicationDoc
	for applicationIter.Next(&application) {
		// Since we are using a raw collection, we need to manually
		// ensure that we prefix the IDs with the model-uuid.
		localID := applicationDeviceConstraintsKey(application.Name, application.CharmURL)
		key := ensureModelUUID(application.ModelUUID, localID)

		// We only need to migrate applications that don't have a device
		// constraints document. So we need to first check if there is one, before
		// creating a txn.Op for the missing document.
		var doc statusDoc
		err := constraintsCol.Find(bson.D{{"_id", key}}).Select(bson.D{{"_id", 1}}).One(&doc)
		if err == nil {
			continue
		} else if err != mgo.ErrNotFound {
			return errors.Trace(err)
		}

		ops = append(ops, txn.Op{
			C:      deviceConstraintsC,
			Id:     key,
			Assert: txn.DocMissing,
			Insert: deviceConstraintsDoc{},
		})
	}
	if err := applicationIter.Close(); err != nil {
		return errors.Trace(err)
	}
	if len(ops) > 0 {
		return errors.Trace(st.runRawTransaction(ops))
	}
	return nil
}

// RemoveInstanceCharmProfileDataCollection removes the
// instanceCharmProfileData collection on upgrade.
func RemoveInstanceCharmProfileDataCollection(pool *StatePool) error {
	db := pool.SystemState().MongoSession().DB(jujuDB)
	instanceCharmProfileData := db.C("instanceCharmProfileData")
	if err := instanceCharmProfileData.DropCollection(); err != nil {
		// If the namespace is already missing, that's fine.
		if isMgoNamespaceNotFound(err) {
			return nil
		}
		return errors.Annotate(err, "failed to drop instanceCharmProfileData collection")
	}
	return nil
}

// UpdateK8sModelNameIndex migrates k8s model indices to be based
// on the model owner rather than the cloud name.
func UpdateK8sModelNameIndex(pool *StatePool) error {
	st := pool.SystemState()

	models, closer := st.db().GetCollection(modelsC)
	defer closer()
	usermodelNames, closer2 := st.db().GetCollection(usermodelnameC)
	defer closer2()

	var ops []txn.Op
	var docs []bson.M
	err := models.Find(bson.D{{"type", ModelTypeCAAS}}).Select(bson.M{"cloud": 1, "name": 1, "owner": 1}).All(&docs)
	if err != nil {
		return errors.Trace(err)
	}

	for _, m := range docs {
		owner := m["owner"].(string)
		name := m["name"].(string)
		cloudName := m["cloud"].(string)
		oldId := userModelNameIndex(cloudName, name)
		expectedId := userModelNameIndex(owner, name)

		n, err := usermodelNames.FindId(expectedId).Count()
		if err != nil {
			return errors.Trace(err)
		}
		if n > 0 {
			continue
		}

		ops = append(ops, []txn.Op{{
			C:      usermodelnameC,
			Id:     oldId,
			Assert: txn.DocExists,
			Remove: true,
		}, {
			C:      usermodelnameC,
			Id:     expectedId,
			Assert: txn.DocMissing,
			Insert: bson.M{},
		}}...)
	}
	if len(ops) > 0 {
		return errors.Trace(st.runRawTransaction(ops))
	}
	return nil
}

// AddModelLogsSize to controller config.
func AddModelLogsSize(pool *StatePool) error {
	st := pool.SystemState()
	coll, closer := st.db().GetRawCollection(controllersC)
	defer closer()
	var doc settingsDoc
	if err := coll.FindId(controllerSettingsGlobalKey).One(&doc); err != nil {
		if err == mgo.ErrNotFound {
			return nil
		}
		return errors.Trace(err)
	}

	settingsChanged :=
		maybeUpdateSettings(doc.Settings, controller.ModelLogsSize, fmt.Sprintf("%vM", controller.DefaultModelLogsSizeMB))
	if settingsChanged {
		return errors.Trace(st.runRawTransaction(
			[]txn.Op{{
				C:      controllersC,
				Id:     doc.DocID,
				Assert: txn.DocExists,
				Update: bson.M{"$set": bson.M{"settings": doc.Settings}},
			}}))
	}
	return nil
}

// AddControllerNodeDocs creates controller nodes for each
// machine that wants to be a member of the mongo replicaset.
func AddControllerNodeDocs(pool *StatePool) error {
	st := pool.SystemState()

	machines, closer := st.db().GetRawCollection(machinesC)
	defer closer()
	controllerNodes, closer2 := st.db().GetRawCollection(controllerNodesC)
	defer closer2()

	var ops []txn.Op
	var docs []bson.M
	err := machines.Find(
		bson.D{{"jobs", bson.D{{"$in", []MachineJob{JobManageModel}}}}},
	).Select(bson.M{"_id": 1, "machineid": 1, "hasvote": 1, "novote": 1}).All(&docs)
	if err != nil {
		return errors.Trace(err)
	}

	for _, m := range docs {
		docId := m["_id"].(string)
		mid := m["machineid"].(string)
		hasvote, _ := m["hasvote"].(bool)
		novote, ok := m["novote"].(bool)
		if !ok {
			continue
		}
		wantsvote := !novote
		modelUUID, _, ok := splitDocID(docId)
		if !ok {
			logger.Warningf("unexpected machine doc id %q", docId)
			continue
		}

		expectedId := ensureModelUUID(modelUUID, mid)
		n, err := controllerNodes.FindId(expectedId).Count()
		if err != nil {
			return errors.Trace(err)
		}
		if n > 0 {
			continue
		}

		doc := &controllerNodeDoc{
			DocID:     ensureModelUUID(modelUUID, mid),
			HasVote:   hasvote,
			WantsVote: wantsvote,
		}
		ops = append(ops, txn.Op{
			C:      controllerNodesC,
			Id:     doc.DocID,
			Assert: txn.DocMissing,
			Insert: doc,
		})
	}
	if len(ops) > 0 {
		return errors.Trace(st.runRawTransaction(ops))
	}
	return nil
}

// AddSpaceIdToSpaceDocs ensures that every space document includes a
// a sequentially generated ID.
// It also adds a doc for the default space (ID=0).
func AddSpaceIdToSpaceDocs(pool *StatePool) (err error) {
	return errors.Trace(runForAllModelStates(pool, func(st *State) error {
		col, closer := st.db().GetCollection(spacesC)
		defer closer()

		type oldSpaceDoc struct {
			SpaceId    string `bson:"spaceid"`
			Life       Life   `bson:"life"`
			Name       string `bson:"name"`
			IsPublic   bool   `bson:"is-public"`
			ProviderId string `bson:"providerid,omitempty"`
		}

		var docs []oldSpaceDoc
		err := col.Find(nil).All(&docs)
		if err != nil {
			return errors.Trace(err)
		}

		var ops []txn.Op
		for _, oldDoc := range docs {
			// A doc with a space ID has already been upgraded.
			if oldDoc.SpaceId != "" {
				continue
			}

			// We cannot edit _id, so we need to delete and re-create each doc.
			ops = append(ops, txn.Op{
				C:      spacesC,
				Id:     oldDoc.Name,
				Assert: txn.DocExists,
				Remove: true,
			})

			seq, err := sequenceWithMin(st, "space", 1)
			if err != nil {
				return errors.Trace(err)
			}
			id := strconv.Itoa(seq)

			newDoc := spaceDoc{
				DocId:      st.docID(id),
				Id:         id,
				Life:       oldDoc.Life,
				Name:       oldDoc.Name,
				IsPublic:   oldDoc.IsPublic,
				ProviderId: oldDoc.ProviderId,
			}

			ops = append(ops, txn.Op{
				C:      spacesC,
				Id:     newDoc.DocId,
				Insert: newDoc,
			})
		}

		ops = append(ops, createDefaultSpaceOp())

		return errors.Trace(st.db().RunTransaction(ops))
	}))
}

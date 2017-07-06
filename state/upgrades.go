// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils/set"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/juju/names.v2"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/storage/provider"
)

var upgradesLogger = loggo.GetLogger("juju.state.upgrade")

// runForAllModelStates will run runner function for every model passing a state
// for that model.
func runForAllModelStates(st *State, runner func(st *State) error) error {
	models, closer := st.db().GetCollection(modelsC)
	defer closer()

	var modelDocs []bson.M
	err := models.Find(nil).Select(bson.M{"_id": 1}).All(&modelDocs)
	if err != nil {
		return errors.Annotate(err, "failed to read models")
	}

	for _, modelDoc := range modelDocs {
		modelUUID := modelDoc["_id"].(string)
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
func RenameAddModelPermission(st *State) error {
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
	if err := iter.Err(); err != nil {
		return errors.Trace(err)
	}
	return st.runRawTransaction(ops)
}

// StripLocalUserDomain removes any @local suffix from any relevant document field values.
func StripLocalUserDomain(st *State) error {
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
	if err := iter.Err(); err != nil {
		return nil, errors.Trace(err)
	}
	return ops, nil
}

// AddMigrationAttempt adds an "attempt" field to migration documents
// which are missing one.
func AddMigrationAttempt(st *State) error {
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
	if err := iter.Err(); err != nil {
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
func AddLocalCharmSequences(st *State) error {
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
					"regions." + region.Name + ".endpoint",
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
	if err := iter.Err(); err != nil {
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
func UpgradeNoProxyDefaults(st *State) error {
	var ops []txn.Op
	coll, closer := st.db().GetRawCollection(settingsC)
	defer closer()
	iter := coll.Find(bson.D{}).Iter()
	var doc settingsDoc
	for iter.Next(&doc) {
		noProxyVal := doc.Settings[config.NoProxyKey]
		noProxy, ok := noProxyVal.(string)
		if !ok {
			continue
		}
		noProxy = upgradeNoProxy(noProxy)
		doc.Settings[config.NoProxyKey] = noProxy
		ops = append(ops,
			txn.Op{
				C:      settingsC,
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

// AddNonDetachableStorageMachineId sets the "machineid" field on
// volume and filesystem docs that are inherently bound to that
// machine.
func AddNonDetachableStorageMachineId(st *State) error {
	return runForAllModelStates(st, addNonDetachableStorageMachineId)
}

func addNonDetachableStorageMachineId(st *State) error {
	im, err := st.IAASModel()
	if err != nil {
		return errors.Trace(err)
	}
	var ops []txn.Op
	volumes, err := im.volumes(
		bson.D{{"machineid", bson.D{{"$exists", false}}}},
	)
	if err != nil {
		return errors.Trace(err)
	}
	for _, v := range volumes {
		var pool string
		if v.doc.Info != nil {
			pool = v.doc.Info.Pool
		} else if v.doc.Params != nil {
			pool = v.doc.Params.Pool
		}
		detachable, err := isDetachableVolumePool(im, pool)
		if err != nil {
			return errors.Trace(err)
		}
		if detachable {
			continue
		}
		attachments, err := im.VolumeAttachments(v.VolumeTag())
		if err != nil {
			return errors.Trace(err)
		}
		if len(attachments) != 1 {
			// There should be exactly one attachment since the
			// filesystem is non-detachable, but be defensive
			// and leave the document alone if our expectations
			// are not met.
			continue
		}
		machineId := attachments[0].Machine().Id()
		ops = append(ops, txn.Op{
			C:      volumesC,
			Id:     v.doc.Name,
			Assert: txn.DocExists,
			Update: bson.D{{"$set", bson.D{
				{"machineid", machineId},
			}}},
		})
	}
	filesystems, err := im.filesystems(
		bson.D{{"machineid", bson.D{{"$exists", false}}}},
	)
	if err != nil {
		return errors.Trace(err)
	}
	for _, f := range filesystems {
		var pool string
		if f.doc.Info != nil {
			pool = f.doc.Info.Pool
		} else if f.doc.Params != nil {
			pool = f.doc.Params.Pool
		}
		if detachable, err := isDetachableFilesystemPool(im, pool); err != nil {
			return errors.Trace(err)
		} else if detachable {
			continue
		}
		attachments, err := im.FilesystemAttachments(f.FilesystemTag())
		if err != nil {
			return errors.Trace(err)
		}
		if len(attachments) != 1 {
			// There should be exactly one attachment since the
			// filesystem is non-detachable, but be defensive
			// and leave the document alone if our expectations
			// are not met.
			continue
		}
		machineId := attachments[0].Machine().Id()
		ops = append(ops, txn.Op{
			C:      filesystemsC,
			Id:     f.doc.DocID,
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
func RemoveNilValueApplicationSettings(st *State) error {
	coll, closer := st.db().GetRawCollection(settingsC)
	defer closer()
	iter := coll.Find(bson.M{"_id": bson.M{"$regex": "^.*:a#.*"}}).Iter()
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
	if len(ops) > 0 {
		return errors.Trace(st.runRawTransaction(ops))
	}
	return nil
}

// AddControllerLogCollectionsSizeSettings adds the controller
// settings to control log pruning and txn log size if they are missing.
func AddControllerLogCollectionsSizeSettings(st *State) error {
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
	settingsChanged := maybeUpdateSettings(doc.Settings, controller.MaxLogsAge, fmt.Sprintf("%vh", controller.DefaultMaxLogsAgeDays*24))
	settingsChanged =
		maybeUpdateSettings(doc.Settings, controller.MaxLogsSize, fmt.Sprintf("%vM", controller.DefaultMaxLogCollectionMB)) || settingsChanged
	settingsChanged =
		maybeUpdateSettings(doc.Settings, controller.MaxTxnLogSize, fmt.Sprintf("%vM", controller.DefaultMaxTxnLogCollectionMB)) || settingsChanged
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

// AddStatusHistoryPruneSettings adds the model settings
// to control log pruning if they are missing.
func AddStatusHistoryPruneSettings(st *State) error {
	coll, closer := st.db().GetRawCollection(settingsC)
	defer closer()

	models, err := st.AllModels()
	if err != nil {
		return errors.Trace(err)
	}
	var ids []string
	for _, m := range models {
		ids = append(ids, m.UUID()+":e")
	}

	iter := coll.Find(bson.M{"_id": bson.M{"$in": ids}}).Iter()
	var ops []txn.Op
	var doc settingsDoc
	for iter.Next(&doc) {
		settingsChanged :=
			maybeUpdateSettings(doc.Settings, config.MaxStatusHistoryAge, config.DefaultStatusHistoryAge)
		settingsChanged =
			maybeUpdateSettings(doc.Settings, config.MaxStatusHistorySize, config.DefaultStatusHistorySize) || settingsChanged
		if settingsChanged {
			ops = append(ops, txn.Op{
				C:      settingsC,
				Id:     doc.DocID,
				Assert: txn.DocExists,
				Update: bson.M{"$set": bson.M{"settings": doc.Settings}},
			})
		}
	}
	if len(ops) > 0 {
		return errors.Trace(st.runRawTransaction(ops))
	}
	return nil
}

// AddUpdateStatusHookSettings adds the model settings
// to control how often to run the update-status hook
// if they are missing.
func AddUpdateStatusHookSettings(st *State) error {
	coll, closer := st.db().GetRawCollection(settingsC)
	defer closer()

	models, err := st.AllModels()
	if err != nil {
		return errors.Trace(err)
	}
	var ids []string
	for _, m := range models {
		ids = append(ids, m.UUID()+":e")
	}

	iter := coll.Find(bson.M{"_id": bson.M{"$in": ids}}).Iter()
	var ops []txn.Op
	var doc settingsDoc
	for iter.Next(&doc) {
		settingsChanged :=
			maybeUpdateSettings(doc.Settings, config.UpdateStatusHookInterval, config.DefaultUpdateStatusHookInterval)
		if settingsChanged {
			ops = append(ops, txn.Op{
				C:      settingsC,
				Id:     doc.DocID,
				Assert: txn.DocExists,
				Update: bson.M{"$set": bson.M{"settings": doc.Settings}},
			})
		}
	}
	if len(ops) > 0 {
		return errors.Trace(st.runRawTransaction(ops))
	}
	return nil
}

// AddStorageInstanceConstraints sets the "constraints" field on
// storage instance docs.
func AddStorageInstanceConstraints(st *State) error {
	return runForAllModelStates(st, addStorageInstanceConstraints)
}

func addStorageInstanceConstraints(st *State) error {
	im, err := st.IAASModel()
	if err != nil {
		return errors.Trace(err)
	}
	storageInstances, err := st.storageInstances(bson.D{
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
			v, err := im.storageInstanceVolume(s.StorageTag())
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
			f, err := im.storageInstanceFilesystem(s.StorageTag())
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
func SplitLogCollections(st *State) error {
	session := st.MongoSession()
	db := session.DB(logsDB)
	oldLogs := db.C("logs")

	// If we haven't seen any particular model, we need to initialise
	// the logs collection with the right indices.
	seen := set.NewStrings()

	iter := oldLogs.Find(nil).Iter()
	var doc bson.M

	for iter.Next(&doc) {
		modelUUID := doc["e"].(string)
		newCollName := logCollectionName(modelUUID)
		newLogs := db.C(newCollName)

		if !seen.Contains(newCollName) {
			if err := InitDbLogs(session, modelUUID); err != nil {
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
func CorrectRelationUnitCounts(st *State) error {
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

func unitApp(unitName string) string {
	unitParts := strings.Split(unitName, "/")
	return unitParts[0]
}

func extractPrincipalUnitApp(scopeKeyParts []string) (string, bool) {
	if len(scopeKeyParts) < 5 {
		return "", false
	}
	return unitApp(scopeKeyParts[2]), true
}

func otherEndIsSubordinate(relation *relationUnitCountInfo, unitName, modelUUID string, applications map[string]bool) (bool, error) {
	app, err := relation.otherEnd(unitApp(unitName))
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
func AddModelEnvironVersion(st *State) error {
	coll, closer := st.db().GetCollection(modelsC)
	defer closer()

	var doc struct {
		UUID           string `bson:"_id"`
		Cloud          string `bson:"cloud"`
		EnvironVersion *int   `bson:"environ-version,omitempty"`
	}

	var ops []txn.Op
	iter := coll.Find(nil).Iter()
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

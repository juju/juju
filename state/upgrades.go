// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/juju/charm/v9"
	"github.com/juju/collections/set"
	"github.com/juju/errors"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/network"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/mongo/utils"
	"github.com/juju/loggo"
	"github.com/juju/mgo/v2"
	"github.com/juju/mgo/v2/bson"
	"github.com/juju/mgo/v2/txn"
	"github.com/juju/names/v4"
	"github.com/juju/replicaset/v2"
)

var upgradesLogger = loggo.GetLogger("juju.state.upgrade")

// MaxDocOps defines the number of documents to put into a single transaction,
// if we make this number too large, mongo and client side transactions
// struggle to complete. It is unclear what the optimum is, but without some
// sort of cap, we've seen txns try to touch 100k docs which makes things fail.
const MaxDocOps = 2000

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
	st, err := pool.SystemState()
	if err != nil {
		return errors.Trace(err)
	}
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
	st, err := pool.SystemState()
	if err != nil {
		return errors.Trace(err)
	}
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
	st, err := pool.SystemState()
	if err != nil {
		return errors.Trace(err)
	}
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
	st, err := pool.SystemState()
	if err != nil {
		return errors.Trace(err)
	}
	charmsColl, closer := st.db().GetRawCollection(charmsC)
	defer closer()

	query := bson.M{
		"url": bson.M{"$regex": "^local:"},
	}
	var docs []bson.M
	err = charmsColl.Find(query).Select(bson.M{
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

func KubernetesInClusterCredentialSpec(
	pool *StatePool,
) (environscloudspec.CloudSpec, *config.Config, string, error) {
	st, err := pool.SystemState()
	if err != nil {
		return environscloudspec.CloudSpec{}, nil, "", errors.Trace(err)
	}
	model, err := st.Model()
	if err != nil {
		return environscloudspec.CloudSpec{}, nil, "", errors.Trace(err)
	}

	if model.Type() != ModelTypeCAAS {
		return environscloudspec.CloudSpec{}, nil, "",
			errors.NotFoundf("controller model %q not a caas model", model.Name())
	}

	cred, ok := model.CloudCredentialTag()
	if !ok {
		return environscloudspec.CloudSpec{}, nil, "",
			errors.NotFoundf("controller cloud credentials")
	}

	cloudSpec, err := cloudSpec(st, model.CloudName(), model.CloudRegion(), cred)
	if err != nil {
		return cloudSpec, nil, "",
			errors.Annotate(err, "fetching controller cloud spec")
	}

	if cloudSpec.Type != "kubernetes" {
		return cloudSpec, nil, "",
			errors.NotFoundf("controller not in a Kubernetes cloud")
	}

	if !cloudSpec.IsControllerCloud {
		return cloudSpec, nil, "",
			errors.NotFoundf("cloudspec is not in the controllers cloud")
	}

	cfg, err := model.Config()
	if err != nil {
		return cloudSpec, cfg, "",
			errors.Annotate(err, "getting model configuration")
	}

	controllerConfig, err := st.ControllerConfig()
	if err != nil {
		return cloudSpec, cfg, "",
			errors.Annotate(err, "fetching controller UUID")
	}
	controllerUUID := controllerConfig[controller.ControllerUUIDKey].(string)
	return cloudSpec, cfg, controllerUUID, nil
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
	st, err := pool.SystemState()
	if err != nil {
		return errors.Trace(err)
	}
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

// AddControllerLogCollectionsSizeSettings adds the controller
// settings to control log pruning and txn log size if they are missing.
func AddControllerLogCollectionsSizeSettings(pool *StatePool) error {
	st, err := pool.SystemState()
	if err != nil {
		return errors.Trace(err)
	}
	coll, closer := st.db().GetRawCollection(controllersC)
	defer closer()
	var doc settingsDoc
	if err := coll.FindId(ControllerSettingsGlobalKey).One(&doc); err != nil {
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

// AddStatusHistoryPruneSettings adds the model settings
// to control log pruning if they are missing.
func AddStatusHistoryPruneSettings(pool *StatePool) error {
	st, err := pool.SystemState()
	if err != nil {
		return errors.Trace(err)
	}
	err = applyToAllModelSettings(st, func(doc *settingsDoc) (bool, error) {
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
	st, err := pool.SystemState()
	if err != nil {
		return errors.Trace(err)
	}
	err = applyToAllModelSettings(st, func(doc *settingsDoc) (bool, error) {
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

// ReplicaSetMembers gets the members of the current Mongo replica
// set. These are needed to bootstrap the raft cluster in an upgrade
// and using MongoSession directly from an upgrade steps would make
// testing difficult.
func ReplicaSetMembers(pool *StatePool) ([]replicaset.Member, error) {
	st, err := pool.SystemState()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return replicaset.CurrentMembers(st.MongoSession())
}

func cloudSpec(
	st *State,
	cloudName, regionName string,
	credentialTag names.CloudCredentialTag,
) (environscloudspec.CloudSpec, error) {
	modelCloud, err := st.Cloud(cloudName)
	if err != nil {
		return environscloudspec.CloudSpec{}, errors.Trace(err)
	}

	var credential *cloud.Credential
	if credentialTag != (names.CloudCredentialTag{}) {
		credentialValue, err := st.CloudCredential(credentialTag)
		if err != nil {
			return environscloudspec.CloudSpec{}, errors.Trace(err)
		}
		cloudCredential := cloud.NewNamedCredential(credentialValue.Name,
			cloud.AuthType(credentialValue.AuthType),
			credentialValue.Attributes,
			credentialValue.Revoked,
		)
		credential = &cloudCredential
	}

	return environscloudspec.MakeCloudSpec(modelCloud, regionName, credential)
}

func RemoveUnusedLinkLayerDeviceProviderIDs(pool *StatePool) error {
	st, err := pool.SystemState()
	if err != nil {
		return errors.Trace(err)
	}

	const idType = "linklayerdevice"
	idTypeExp := fmt.Sprintf("^.*:%s:.*$", idType)

	lldCol, lldCloser := st.db().GetRawCollection(linkLayerDevicesC)
	defer lldCloser()

	// Gather the full qualified IDs for used link-layer device provider IDs.
	used := set.NewStrings()
	var doc struct {
		ModelUUID  string `bson:"model-uuid"`
		ProviderID string `bson:"providerid"`
	}
	iter := lldCol.Find(bson.M{"providerid": bson.M{"$exists": true}}).Iter()
	for iter.Next(&doc) {
		used.Add(strings.Join([]string{doc.ModelUUID, idType, doc.ProviderID}, ":"))
	}
	if err := iter.Close(); err != nil {
		return errors.Trace(err)
	}

	pidCol, pidCloser := st.db().GetRawCollection(providerIDsC)
	defer pidCloser()

	// Delete all link-layer device provider IDs we didn't find.
	// Get a count before and after for logging the delta.
	before, err := pidCol.Find(nil).Count()
	if err != nil {
		return errors.Trace(err)
	}

	_, err = pidCol.RemoveAll(bson.D{{
		"$and", []bson.D{
			{{"_id", bson.D{{"$regex", idTypeExp}}}},
			{{"_id", bson.D{{"$nin", used.Values()}}}},
		},
	}})
	if err != nil {
		return errors.Trace(err)
	}

	after, err := pidCol.Find(nil).Count()
	if err != nil {
		return errors.Trace(err)
	}

	logger.Infof("deleted %d unused link-layer device provider IDs", before-after)
	return nil
}

// UpdateDHCPAddressConfigs ensures that any addresses in the ip.addresses
// collection with the removed "dynamic" address configuration method are
// updated to indicate the "dhcp" configuration method.
func UpdateDHCPAddressConfigs(pool *StatePool) error {
	st, err := pool.SystemState()
	if err != nil {
		return errors.Trace(err)
	}

	col, closer := st.db().GetRawCollection(ipAddressesC)
	defer closer()

	iter := col.Find(bson.M{"config-method": "dynamic"}).Iter()

	var ops []txn.Op
	var doc bson.M
	for iter.Next(&doc) {
		ops = append(ops, txn.Op{
			C:      ipAddressesC,
			Id:     doc["_id"],
			Assert: txn.DocExists,
			Update: bson.M{"$set": bson.M{"config-method": network.ConfigDHCP}},
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

func AddSpawnedTaskCountToOperations(pool *StatePool) error {
	st, err := pool.SystemState()
	if err != nil {
		return errors.Trace(err)
	}

	opsCol, closer := st.db().GetRawCollection(operationsC)
	defer closer()
	iter := opsCol.Find(nil).Iter()

	actionsCol, closer := st.db().GetRawCollection(actionsC)
	defer closer()

	var ops []txn.Op
	var doc operationDoc
	for iter.Next(&doc) {
		_, localID, ok := splitDocID(doc.DocId)
		if !ok {
			return errors.Errorf("bad data, operation _id %s", doc.DocId)
		}
		criteria := bson.D{
			{"model-uuid", doc.ModelUUID},
			{"operation", localID},
		}
		count, err := actionsCol.Find(criteria).Count()
		if err != nil {
			return errors.Trace(err)
		}
		ops = append(ops, txn.Op{
			C:      operationsC,
			Id:     doc.DocId,
			Assert: txn.DocExists,
			Update: bson.M{"$set": bson.M{"spawned-task-count": count}},
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

func TransformEmptyManifestsToNil(pool *StatePool) error {
	return errors.Trace(runForAllModelStates(pool, func(st *State) error {
		col, closer := st.db().GetCollection(charmsC)
		defer closer()

		var docs []charmDoc
		if err := col.Find(nil).All(&docs); err != nil {
			return errors.Trace(err)
		}

		var ops []txn.Op
		for _, doc := range docs {
			if doc.Manifest == nil || len(doc.Manifest.Bases) == 0 {
				ops = append(ops, txn.Op{
					C:      charmsC,
					Id:     doc.DocID,
					Assert: txn.DocExists,
					Update: bson.D{{
						"$unset", bson.D{{
							"manifest", nil,
						}},
					}},
				})
			}
		}
		if len(ops) > 0 {
			return errors.Trace(st.db().RunTransaction(ops))
		}
		return nil
	}))
}

func EnsureCharmOriginRisk(pool *StatePool) error {
	return errors.Trace(runForAllModelStates(pool, func(st *State) error {
		col, closer := st.db().GetCollection(applicationsC)
		defer closer()

		var docs []applicationDoc
		if err := col.Find(nil).All(&docs); err != nil {
			return errors.Trace(err)
		}

		var ops []txn.Op
		for _, doc := range docs {
			// It is expected that every application should have a charm URL.
			charmURL, err := charm.ParseURL(*doc.CharmURL)
			if err != nil {
				return errors.Annotatef(err, "parsing charm url")
			}

			if charmURL.Schema == "local" {
				continue
			}

			// This should never happen, instead we should always have one.
			// See: AddCharmOriginToApplications
			if doc.CharmOrigin == nil {
				continue
			}

			// If the "cs-channel" is empty, then we want to ensure that the
			// channel isn't just empty, but also set to something useful.
			channel := doc.Channel
			if channel == "" {
				channel = "stable"
			}

			var originChannel *Channel
			if doc.CharmOrigin.Channel == nil {
				originChannel = &Channel{
					Risk: channel,
				}
			} else if doc.CharmOrigin.Channel.Risk == "" {
				originChannel = &Channel{
					Risk:   channel,
					Track:  doc.CharmOrigin.Channel.Track,
					Branch: doc.CharmOrigin.Channel.Branch,
				}
			}
			// Nothing to do, we have a valid channel.
			if originChannel == nil {
				continue
			}

			ops = append(ops, txn.Op{
				C:      applicationsC,
				Id:     doc.DocID,
				Assert: txn.DocExists,
				Update: bson.D{{
					"$set", bson.D{{
						"charm-origin.channel", originChannel,
					}},
				}},
			})
		}
		if len(ops) > 0 {
			return errors.Trace(st.db().RunTransaction(ops))
		}
		return nil
	}))
}

func RemoveOrphanedCrossModelProxies(pool *StatePool) error {
	return errors.Trace(runForAllModelStates(pool, func(st *State) error {
		col, closer := st.db().GetCollection(applicationOffersC)
		defer closer()

		// Ideally we'd manipulate the collection data directly, but the
		// operations to remove remotes apps are too complex to craft by hand.
		allRemoteApps, err := st.AllRemoteApplications()
		if err != nil {
			return errors.Trace(err)
		}

		var appsToRemove []*RemoteApplication
		for _, app := range allRemoteApps {
			// We only want this for the offering side.
			if !app.IsConsumerProxy() {
				continue
			}
			num, err := col.Find(bson.D{{"offer-uuid", app.OfferUUID()}}).Count()
			if err != nil {
				return errors.Trace(err)
			}
			if num == 0 {
				appsToRemove = append(appsToRemove, app)
			}
		}

		for _, app := range appsToRemove {
			op := app.DestroyOperation(true)
			if err := st.ApplyOperation(op); err != nil {
				return errors.Trace(err)
			}
		}
		return nil
	}))
}

// DropLegacyAssumesSectionsFromCharmMetadata drops any existing "assumes"
// fields in the charms collection. This is because earlier Juju versions
// prematurely introduced an assumes field (a []string) before the assumes spec
// was finalized and while no charms out there use assumes expressions.
//
// This decision, coupled with the fact that the metadata structs from the
// charm package are directly serialized to BSON instead of being mapped
// to a struct maintained within the state package necessitates this upgrade
// step.
func DropLegacyAssumesSectionsFromCharmMetadata(pool *StatePool) error {
	return errors.Trace(runForAllModelStates(pool, func(st *State) error {
		col, closer := st.db().GetCollection(charmsC)
		defer closer()

		err := col.Writeable().Update(
			bson.M{
				"assumes": bson.M{"$exists": true},
			},
			bson.M{
				"$unset": bson.M{"assumes": ""},
			},
		)

		// Ignore errors about empty charms collections
		if err != nil && err != mgo.ErrNotFound {
			return errors.Trace(err)
		}

		return nil
	}))
}

// MigrateLegacyCrossModelTokens updates the remoteEntities collection
// to fix a potential legacy Juju 2.5.1 issue.
func MigrateLegacyCrossModelTokens(pool *StatePool) error {
	return errors.Trace(runForAllModelStates(pool, func(st *State) error {
		entities, closer := st.db().GetCollection(remoteEntitiesC)
		defer closer()

		offers, closer := st.db().GetCollection(applicationOffersC)
		defer closer()

		var docs []remoteEntityDoc
		if err := entities.Find(nil).All(&docs); err != nil {
			return errors.Trace(err)
		}

		var ops []txn.Op
		for _, entityDoc := range docs {
			modelUUID, originalID, ok := splitDocID(entityDoc.DocID)
			if !ok {
				return errors.Errorf("bad data, remote entity _id %s", entityDoc.DocID)
			}
			tag, err := names.ParseTag(originalID)
			if err != nil {
				return errors.Errorf("bad data, invalid entity tag %q", originalID)
			}
			// We only want to deal with application tags.
			if tag.Kind() != names.ApplicationTagKind {
				continue
			}

			// Check to see if there's any records using the
			// offer application name instead of the offer name.
			var matchingOffers []applicationOfferDoc
			err = offers.Find(bson.D{{"application-name", tag.Id()}}).All(&matchingOffers)
			if err != nil {
				return errors.Trace(err)
			}
			if len(matchingOffers) == 0 {
				continue
			}
			ops = append(ops, txn.Op{
				C:      remoteEntitiesC,
				Id:     entityDoc.DocID,
				Remove: true,
			})
			// If there's only 1, we know what the offer should be.
			// If there's > 1, its ambiguous so best just to delete
			// the ambiguous record.
			if len(matchingOffers) != 1 {
				continue
			}
			entityDoc.DocID = ensureModelUUID(
				modelUUID,
				names.NewApplicationTag(matchingOffers[0].OfferName).String())
			ops = append(ops, txn.Op{
				C:      remoteEntitiesC,
				Id:     entityDoc.DocID,
				Insert: entityDoc,
			})
		}
		if len(ops) > 0 {
			return errors.Trace(st.db().RunTransaction(ops))
		}
		return nil
	}))
}

// CleanupDeadAssignUnits removes all dead or removed applications' the assignunits documents.
func CleanupDeadAssignUnits(pool *StatePool) error {
	return errors.Trace(runForAllModelStates(pool, func(st *State) error {
		unitAssignments, err := st.AllUnitAssignments()
		if err != nil {
			return errors.Trace(err)
		}
		var ops []txn.Op
		deadOrRemovedApps := set.NewStrings()
		for _, ua := range unitAssignments {
			appName, err := names.UnitApplication(ua.Unit)
			if err != nil {
				return errors.Trace(err)
			}
			if deadOrRemovedApps.Contains(appName) {
				ops = append(ops, removeStagedAssignmentOp(st.docID(ua.Unit)))
				continue
			}
			app, err := st.Application(appName)
			if err != nil && !errors.IsNotFound(err) {
				return errors.Trace(err)
			}
			if errors.IsNotFound(err) || app.Life() == Dead {
				deadOrRemovedApps.Add(appName)
				ops = append(ops, removeStagedAssignmentOp(st.docID(ua.Unit)))
			}
		}
		if len(ops) > 0 {
			return errors.Trace(st.db().RunTransaction(ops))
		}
		return nil
	}))
}

// RemoveOrphanedLinkLayerDevices removes link-layer devices and addresses
// that have no corresponding machine in the model.
// This situation could occur in the past for force-destroyed machines.
func RemoveOrphanedLinkLayerDevices(pool *StatePool) error {
	return errors.Trace(runForAllModelStates(pool, func(st *State) error {
		machines, mCloser := st.db().GetCollection(machinesC)
		defer mCloser()
		iter := machines.Find(nil).Iter()

		machineIDs := set.NewStrings()
		var mDoc struct {
			ID string `bson:"machineid"`
		}
		for iter.Next(&mDoc) {
			machineIDs.Add(mDoc.ID)
		}

		if err := iter.Close(); err != nil {
			return errors.Trace(err)
		}

		linkLayerDevices, lldCloser := st.db().GetCollection(linkLayerDevicesC)
		defer lldCloser()
		iter = linkLayerDevices.Find(nil).Iter()

		var devDoc linkLayerDeviceDoc
		for iter.Next(&devDoc) {
			if machineIDs.Contains(devDoc.MachineID) {
				continue
			}
			if err := newLinkLayerDevice(st, devDoc).Remove(); err != nil {
				_ = iter.Close()
				return errors.Trace(err)
			}
		}

		return errors.Trace(iter.Close())
	}))
}

// UpdateExternalControllerInfo sets the source controller UUID for any
// consumer side remote apps whose offer is hosted in another controller.
func UpdateExternalControllerInfo(pool *StatePool) error {
	// First remove any orphaned external controllers which are not
	// referenced by any SAAS application. This is global operation
	// so do it using the system state.
	st, err := pool.SystemState()
	if err != nil {
		return errors.Trace(err)
	}
	extControllers, cCloser := st.db().GetCollection(externalControllersC)
	defer cCloser()
	iter := extControllers.Find(nil).Iter()

	var extControllerDoc struct {
		DocID  string   `bson:"_id"`
		Models []string `bson:"models"`
	}

	// Load all external controllers and then remove the ones
	// in use to know which ones are orphaned.
	orphanedControllers := set.NewStrings()
	modelControllers := make(map[string]string) // Used below to update applications.
	for iter.Next(&extControllerDoc) {
		orphanedControllers.Add(extControllerDoc.DocID)
		for _, modelUUID := range extControllerDoc.Models {
			modelControllers[modelUUID] = extControllerDoc.DocID
		}
	}
	if err := iter.Close(); err != nil {
		return errors.Trace(err)
	}

	refCountPerController := make(map[string]int)
	err = errors.Trace(runForAllModelStates(pool, func(st *State) error {
		remoteApps, rCloser := st.db().GetCollection(remoteApplicationsC)
		defer rCloser()
		iter = remoteApps.Find(bson.D{{"is-consumer-proxy", false}}).Iter()

		var appDoc struct {
			DocID                string `bson:"_id"`
			SourceControllerUUID string `bson:"source-controller-uuid"`
			SourceModelUUID      string `bson:"source-model-uuid"`
		}

		var ops []txn.Op
		for iter.Next(&appDoc) {
			if appDoc.SourceControllerUUID != "" {
				orphanedControllers.Remove(appDoc.SourceControllerUUID)
				refCountPerController[appDoc.SourceControllerUUID] = refCountPerController[appDoc.SourceControllerUUID] + 1
				continue
			}
			controllerUUID, ok := modelControllers[appDoc.SourceModelUUID]
			if !ok {
				continue
			}
			orphanedControllers.Remove(controllerUUID)
			ops = append(ops, txn.Op{
				C:  remoteApplicationsC,
				Id: appDoc.DocID,
				Update: bson.D{{"$set", bson.D{{
					"source-controller-uuid", controllerUUID}},
				}},
			})
			refCountPerController[controllerUUID] = refCountPerController[controllerUUID] + 1
		}
		if err := iter.Close(); err != nil {
			return errors.Trace(err)
		}

		if len(ops) > 0 {
			err := st.db().RunTransaction(ops)
			if err != nil {
				return errors.Trace(err)
			}
		}
		return nil
	}))
	if err != nil {
		return errors.Trace(err)
	}

	var ops []txn.Op
	for controllerUUID, refCount := range refCountPerController {
		incRefOp, err := setExternalControllersRefOp(st, controllerUUID, refCount)
		if err != nil {
			return errors.Trace(err)
		}
		ops = append(ops, incRefOp...)
	}
	if len(ops) > 0 {
		err := st.db().RunTransaction(ops)
		if err != nil {
			return errors.Trace(err)
		}
	}

	if orphanedControllers.Size() > 0 {
		_, err := extControllers.Writeable().RemoveAll(bson.D{
			{"_id", bson.D{{"$in", orphanedControllers.Values()}}},
		})
		if err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

// setExternalControllersRefOp returns a txn.Op that sets the reference
// count for an external controller, incrementing any existing value as needed.
// These ref counts are controller wide.
func setExternalControllersRefOp(mb modelBackend, controllerUUID string, count int) ([]txn.Op, error) {
	refcounts, closer := mb.db().GetCollection(globalRefcountsC)
	defer closer()
	refCountKey := externalControllerRefCountKey(controllerUUID)
	existing, err := nsRefcounts.read(refcounts, refCountKey)
	if err != nil && !errors.IsNotFound(err) {
		return nil, errors.Trace(err)
	}
	if count == existing {
		return nil, nil
	}
	newCount := count - existing
	incRefOp, err := nsRefcounts.CreateOrIncRefOp(refcounts, refCountKey, newCount)
	return []txn.Op{incRefOp}, errors.Trace(err)
}

// RemoveInvalidCharmPlaceholders removes invalid charms that have invalid charm
// urls, that also have placeholder fields set.
func RemoveInvalidCharmPlaceholders(pool *StatePool) error {
	return errors.Trace(runForAllModelStates(pool, func(st *State) error {
		charms, cCloser := st.db().GetCollection(charmsC)
		defer cCloser()

		// Get all the charm placeholders.
		docs := make(map[string]string)

		iter := charms.Find(stillPlaceholder).Iter()
		var cDoc charmDoc
		for iter.Next(&cDoc) {
			docs[*cDoc.URL] = cDoc.DocID
		}

		if err := iter.Close(); err != nil {
			return errors.Trace(err)
		}

		if len(docs) == 0 {
			return nil
		}

		apps, aCloser := st.db().GetCollection(applicationsC)
		defer aCloser()

		var ops []txn.Op
		for charmURL, id := range docs {
			amount, err := apps.Find(bson.M{"charmurl": charmURL}).Count()
			if err != nil {
				continue
			}
			// There is an application reference, we should keep the
			// placeholder.
			if amount > 0 {
				continue
			}
			ops = append(ops, txn.Op{
				C:      charmsC,
				Id:     id,
				Remove: true,
			})
		}

		if len(ops) == 0 {
			return nil
		}

		return errors.Trace(st.db().RunTransaction(ops))
	}))
}

// SetContainerAddressOriginToMachine corrects a prior upgrade step that ran
// AddOriginToIPAddresses. It was incorrect to set "provider" as the source of
// container addresses, because we do not run the instance-poller for
// containers. The effect for VIPs added by Corosync/Pacemaker was to freeze
// such addresses to the machine, because they were never relinquished and in
// turn never deleted by the machine address updates.
func SetContainerAddressOriginToMachine(pool *StatePool) error {
	st, err := pool.SystemState()
	if err != nil {
		return errors.Trace(err)
	}
	coll, closer := st.db().GetRawCollection(ipAddressesC)
	defer closer()

	// Get all addresses assigned to container machines
	// that have a provider origin.
	iter := coll.Find(bson.D{
		{"machine-id", bson.D{{"$regex", `\/(lxd|kvm)\/`}}},
		{"origin", network.OriginProvider},
	}).Iter()

	type idDoc struct {
		DocID string `bson:"_id"`
	}

	var doc idDoc
	var ids []string
	for iter.Next(&doc) {
		ids = append(ids, doc.DocID)
	}
	if err := iter.Close(); err != nil {
		return errors.Trace(err)
	}

	var ops []txn.Op
	for _, id := range ids {
		ops = append(ops, txn.Op{
			C:      ipAddressesC,
			Id:     id,
			Update: bson.M{"$set": bson.M{"origin": network.OriginMachine}},
		})
	}

	if len(ops) == 0 {
		return nil
	}
	return st.runRawTransaction(ops)
}

// UpdateCharmOriginAfterSetSeries updates application's charm origin platform series
// if it doesn't match the application series.  E.G. after set-series is called.
func UpdateCharmOriginAfterSetSeries(pool *StatePool) error {
	return errors.Trace(runForAllModelStates(pool, func(st *State) error {
		col, closer := st.db().GetCollection(applicationsC)
		defer closer()

		var docs []applicationDoc
		if err := col.Find(nil).All(&docs); err != nil {
			return errors.Trace(err)
		}

		var ops []txn.Op
		for _, application := range docs {

			appSeries := application.Series
			if application.CharmOrigin == nil || application.CharmOrigin.Platform == nil {
				logger.Errorf("%s has no platform in the charm origin", application.Name)
				continue
			}
			if appSeries == application.CharmOrigin.Platform.Series {
				continue
			}
			ops = append(ops, txn.Op{
				C:      applicationsC,
				Id:     application.DocID,
				Assert: txn.DocExists,
				Update: bson.D{{"$set", bson.D{{
					"charm-origin.platform.series", appSeries,
				}}}},
			})
		}
		if len(ops) > 0 {
			return errors.Trace(st.db().RunTransaction(ops))
		}
		return nil
	}))
}

// UpdateOperationWithEnqueuingErrors updates operations with enqueuing errors to allow
// started actions to complete. See LP 1953077.
func UpdateOperationWithEnqueuingErrors(pool *StatePool) error {
	st, err := pool.SystemState()
	if err != nil {
		return errors.Trace(err)
	}
	opCol, opCloser := st.db().GetRawCollection(operationsC)
	defer opCloser()
	actionsCol, actionsCloser := st.db().GetRawCollection(actionsC)
	defer actionsCloser()

	// Get all operations with an error status and a fail message
	// to indicate FailOperation was used.
	iter := opCol.Find(bson.D{
		{"status", "error"},
		{"fail", bson.M{"$ne": ""}},
	}).Iter()

	var ops []txn.Op
	var doc operationDoc
	for iter.Next(&doc) {
		if doc.SpawnedTaskCount == doc.CompleteTaskCount || doc.Fail == "" {
			continue
		}
		modelUUID, opID, ok := splitDocID(doc.DocId)
		if !ok {
			_ = iter.Close()
			return errors.Errorf("bad data, remote entity _id %s", doc.DocId)
		}
		spawned, err := actionsCol.Find(bson.D{
			{"operation", opID},
			{"model-uuid", modelUUID},
		}).Count()
		if err != nil {
			logger.Errorf("error getting spawned task count from %q:", doc.DocId, err)
			continue
		}
		setValue := bson.D{
			{"spawned-task-count", spawned},
		}
		if spawned != 0 {
			setValue = append(setValue, bson.DocElem{"status", "running"})
		}
		ops = append(ops, txn.Op{
			C:      operationsC,
			Id:     doc.DocId,
			Assert: txn.DocExists,
			Update: bson.D{{
				"$set", setValue,
			}},
		})
	}
	if err := iter.Close(); err != nil {
		return errors.Trace(err)
	}
	if len(ops) == 0 {
		return nil
	}
	return st.runRawTransaction(ops)
}

// RemoveLocalCharmOriginChannels removes the charm-origin channel from all
// local charms, it cannot have even an empty risk. See LP1970608.
func RemoveLocalCharmOriginChannels(pool *StatePool) error {
	return errors.Trace(runForAllModelStates(pool, func(st *State) error {
		col, closer := st.db().GetCollection(applicationsC)
		defer closer()

		var docs []applicationDoc
		if err := col.Find(nil).All(&docs); err != nil {
			return errors.Trace(err)
		}

		var ops []txn.Op
		for _, doc := range docs {
			// It is expected that every application should have a charm URL.
			charmURL, err := charm.ParseURL(*doc.CharmURL)
			if err != nil {
				return errors.Annotatef(err, "parsing charm url")
			}

			if charmURL.Schema != "local" {
				continue
			}

			if doc.CharmOrigin == nil || doc.CharmOrigin.Channel == nil {
				continue
			}

			ops = append(ops, txn.Op{
				C:      applicationsC,
				Id:     doc.DocID,
				Assert: txn.DocExists,
				Update: bson.D{{"$unset", bson.D{{"charm-origin.channel", nil}}}},
			})
		}
		if len(ops) > 0 {
			return errors.Trace(st.db().RunTransaction(ops))
		}
		return nil
	}))
}

// FixCharmhubLastPolltime adds a non-zero last poll time to
// charmhub resource records. We don't know the exact time (it
// would have been sometime in the last 24 hours, so time.Now()
// will suffice.
func FixCharmhubLastPolltime(pool *StatePool) error {
	return errors.Trace(runForAllModelStates(pool, func(st *State) error {
		coll, closer := st.db().GetCollection(resourcesC)
		defer closer()

		query := bson.M{
			"_id": bson.M{"$regex": ".*" + resourcesCharmstoreIDSuffix + "$"},
		}
		iter := coll.Find(query).Select(bson.M{
			"_id":                        1,
			"timestamp-when-last-polled": 1,
		}).Iter()
		defer iter.Close()
		var ops []txn.Op
		var doc bson.M
		for iter.Next(&doc) {
			id, ok := doc["_id"]
			if !ok {
				return errors.New("no id found in resource doc")
			}
			t, ok := doc["timestamp-when-last-polled"].(time.Time)
			if ok && !t.IsZero() {
				continue
			}
			ops = append(ops, txn.Op{
				C:      resourcesC,
				Id:     id,
				Assert: txn.DocExists,
				Update: bson.D{{"$set", bson.D{{"timestamp-when-last-polled", st.nowToTheSecond()}}}},
			})
		}
		if err := iter.Close(); err != nil {
			return errors.Trace(err)
		}
		return st.runRawTransaction(ops)
	}))
}

// RemoveUseFloatingIPConfigFalse removes any model config key value pair:
// use-floating-ip=false. It is deprecated, default by false and causing
// much noise in logs.
func RemoveUseFloatingIPConfigFalse(pool *StatePool) error {
	st, err := pool.SystemState()
	if err != nil {
		return errors.Trace(err)
	}
	return errors.Trace(applyToAllModelSettings(st, func(doc *settingsDoc) (bool, error) {
		var changed bool
		value, ok := doc.Settings["use-floating-ip"]
		if ok && value != "" {
			if v, ok := value.(bool); ok && !v {
				changed = true
				delete(doc.Settings, "use-floating-ip")
			}
		}
		return changed, nil
	}))
}

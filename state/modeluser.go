// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"strings"
	"time"

	"github.com/juju/errors"
	"github.com/juju/utils/set"
	"github.com/juju/version"
	"gopkg.in/juju/names.v2"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/mongo/utils"
	"github.com/juju/juju/permission"
	"github.com/juju/juju/status"
)

// modelUserLastConnectionDoc is updated by the apiserver whenever the user
// connects over the API. This update is not done using mgo.txn so the values
// could well change underneath a normal transaction and as such, it should
// NEVER appear in any transaction asserts. It is really informational only as
// far as everyone except the api server is concerned.
type modelUserLastConnectionDoc struct {
	ID             string    `bson:"_id"`
	ModelUUID      string    `bson:"model-uuid"`
	UserName       string    `bson:"user"`
	LastConnection time.Time `bson:"last-connection"`
}

// setModelAccess changes the user's access permissions on the model.
func (st *State) setModelAccess(access permission.Access, userGlobalKey, modelUUID string) error {
	if err := permission.ValidateModelAccess(access); err != nil {
		return errors.Trace(err)
	}
	op := updatePermissionOp(modelKey(modelUUID), userGlobalKey, access)
	err := st.db().RunTransactionFor(modelUUID, []txn.Op{op})
	if err == txn.ErrAborted {
		return errors.NotFoundf("existing permissions")
	}
	return errors.Trace(err)
}

// LastModelConnection returns when this User last connected through the API
// in UTC. The resulting time will be nil if the user has never logged in.
func (m *Model) LastModelConnection(user names.UserTag) (time.Time, error) {
	lastConnections, closer := m.st.db().GetRawCollection(modelUserLastConnectionC)
	defer closer()

	username := user.Id()
	var lastConn modelUserLastConnectionDoc
	err := lastConnections.FindId(m.st.docID(username)).Select(bson.D{{"last-connection", 1}}).One(&lastConn)
	if err != nil {
		if err == mgo.ErrNotFound {
			err = errors.Wrap(err, NeverConnectedError(username))
		}
		return time.Time{}, errors.Trace(err)
	}

	return lastConn.LastConnection.UTC(), nil
}

// NeverConnectedError is used to indicate that a user has never connected to
// an model.
type NeverConnectedError string

// Error returns the error string for a user who has never connected to an
// model.
func (e NeverConnectedError) Error() string {
	return `never connected: "` + string(e) + `"`
}

// IsNeverConnectedError returns true if err is of type NeverConnectedError.
func IsNeverConnectedError(err error) bool {
	_, ok := errors.Cause(err).(NeverConnectedError)
	return ok
}

// UpdateLastModelConnection updates the last connection time of the model user.
func (m *Model) UpdateLastModelConnection(user names.UserTag) error {
	return m.updateLastModelConnection(user, m.st.nowToTheSecond())
}

func (m *Model) updateLastModelConnection(user names.UserTag, when time.Time) error {
	lastConnections, closer := m.st.db().GetCollection(modelUserLastConnectionC)
	defer closer()

	lastConnectionsW := lastConnections.Writeable()

	// Update the safe mode of the underlying session to not require
	// write majority, nor sync to disk.
	session := lastConnectionsW.Underlying().Database.Session
	session.SetSafe(&mgo.Safe{})

	lastConn := modelUserLastConnectionDoc{
		ID:             m.st.docID(strings.ToLower(user.Id())),
		ModelUUID:      m.UUID(),
		UserName:       user.Id(),
		LastConnection: when,
	}
	_, err := lastConnectionsW.UpsertId(lastConn.ID, lastConn)
	return errors.Trace(err)
}

// ModelUser a model userAccessDoc.
func (st *State) modelUser(modelUUID string, user names.UserTag) (userAccessDoc, error) {
	modelUser := userAccessDoc{}
	modelUsers, closer := st.db().GetCollectionFor(modelUUID, modelUsersC)
	defer closer()

	username := strings.ToLower(user.Id())
	err := modelUsers.FindId(username).One(&modelUser)
	if err == mgo.ErrNotFound {
		return userAccessDoc{}, errors.NotFoundf("model user %q", username)
	}
	if err != nil {
		return userAccessDoc{}, errors.Trace(err)
	}
	// DateCreated is inserted as UTC, but read out as local time. So we
	// convert it back to UTC here.
	modelUser.DateCreated = modelUser.DateCreated.UTC()
	return modelUser, nil
}

func createModelUserOps(modelUUID string, user, createdBy names.UserTag, displayName string, dateCreated time.Time, access permission.Access) []txn.Op {
	creatorname := createdBy.Id()
	doc := &userAccessDoc{
		ID:          userAccessID(user),
		ObjectUUID:  modelUUID,
		UserName:    user.Id(),
		DisplayName: displayName,
		CreatedBy:   creatorname,
		DateCreated: dateCreated,
	}

	ops := []txn.Op{
		createPermissionOp(modelKey(modelUUID), userGlobalKey(userAccessID(user)), access),
		{
			C:      modelUsersC,
			Id:     userAccessID(user),
			Assert: txn.DocMissing,
			Insert: doc,
		},
	}
	return ops
}

func removeModelUserOps(modelUUID string, user names.UserTag) []txn.Op {
	return []txn.Op{
		removePermissionOp(modelKey(modelUUID), userGlobalKey(userAccessID(user))),
		{
			C:      modelUsersC,
			Id:     userAccessID(user),
			Assert: txn.DocExists,
			Remove: true,
		}}
}

// removeModelUser removes a user from the database.
func (st *State) removeModelUser(user names.UserTag) error {
	ops := removeModelUserOps(st.ModelUUID(), user)
	err := st.db().RunTransaction(ops)
	if err == txn.ErrAborted {
		err = errors.NewNotFound(nil, fmt.Sprintf("model user %q does not exist", user.Id()))
	}
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}

type UserAccessInfo struct {
	permission.UserAccess
	LastConnection *time.Time
}

type ModelDetails struct {
	Name           string
	UUID           string
	Owner          string
	ControllerUUID string
	Life           Life

	CloudTag           string
	CloudRegion        string
	CloudCredentialTag string

	// SLA contains the information about the SLA for the model, if set.
	SLALevel string
	SLAOwner string

	// Needs Config()
	ProviderType  string
	DefaultSeries string
	AgentVersion  *version.Number

	// Needs Statuses collection
	Status status.StatusInfo

	// Needs ModelUser, ModelUserLastConnection, and Permissions collections
	Users map[string]UserAccessInfo

	// Need to add LastConnection information for each user

	// Machines contains information about the machines in the model.
	// This information is available to owners and users with write
	// access or greater.
	MachineCount int64
	CoreCount    int64
	// DO WE EVEN WANT THIS DATA HERE? The only thing we show in 'juju models' is the *count*
	/// Machines []ModelMachineInfo `json:"machines"`

	// Needs Migration collection
	Migration ModelMigration
	// Migration contains information about the latest failed or
	// currently-running migration. It'll be nil if there isn't one.
	/// Migration *ModelMigrationStatus `json:"migration,omitempty"`
	// //	type ModelMigrationStatus struct {
	// //	Status string     `json:"status"`
	// //	Start  *time.Time `json:"start"`
	// //	End    *time.Time `json:"end,omitempty"`
	// //}

}

// modelDetailProcessor provides the working space for extracting details for models that a user has access to.
type modelDetailProcessor struct {
	st          *State
	details     []ModelDetails
	indexByUUID map[string]int
	modelUUIDs  []string

	//invalidLocalUsers are usernames that show up as we're walking the database, but ultimately are considered deleted
	invalidLocalUsers set.Strings

	// incompleteUUIDs are ones that are missing some information, we should treat them as not being available
	// we wait to strip them out until we're done doing all the processing steps.
	incompleteUUIDs set.Strings
}

func newProcessorFromModelDocs(st *State, modelDocs []modelDoc) *modelDetailProcessor {
	p := &modelDetailProcessor{
		st: st,
	}
	p.details = make([]ModelDetails, len(modelDocs))
	p.indexByUUID = make(map[string]int, len(modelDocs))
	p.modelUUIDs = make([]string, len(modelDocs))
	for i, doc := range modelDocs {
		var cloudCred string
		if names.IsValidCloudCredential(doc.CloudCredential) {
			cloudCred = names.NewCloudCredentialTag(doc.CloudCredential).String()
		}
		p.details[i] = ModelDetails{
			Name:               doc.Name,
			UUID:               doc.UUID,
			Life:               doc.Life,
			Owner:              doc.Owner,
			ControllerUUID:     doc.ControllerUUID,
			SLALevel:           string(doc.SLA.Level),
			SLAOwner:           doc.SLA.Owner,
			CloudTag:           names.NewCloudTag(doc.Cloud).String(),
			CloudRegion:        doc.CloudRegion,
			CloudCredentialTag: cloudCred,
			Users:              make(map[string]UserAccessInfo),
		}
		p.indexByUUID[doc.UUID] = i
		p.modelUUIDs[i] = doc.UUID
	}
	return p
}

func (p *modelDetailProcessor) fillInFromConfig() error {
	// We use the raw settings because we are reading across model UUIDs
	rawSettings, closer := p.st.database.GetRawCollection(settingsC)
	defer closer()

	remaining := set.NewStrings(p.modelUUIDs...)
	settingIds := make([]string, len(p.modelUUIDs))
	for i, uuid := range p.modelUUIDs {
		settingIds[i] = uuid + ":" + modelGlobalKey
	}
	query := rawSettings.Find(bson.M{"_id": bson.M{"$in": settingIds}})
	var doc settingsDoc
	iter := query.Iter()
	for iter.Next(&doc) {
		idx, ok := p.indexByUUID[doc.ModelUUID]
		if !ok {
			// How could it return a doc that we don't have?
			continue
		}
		remaining.Remove(doc.ModelUUID)

		cfg, err := config.New(config.NoDefaults, doc.Settings)
		if err != nil {
			// err on one model should kill all the other ones?
			return errors.Trace(err)
		}
		detail := &(p.details[idx])
		detail.ProviderType = cfg.Type()
		detail.DefaultSeries = config.PreferredSeries(cfg)
		if agentVersion, exists := cfg.AgentVersion(); exists {
			detail.AgentVersion = &agentVersion
		}
	}
	if err := iter.Close(); err != nil {
		return errors.Trace(err)
	}
	if !remaining.IsEmpty() {
		// XXX: What error is appropriate? Do we need to care about models that its ok to be missing?
		return errors.Errorf("could not find settings/config for models: %v", remaining.SortedValues())
	}
	return nil
}

// removeExistingUsers takes a set of usernames, and removes any of them that are known-valid users.
// it leaves behind names that could not be validated
func (p *modelDetailProcessor) removeExistingUsers(names set.Strings) error {
	// usersC is a global collection, so we can access it from any state
	users, closer := p.st.db().GetCollection(usersC)
	defer closer()

	var doc struct {
		Id string `bson:"_id"`
	}
	query := users.Find(bson.M{"_id": bson.M{"$in": names.Values()}}).Select(bson.M{"_id": 1})
	query.Batch(100)
	iter := query.Iter()
	for iter.Next(&doc) {
		names.Remove(doc.Id)
	}
	if err := iter.Close(); err != nil {
		return errors.Trace(err)
	}
	return nil
}

func (p *modelDetailProcessor) fillInFromStatus() error {
	// We use the raw statuses because otherwise it filters by model-uuid
	rawStatus, closer := p.st.database.GetRawCollection(statusesC)
	defer closer()
	statusIds := make([]string, len(p.modelUUIDs))
	for i, uuid := range p.modelUUIDs {
		statusIds[i] = uuid + ":" + modelGlobalKey
	}
	// TODO: Track remaining and error if we're missing any
	query := rawStatus.Find(bson.M{"_id": bson.M{"$in": statusIds}})
	var doc statusDoc
	iter := query.Iter()
	for iter.Next(&doc) {
		idx, ok := p.indexByUUID[doc.ModelUUID]
		if !ok {
			// missing?
			continue
		}
		p.details[idx].Status = status.StatusInfo{
			Status:  doc.Status,
			Message: doc.StatusInfo,
			Data:    utils.UnescapeKeys(doc.StatusData),
			Since:   unixNanoToTime(doc.Updated),
		}
	}
	if err := iter.Close(); err != nil {
		return errors.Trace(err)
	}
	return nil
}

func (p *modelDetailProcessor) fillInPermissions(permissionIdToUserAccessDoc map[string]userAccessDoc, permissionIds []string) error {
	// permissionsC is a global collection, so can be accessed from any state
	perms, closer := p.st.db().GetCollection(permissionsC)
	defer closer()
	query := perms.Find(bson.M{"_id": bson.M{"$in": permissionIds}})
	iter := query.Iter()

	var doc permissionDoc
	for iter.Next(&doc) {
		userDoc, ok := permissionIdToUserAccessDoc[doc.ID]
		if !ok {
			continue
		}
		modelIdx, ok := p.indexByUUID[userDoc.ObjectUUID]
		if !ok {
			// ??
			continue
		}
		details := &p.details[modelIdx]
		username := strings.ToLower(userDoc.UserName)
		// mu, err := NewModelUserAccess(m.st, doc)
		details.Users[username] = UserAccessInfo{
			// newUserAccess?
			UserAccess: permission.UserAccess{
				UserID:      userDoc.ID,
				UserTag:     names.NewUserTag(userDoc.UserName),
				Object:      names.NewModelTag(userDoc.ObjectUUID),
				Access:      stringToAccess(doc.Access),
				CreatedBy:   names.NewUserTag(userDoc.CreatedBy),
				DateCreated: userDoc.DateCreated.UTC(),
				DisplayName: userDoc.DisplayName,
				UserName:    userDoc.UserName,
			},
		}
	}
	if err := iter.Close(); err != nil {
		return errors.Trace(err)
	}
	return nil
}

func (p *modelDetailProcessor) fillInLastConnection(lastAccessIds []string) error {
	lastConnections, closer2 := p.st.db().GetRawCollection(modelUserLastConnectionC)
	defer closer2()
	query := lastConnections.Find(bson.M{"_id": bson.M{"$in": lastAccessIds}})
	query.Batch(100)
	iter := query.Iter()
	var connInfo modelUserLastConnectionDoc
	for iter.Next(&connInfo) {
		idx, ok := p.indexByUUID[connInfo.ModelUUID]
		if !ok {
			continue
		}
		details := &p.details[idx]
		username := strings.ToLower(connInfo.UserName)
		t := connInfo.LastConnection
		userInfo := details.Users[username]
		userInfo.LastConnection = &t
		details.Users[username] = userInfo
	}
	if err := iter.Close(); err != nil {
		return errors.Trace(err)
	}
	return nil
}

func (p *modelDetailProcessor) fillInMachines() error {
	// TODO: (jam) 2017-11-18 we should have filled in the authorization information already. So just use that
	// information to know what machine information we should be reporting.
	// Then again, if we are just returning summary information instead of the details, do we care about exposing it to
	// readonly users?
	// We'd want to do this to collect the model uuids that we will collect machine information for.
	//// canSeeMachines := authorizedOwner
	//// if !canSeeMachines {
	//// 	if canSeeMachines, err = m.hasWriteAccess(tag); err != nil {
	//// 		return params.ModelInfo{}, errors.Trace(err)
	//// 	}
	//// }
	//// if canSeeMachines {
	//// 	if info.Machines, err = common.ModelMachineInfo(st); shouldErr(err) {
	//// 		return params.ModelInfo{}, err
	//// 	}
	//// }
	machines, closer := p.st.db().GetRawCollection(machinesC)
	defer closer()
	query := machines.Find(bson.M{
		"model-uuid": bson.M{"$in": p.modelUUIDs},
		"life":       Alive,
	})
	query.Select(bson.M{"life": 1, "model-uuid": 1, "_id": 1, "machineid": 1})
	iter := query.Iter()
	var doc machineDoc
	machineIds := make([]string, 0)
	for iter.Next(&doc) {
		if doc.Life != Alive {
			continue
		}
		idx, ok := p.indexByUUID[doc.ModelUUID]
		if !ok {
			continue
		}
		// There was a lot of data that was collected from things like Machine.Status.
		// However, if we're just aggregating the counts, we don't care about any of that.
		details := &p.details[idx]
		details.MachineCount++
		// TODO: Don't double count Containers
		machineIds = append(machineIds, doc.ModelUUID+":"+doc.Id)
	}
	if err := iter.Close(); err != nil {
		return errors.Trace(err)
	}
	instances, closer2 := p.st.db().GetRawCollection(instanceDataC)
	defer closer2()
	query = instances.Find(bson.M{"_id": bson.M{"$in": machineIds}})
	query.Select(bson.M{"cpucores": 1, "model-uuid": 1})
	iter = query.Iter()
	var instData instanceData
	for iter.Next(&instData) {
		idx, ok := p.indexByUUID[instData.ModelUUID]
		if !ok {
			continue
		}
		details := &p.details[idx]
		if instData.CpuCores != nil {
			details.CoreCount += int64(*instData.CpuCores)
		}
	}
	if err := iter.Close(); err != nil {
		return errors.Trace(err)
	}
	return nil
}

func (p *modelDetailProcessor) fillInMigration() error {
	// For now, we just potato the Migration information. Its a little unfortunate, but the expectation is that most
	// models won't have been migrated, and thus the table is mostly empty anyway.
	// It might be possible to do it differently with an aggregation and $first queries.
	// $first appears to have been available since Mongo 2.4.
	// Migrations is a global collection so can be accessed from any State
	migrations, closer := p.st.db().GetCollection(migrationsC)
	defer closer()
	pipe := migrations.Pipe([]bson.M{
		{"$match": bson.M{"model-uuid": bson.M{"$in": p.modelUUIDs}}},
		{"$sort": bson.M{"model-uuid": 1, "attempt": -1}},
		{"$group": bson.M{
			"_id":     "$model-uuid",
			"attempt": -1,
		},
		},
	})
	pipe.Batch(100)
	iter := pipe.Iter()
	modelMigDocs := make(map[string]modelMigDoc)
	docIds := make([]string, 0)
	var doc modelMigDoc
	for iter.Next(&doc) {
		if _, ok := p.indexByUUID[doc.ModelUUID]; !ok {
			continue
		}
		modelMigDocs[doc.Id] = doc
		docIds = append(docIds, doc.Id)
	}
	if err := iter.Close(); err != nil {
		return errors.Trace(err)
	}
	// Now look up the status documents and join them together
	migStatus, closer2 := p.st.db().GetCollection(migrationsStatusC)
	defer closer2()
	query := migStatus.Find(bson.M{"_id": bson.M{"$in": docIds}})
	query.Batch(100)
	iter = query.Iter()
	var statusDoc modelMigStatusDoc
	for iter.Next(&statusDoc) {
		doc, ok := modelMigDocs[statusDoc.Id]
		if !ok {
			continue
		}
		idx, ok := p.indexByUUID[doc.ModelUUID]
		if !ok {
			continue
		}
		details := &p.details[idx]
		// TODO (jam): Can we make modelMigration *not* accept a State object so that we know we won't potato more stuff in the future?
		details.Migration = &modelMigration{
			doc:       doc,
			statusDoc: statusDoc,
			st:        p.st,
		}
	}
	if err := iter.Close(); err != nil {
		return errors.Trace(err)
	}
	return nil
}

func (p *modelDetailProcessor) fillInFromModelUsers() error {
	// We use the raw settings because we are reading across model UUIDs
	rawModelUsers, closer := p.st.database.GetRawCollection(modelUsersC)
	defer closer()

	// TODO(jam): ensure that we have appropriate indexes so that users that aren't "admin" and only see a couple
	// models don't do a COLLSCAN on the table.
	query := rawModelUsers.Find(bson.M{"object-uuid": bson.M{"$in": p.modelUUIDs}})
	var doc userAccessDoc
	iter := query.Iter()
	permissionIdToAccessDoc := make(map[string]userAccessDoc)
	// does this need to be a set, or just a simple slice, we shouldn't have duplicates
	permissionIds := make([]string, 0)
	lastAccessIds := make([]string, 0)
	// These need to be checked if they have been deleted
	localUsers := set.NewStrings()
	for iter.Next(&doc) {
		username := strings.ToLower(doc.UserName)
		userTag := names.NewUserTag(username)
		if userTag.IsLocal() {
			localUsers.Add(username)
		}
		permId := permissionID(modelKey(doc.ObjectUUID), userGlobalKey(username))
		permissionIds = append(permissionIds, permId)
		permissionIdToAccessDoc[permId] = doc
		lastAccessIds = append(lastAccessIds, doc.ObjectUUID+":"+username)
	}
	if err := iter.Close(); err != nil {
		return errors.Trace(err)
	}
	// Now we check if any of these Users are actually deleted
	// TODO: wait to add this check for an actual test case covering it
	if err := p.removeExistingUsers(localUsers); err != nil {
		return errors.Trace(err)
	}
	if err := p.fillInPermissions(permissionIdToAccessDoc, permissionIds); err != nil {
		return errors.Trace(err)
	}
	if err := p.fillInLastConnection(lastAccessIds); err != nil {
		return errors.Trace(err)
	}
	if err := p.fillInMachines(); err != nil {
		return errors.Trace(err)
	}
	if err := p.fillInMigration(); err != nil {
		return errors.Trace(err)
	}
	return nil
}

func (st *State) ModelDetailsForUser(user names.UserTag) ([]ModelDetails, error) {
	modelQuery, closer, err := st.modelQueryForUser(user)
	if err != nil {
		return nil, errors.Trace(err)
	}
	defer closer()
	var modelDocs []modelDoc
	if err := modelQuery.All(&modelDocs); err != nil {
		return nil, errors.Trace(err)
	}
	p := newProcessorFromModelDocs(st, modelDocs)
	modelDocs = nil
	if err := p.fillInFromConfig(); err != nil {
		return nil, errors.Trace(err)
	}
	if err := p.fillInFromStatus(); err != nil {
		return nil, errors.Trace(err)
	}
	if err := p.fillInFromModelUsers(); err != nil {
		return nil, errors.Trace(err)
	}
	return p.details, nil
}

// modelsForUser gives you the information about all models that a user has access to.
// This includes the name and UUID, as well as the last time the user connected to that model.
func (st *State) modelQueryForUser(user names.UserTag) (mongo.Query, SessionCloser, error) {
	access, err := st.UserAccess(user, st.controllerTag)
	if err != nil && !errors.IsNotFound(err) {
		return nil, nil, errors.Trace(err)
	}
	var modelQuery mongo.Query
	models, closer := st.db().GetCollection(modelsC)
	if access.Access == permission.SuperuserAccess {
		// Fast path, we just return all the models that aren't Importing
		modelQuery = models.Find(bson.M{"migration-mode": bson.M{"$ne": MigrationModeImporting}})
	} else {
		// Start by looking up model uuids that the user has access to, and then load only the records that are
		// included in that set
		var modelUUID struct {
			UUID string `bson:"object-uuid"`
		}
		modelUsers, userCloser := st.db().GetRawCollection(modelUsersC)
		defer userCloser()
		query := modelUsers.Find(bson.D{{"user", user.Id()}})
		query.Select(bson.M{"object-uuid": 1, "_id": 0})
		query.Batch(100)
		iter := query.Iter()
		var modelUUIDs []string
		for iter.Next(&modelUUID) {
			modelUUIDs = append(modelUUIDs, modelUUID.UUID)
		}
		if err := iter.Close(); err != nil {
			closer()
			return nil, nil, errors.Trace(err)
		}
		modelQuery = models.Find(bson.M{
			"_id":            bson.M{"$in": modelUUIDs},
			"migration-mode": bson.M{"$ne": MigrationModeImporting},
		})
	}
	modelQuery.Sort("name", "owner")
	return modelQuery, closer, nil
}

type ModelAccessInfo struct {
	Name           string `bson:"name"`
	UUID           string `bson:"_id"`
	Owner          string `bson:"owner"`
	LastConnection time.Time
}

// ModelSummariesForUser gives you the information about all models that a user has access to.
// This includes the name and UUID, as well as the last time the user connected to that model.
func (st *State) ModelSummariesForUser(user names.UserTag) ([]ModelAccessInfo, error) {
	modelQuery, closer1, err := st.modelQueryForUser(user)
	if err != nil {
		return nil, errors.Trace(err)
	}
	defer closer1()
	modelQuery.Select(bson.M{"_id": 1, "name": 1, "owner": 1})
	var accessInfo []ModelAccessInfo
	if err := modelQuery.All(&accessInfo); err != nil {
		return nil, errors.Trace(err)
	}
	// Now we need to find the last-connection time for each model for this user
	username := user.Id()
	connDocIds := make([]string, len(accessInfo))
	for i, acc := range accessInfo {
		connDocIds[i] = acc.UUID + ":" + username
	}
	lastConnections, closer2 := st.db().GetRawCollection(modelUserLastConnectionC)
	defer closer2()
	query := lastConnections.Find(bson.M{"_id": bson.M{"$in": connDocIds}})
	query.Select(bson.M{"last-connection": 1, "_id": 0, "model-uuid": 1})
	query.Batch(100)
	iter := query.Iter()
	lastConns := make(map[string]time.Time, len(connDocIds))
	var connInfo modelUserLastConnectionDoc
	for iter.Next(&connInfo) {
		lastConns[connInfo.ModelUUID] = connInfo.LastConnection
	}
	if err := iter.Close(); err != nil {
		return nil, errors.Trace(err)
	}
	for i := range accessInfo {
		uuid := accessInfo[i].UUID
		accessInfo[i].LastConnection = lastConns[uuid]
	}
	return accessInfo, nil
}

// ModelUUIDsForUser returns a list of models that the user is able to
// access.
// Results are sorted by (name, owner).
func (st *State) ModelUUIDsForUser(user names.UserTag) ([]string, error) {
	// Consider the controller permissions overriding Model permission, for
	// this case the only relevant one is superuser.
	// The mgo query below wont work for superuser case because it needs at
	// least one model user per model.
	access, err := st.UserAccess(user, st.controllerTag)
	if err != nil && !errors.IsNotFound(err) {
		return nil, errors.Trace(err)
	}

	var modelUUIDs []string
	if access.Access == permission.SuperuserAccess {
		var err error
		modelUUIDs, err = st.AllModelUUIDs()
		if err != nil {
			return nil, errors.Trace(err)
		}
	} else {
		// Since there are no groups at this stage, the simplest way to get all
		// the models that a particular user can see is to look through the
		// model user collection. A raw collection is required to support
		// queries across multiple models.
		modelUsers, userCloser := st.db().GetRawCollection(modelUsersC)
		defer userCloser()

		var userSlice []userAccessDoc
		err := modelUsers.Find(bson.D{{"user", user.Id()}}).Select(bson.D{{"object-uuid", 1}, {"_id", 1}}).All(&userSlice)
		if err != nil {
			return nil, err
		}
		for _, doc := range userSlice {
			modelUUIDs = append(modelUUIDs, doc.ObjectUUID)
		}
	}

	modelsColl, close := st.db().GetCollection(modelsC)
	defer close()
	query := modelsColl.Find(bson.M{
		"_id":            bson.M{"$in": modelUUIDs},
		"migration-mode": bson.M{"$ne": MigrationModeImporting},
	}).Sort("name", "owner").Select(bson.M{"_id": 1})

	var docs []bson.M
	err = query.All(&docs)
	if err != nil {
		return nil, errors.Trace(err)
	}
	out := make([]string, len(docs))
	for i, doc := range docs {
		out[i] = doc["_id"].(string)
	}
	return out, nil
}

// IsControllerAdmin returns true if the user specified has Super User Access.
func (st *State) IsControllerAdmin(user names.UserTag) (bool, error) {
	model, err := st.Model()
	if err != nil {
		return false, errors.Trace(err)
	}
	ua, err := st.UserAccess(user, model.ControllerTag())
	if errors.IsNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, errors.Trace(err)
	}
	return ua.Access == permission.SuperuserAccess, nil
}

func (st *State) isControllerOrModelAdmin(user names.UserTag) (bool, error) {
	isAdmin, err := st.IsControllerAdmin(user)
	if err != nil {
		return false, errors.Trace(err)
	}
	if isAdmin {
		return true, nil
	}
	ua, err := st.UserAccess(user, names.NewModelTag(st.modelUUID()))
	if errors.IsNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, errors.Trace(err)
	}
	return ua.Access == permission.AdminAccess, nil
}

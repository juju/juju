// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"strings"
	"time"

	"github.com/juju/errors"
	"github.com/juju/utils/set"
	"github.com/juju/version"
	"gopkg.in/juju/names.v2"
	"gopkg.in/mgo.v2/bson"

	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/mongo/utils"
	"github.com/juju/juju/permission"
	"github.com/juju/juju/status"
)

// UserAccessInfo contains just the information about a single user's access to a model and when they last connected.
type UserAccessInfo struct {
	permission.UserAccess
	LastConnection *time.Time
}

// MachineModelInfo contains the summary information about a machine for a given model.
type MachineModelInfo struct {
	Id         string
	Hardware   *instance.HardwareCharacteristics
	InstanceId string
	Status     string
}

// ModelDetails describe interesting information for a given model. This is meant to match the values that a user wants
// to see as part of either show-model or list-models.
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
	// This information will only be filled out if includeUsers is true and the user has at least Admin (write?) access
	// Otherwise only this user's information will be included.
	Users map[string]UserAccessInfo

	// Machines contains information about the machines in the model.
	// This information is available to owners and users with write
	// access or greater.
	// The information will also only be filled out if includeMachineDetails is true
	Machines     map[string]MachineModelInfo
	MachineCount int64
	CoreCount    int64

	// Needs Migration collection
	Migration ModelMigration
}

// modelDetailProcessor provides the working space for extracting details for models that a user has access to.
type modelDetailProcessor struct {
	st              *State
	details         []ModelDetails
	user            names.UserTag
	isSuperuser     bool
	indexByUUID     map[string]int
	modelUUIDs      []string
	writeModelUUIDs []string // models that we have admin access to

	//invalidLocalUsers are usernames that show up as we're walking the database, but ultimately are considered deleted
	invalidLocalUsers set.Strings

	// incompleteUUIDs are ones that are missing some information, we should treat them as not being available
	// we wait to strip them out until we're done doing all the processing steps.
	incompleteUUIDs set.Strings
}

func newProcessorFromModelDocs(st *State, modelDocs []modelDoc, user names.UserTag, isSuperuser bool) *modelDetailProcessor {
	p := &modelDetailProcessor{
		st:          st,
		user:        user,
		isSuperuser: isSuperuser,
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
			Machines:           make(map[string]MachineModelInfo),
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

func (p *modelDetailProcessor) fillInMachineSummary() error {
	machines, closer := p.st.db().GetRawCollection(machinesC)
	defer closer()
	query := machines.Find(bson.M{
		"model-uuid": bson.M{"$in": p.writeModelUUIDs},
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

func (p *modelDetailProcessor) fillInMachineDetails() error {
	// machinedocs
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
	statusIds := make([]string, 0)
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
		machineIds = append(machineIds, doc.DocID)
		statusIds = append(statusIds, doc.ModelUUID+":"+machineGlobalKey(doc.Id))
	}
	instances, closer2 := p.st.db().GetRawCollection(instanceDataC)
	defer closer2()
	query = instances.Find(bson.M{"_id": bson.M{"$in": machineIds}})
	//query.Select(bson.M{"cpucores": 1, "model-uuid": 1})
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
		details.Machines[instData.MachineId] = MachineModelInfo{
			Id:       instData.MachineId,
			Hardware: hardwareCharacteristics(instData),
		}
	}
	if err := iter.Close(); err != nil {
		return errors.Trace(err)
	}
	statuses, closer3 := p.st.db().GetRawCollection(statusesC)
	defer closer3()
	query = statuses.Find(bson.M{"_id": bson.M{"$in": statusIds}})
	query.Select(bson.M{"model-uuid": 1, "status": 1, "_id": 1})
	iter = query.Iter()
	var stDoc struct {
		// We can't use statusDoc because it doesn't have a field for _id
		Id        string `bson:"_id"`
		ModelUUID string `bson:"model-uuid"`
		Status    string `bson:"status"`
	}
	for iter.Next(&stDoc) {
		idx, ok := p.indexByUUID[stDoc.ModelUUID]
		if !ok {
			continue
		}
		details := &p.details[idx]
		// This is taken as "<model-uuid>:m#<machine-id>"
		prefixLen := len(stDoc.ModelUUID) + 3
		if len(stDoc.Id) > prefixLen {
			machineId := stDoc.Id[prefixLen:]
			mInfo := details.Machines[machineId]
			mInfo.Status = stDoc.Status
			details.Machines[machineId] = mInfo
			// apiserver/common.MachineStatus needs access to each individual State object to ask about AgentPresence
			// *sigh*.
			// that would allow us to override machine status with "Down" when appropriate
		} else {
			// Invalid machineId for status document
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
			"_id":   "$model-uuid",
			"docid": bson.M{"$first": "$_id"},
			// TODO: Do we need all of these, do we care about anything but doc _id?
			"attempt":           bson.M{"$first": "$attempt"},
			"initiated-by":      bson.M{"$first": "$initiated-by"},
			"target-controller": bson.M{"$first": "$target-controller"},
			"target-addrs":      bson.M{"$first": "$target-addrs"},
			"target-cacert":     bson.M{"$first": "$target-cacert"},
			"target-entity":     bson.M{"$first": "$target-entity"},
		}},
		// We grouped on model-uuid, but need to project back to normal fields
		{"$project": bson.M{
			"_id":               "$docid",
			"model-uuid":        "$_id",
			"attempt":           1,
			"initiated-by":      1,
			"target-controller": 1,
			"target-addrs":      1,
			"target-cacert":     1,
			"target-entity":     1,
		}},
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

// fillInJustUser fills in the Access rights for this user on every model (but not other users).
// We will use this information later to determine whether it is reasonable to include the information from other models.
func (p *modelDetailProcessor) fillInJustUser() error {
	if p.isSuperuser {
		// we skip this check, because we're the superuser, so we'll fill in everything anyway.
		return nil
	}
	rawModelUsers, closer := p.st.database.GetRawCollection(modelUsersC)
	defer closer()

	// TODO(jam): ensure that we have appropriate indexes so that users that aren't "admin" and only see a couple
	// models don't do a COLLSCAN on the table.
	username := strings.ToLower(p.user.Name())
	var permissionIds []string
	permIdToUserDoc := make(map[string]userAccessDoc)
	// TODO: Do we have to read the user access docs? We know the user and the model already, but if we want any details
	// for this user (DisplayName), then we need to read it.
	query := rawModelUsers.Find(bson.M{"object-uuid": bson.M{"$in": p.modelUUIDs}, "user": p.user.Name()})
	var doc userAccessDoc
	iter := query.Iter()
	for iter.Next(&doc) {
		permId := permissionID(modelKey(doc.ObjectUUID), userGlobalKey(username))
		permissionIds = append(permissionIds, permId)
		permIdToUserDoc[permId] = doc
	}
	if err := iter.Close(); err != nil {
		return errors.Trace(err)
	}
	if err := p.fillInPermissions(permIdToUserDoc, permissionIds); err != nil {
		return errors.Trace(err)
	}
	return nil
}

func (p *modelDetailProcessor) computeAdminModels() {
	// After calling fillInJustUser we can check for what models this user has Admin access to
	var writeModelUUIDs []string
	if p.isSuperuser {
		writeModelUUIDs = p.modelUUIDs
	} else {
		username := strings.ToLower(p.user.Name())
		for _, detail := range p.details {
			userInfo, ok := detail.Users[username]
			if ok && userInfo.Access.EqualOrGreaterModelAccessThan(permission.WriteAccess) {
				// See the Machines listing happens at Write access, and its a bit annoying to have Machines
				// at Write but Users at Admin, so we'll just set both at Write
				writeModelUUIDs = append(writeModelUUIDs, detail.UUID)
			}
		}
	}
	p.writeModelUUIDs = writeModelUUIDs
}

func (p *modelDetailProcessor) fillInLastAccess() error {
	// We fill in the last access only for the requesting user.
	lastAccessIds := make([]string, len(p.modelUUIDs))
	suffix := ":" + strings.ToLower(p.user.Name())
	for i, modelUUID := range p.modelUUIDs {
		lastAccessIds[i] = modelUUID + suffix
	}
	if err := p.fillInLastConnection(lastAccessIds); err != nil {
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
	query := rawModelUsers.Find(bson.M{"object-uuid": bson.M{"$in": p.writeModelUUIDs}})
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
	return nil
}

// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"strings"
	"time"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/version"
	"gopkg.in/juju/names.v2"
	"gopkg.in/mgo.v2/bson"

	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/mongo"
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

// ModelSummary describe interesting information for a given model. This is meant to match the values that a user wants
// to see as part of either show-model or list-models.
type ModelSummary struct {
	Name           string
	UUID           string
	Type           ModelType
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

	// Access is the access level the supplied user has on this model
	Access permission.Access
	// UserLastConnection is the last time this user has accessed this model
	UserLastConnection *time.Time

	MachineCount int64
	CoreCount    int64

	// Needs Migration collection
	// Do we need all the Migration fields?
	// Migration needs to be a pointer as we may not always have one.
	Migration ModelMigration
}

// modelSummaryProcessor provides the working space for extracting details for models that a user has access to.
type modelSummaryProcessor struct {
	st          *State
	summaries   []ModelSummary
	user        names.UserTag
	isSuperuser bool
	indexByUUID map[string]int
	modelUUIDs  []string

	//invalidLocalUsers are usernames that show up as we're walking the database, but ultimately are considered deleted
	invalidLocalUsers set.Strings

	// incompleteUUIDs are ones that are missing some information, we should treat them as not being available
	// we wait to strip them out until we're done doing all the processing steps.
	incompleteUUIDs set.Strings
}

func newProcessorFromModelDocs(st *State, modelDocs []modelDoc, user names.UserTag, isSuperuser bool) *modelSummaryProcessor {
	p := &modelSummaryProcessor{
		st:          st,
		user:        user,
		isSuperuser: isSuperuser,
	}
	p.summaries = make([]ModelSummary, len(modelDocs))
	p.indexByUUID = make(map[string]int, len(modelDocs))
	p.modelUUIDs = make([]string, len(modelDocs))
	for i, doc := range modelDocs {
		var cloudCred string
		if names.IsValidCloudCredential(doc.CloudCredential) {
			cloudCred = names.NewCloudCredentialTag(doc.CloudCredential).String()
		}
		p.summaries[i] = ModelSummary{
			Name:               doc.Name,
			UUID:               doc.UUID,
			Type:               doc.Type,
			Life:               doc.Life,
			Owner:              doc.Owner,
			ControllerUUID:     doc.ControllerUUID,
			SLALevel:           string(doc.SLA.Level),
			SLAOwner:           doc.SLA.Owner,
			CloudTag:           names.NewCloudTag(doc.Cloud).String(),
			CloudRegion:        doc.CloudRegion,
			CloudCredentialTag: cloudCred,
			/// Users:              make(map[string]UserAccessInfo),
			/// Machines:           make(map[string]MachineModelInfo),
		}
		p.indexByUUID[doc.UUID] = i
		p.modelUUIDs[i] = doc.UUID
	}
	return p
}

func (p *modelSummaryProcessor) fillInFromConfig() error {
	// We use the raw settings because we are reading across model UUIDs
	rawSettings, closer := p.st.database.GetRawCollection(settingsC)
	defer closer()

	settingIds := make([]string, len(p.modelUUIDs))
	for i, uuid := range p.modelUUIDs {
		settingIds[i] = uuid + ":" + modelGlobalKey
	}
	query := rawSettings.Find(bson.M{"_id": bson.M{"$in": settingIds}})
	var doc settingsDoc
	iter := query.Iter()
	defer iter.Close()
	for iter.Next(&doc) {
		idx, ok := p.indexByUUID[doc.ModelUUID]
		if !ok {
			// How could it return a doc that we don't have?
			continue
		}

		cfg, err := config.New(config.NoDefaults, doc.Settings)
		if err != nil {
			// err on one model should kill all the other ones?
			return errors.Trace(err)
		}
		detail := &(p.summaries[idx])
		detail.ProviderType = cfg.Type()
		detail.DefaultSeries = config.PreferredSeries(cfg)
		if agentVersion, exists := cfg.AgentVersion(); exists {
			detail.AgentVersion = &agentVersion
		}
	}
	if err := iter.Close(); err != nil {
		return errors.Trace(err)
	}
	return nil
}

func (p *modelSummaryProcessor) fillInFromStatus() error {
	// We use the raw statuses because otherwise it filters by model-uuid
	rawStatus, closer := p.st.database.GetRawCollection(statusesC)
	defer closer()
	statusIds := make([]string, len(p.modelUUIDs))
	for i, uuid := range p.modelUUIDs {
		statusIds[i] = uuid + ":" + modelGlobalKey
	}
	// TODO(jam): 2017-11-27 Track remaining and error if we're missing any
	query := rawStatus.Find(bson.M{"_id": bson.M{"$in": statusIds}})
	var doc statusDoc
	iter := query.Iter()
	defer iter.Close()
	for iter.Next(&doc) {
		idx, ok := p.indexByUUID[doc.ModelUUID]
		if !ok {
			// missing?
			continue
		}
		p.summaries[idx].Status = status.StatusInfo{
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

func (p *modelSummaryProcessor) fillInPermissions(permissionIds []string) error {
	// permissionsC is a global collection, so can be accessed from any state
	perms, closer := p.st.db().GetCollection(permissionsC)
	defer closer()
	query := perms.Find(bson.M{"_id": bson.M{"$in": permissionIds}})
	iter := query.Iter()
	defer iter.Close()

	var doc permissionDoc
	for iter.Next(&doc) {
		var modelUUID string
		if strings.HasPrefix(doc.ObjectGlobalKey, modelGlobalKey+"#") {
			modelUUID = doc.ObjectGlobalKey[2:]
		} else {
			// Invalid ObjectGlobalKey
			continue
		}
		modelIdx, ok := p.indexByUUID[modelUUID]
		if !ok {
			// How did we get a document that isn't in our list of documents?
			// TODO(jam) 2017-11-27, probably should be treated at least as a logged warning
			continue
		}
		details := &p.summaries[modelIdx]
		access := permission.Access(doc.Access)
		if err := access.Validate(); err == nil {
			details.Access = access
		}
	}
	if err := iter.Close(); err != nil {
		return errors.Trace(err)
	}
	return nil
}

func (p *modelSummaryProcessor) fillInMachineSummary() error {
	machines, closer := p.st.db().GetRawCollection(machinesC)
	defer closer()
	query := machines.Find(bson.M{
		"model-uuid": bson.M{"$in": p.modelUUIDs},
		"life":       Alive,
	})
	query.Select(bson.M{"life": 1, "model-uuid": 1, "_id": 1, "machineid": 1})
	iter := query.Iter()
	defer iter.Close()
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
		details := &p.summaries[idx]
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
	defer iter.Close()
	var instData instanceData
	for iter.Next(&instData) {
		idx, ok := p.indexByUUID[instData.ModelUUID]
		if !ok {
			continue
		}
		details := &p.summaries[idx]
		if instData.CpuCores != nil {
			details.CoreCount += int64(*instData.CpuCores)
		}
	}
	if err := iter.Close(); err != nil {
		return errors.Trace(err)
	}
	return nil
}

func (p *modelSummaryProcessor) fillInMigration() error {
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
			// TODO(jam): 2017-11-27 Do we need all of these, do we care about anything but doc _id?
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
	var iter mongo.Iterator = pipe.Iter()
	defer iter.Close()
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
	defer iter.Close()
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
		details := &p.summaries[idx]
		// TODO(jam): 2017-11-27 Can we make modelMigration *not* accept a State object so that we know we won't potato
		// more stuff in the future?
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
func (p *modelSummaryProcessor) fillInJustUser() error {
	// Note: Even for Superuser we track the individual Access for each model.
	// TODO(jam): 2017-11-27 ensure that we have appropriate indexes so that users that aren't "admin" and only see a couple
	// models don't do a COLLSCAN on the table.
	username := strings.ToLower(p.user.Name())
	var permissionIds []string
	for _, modelUUID := range p.modelUUIDs {
		permId := permissionID(modelKey(modelUUID), userGlobalKey(username))
		permissionIds = append(permissionIds, permId)
	}
	if err := p.fillInPermissions(permissionIds); err != nil {
		return errors.Trace(err)
	}
	return nil
}

func (p *modelSummaryProcessor) fillInLastAccess() error {
	// We fill in the last access only for the requesting user.
	lastAccessIds := make([]string, len(p.modelUUIDs))
	suffix := ":" + strings.ToLower(p.user.Name())
	for i, modelUUID := range p.modelUUIDs {
		lastAccessIds[i] = modelUUID + suffix
	}
	lastConnections, closer := p.st.db().GetRawCollection(modelUserLastConnectionC)
	defer closer()
	query := lastConnections.Find(bson.M{"_id": bson.M{"$in": lastAccessIds}})
	query.Select(bson.M{"_id": 1, "model-uuid": 1, "last-connection": 1})
	query.Batch(100)
	iter := query.Iter()
	defer iter.Close()
	var connInfo modelUserLastConnectionDoc
	for iter.Next(&connInfo) {
		idx, ok := p.indexByUUID[connInfo.ModelUUID]
		if !ok {
			continue
		}
		details := &p.summaries[idx]
		t := connInfo.LastConnection
		details.UserLastConnection = &t
	}
	if err := iter.Close(); err != nil {
		return errors.Trace(err)
	}
	// Note: We don't care if there are lastAccessIds that are not found, because its possible the user never
	// actually connected to a model they were given access to.
	return nil
}

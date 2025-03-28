// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/juju/errors"
	"github.com/juju/mgo/v3"
	"github.com/juju/mgo/v3/bson"
	"github.com/juju/mgo/v3/txn"
	"github.com/juju/names/v5"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/core/migration"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/mongo"
)

// This file contains functionality for managing the state documents
// used by Juju to track model migrations.

// ModelMigration represents the state of an migration attempt for a
// model.
type ModelMigration interface {
	// Id returns a unique identifier for the model migration.
	Id() string

	// ModelUUID returns the UUID for the model being migrated.
	ModelUUID() string

	// Attempt returns the migration attempt identifier. This
	// increments for each migration attempt for the model.
	Attempt() int

	// StartTime returns the time when the migration was started.
	StartTime() time.Time

	// SuccessTime returns the time when the migration reached
	// SUCCESS.
	SuccessTime() time.Time

	// EndTime returns the time when the migration reached DONE or
	// REAPFAILED.
	EndTime() time.Time

	// Phase returns the migration's phase.
	Phase() (migration.Phase, error)

	// PhaseChangedTime returns the time when the migration's phase
	// last changed.
	PhaseChangedTime() time.Time

	// StatusMessage returns human readable text about the current
	// progress of the migration.
	StatusMessage() string

	// InitiatedBy returns username the initiated the migration.
	InitiatedBy() string

	// TargetInfo returns the details required to connect to the
	// migration's target controller.
	TargetInfo() (*migration.TargetInfo, error)

	// SetPhase sets the phase of the migration. An error will be
	// returned if the new phase does not follow the current phase or
	// if the migration is no longer active.
	SetPhase(nextPhase migration.Phase) error

	// SetStatusMessage sets some human readable text about the
	// current progress of the migration.
	SetStatusMessage(text string) error

	// SubmitMinionReport records a report from a migration minion
	// worker about the success or failure to complete its actions for
	// a given migration phase.
	SubmitMinionReport(tag names.Tag, phase migration.Phase, success bool) error

	// MinionReports returns details of the minions that have reported
	// success or failure for the current migration phase, as well as
	// those which are yet to report.
	MinionReports() (*MinionReports, error)

	// WatchMinionReports returns a notify watcher which triggers when
	// a migration minion has reported back about the success or failure
	// of its actions for the current migration phase.
	WatchMinionReports() (NotifyWatcher, error)

	// Refresh updates the contents of the ModelMigration from the
	// underlying state.
	Refresh() error

	// ModelUserAccess returns the type of access that the given tag had to
	// the model prior to it being migrated.
	ModelUserAccess(names.Tag) permission.Access
}

// MinionReports indicates the sets of agents whose migration minion
// workers have completed the current migration phase, have failed to
// complete the current migration phase, or are yet to report
// regarding the current migration phase.
type MinionReports struct {
	Succeeded []names.Tag
	Failed    []names.Tag
	Unknown   []names.Tag
}

// modelMigration is an implementation of ModelMigration.
type modelMigration struct {
	st               *State
	doc              modelMigDoc
	statusDoc        modelMigStatusDoc
	statusMessageDoc modelMigStatusMessageDoc
}

// modelMigDoc holds parameters of a migration attempt for a
// model. These are written into migrationsC.
type modelMigDoc struct {
	// Id holds migration document key. It has the format
	// "uuid:sequence".
	Id string `bson:"_id"`

	// The UUID of the model being migrated.
	ModelUUID string `bson:"model-uuid"`

	// The attempt number of the model migration for this model.
	Attempt int `bson:"attempt"`

	// InitiatedBy holds the username of the user that triggered the
	// migration. It should be in "user@domain" format.
	InitiatedBy string `bson:"initiated-by"`

	// TargetController holds the UUID of the target controller.
	TargetController string `bson:"target-controller"`

	// An optional alias for the controller the model got migrated to.
	TargetControllerAlias string `bson:"target-controller-alias"`

	// TargetAddrs holds the host:port values for the target API
	// server.
	TargetAddrs []string `bson:"target-addrs"`

	// TargetCACert holds the certificate to validate the target API
	// server's TLS certificate.
	TargetCACert string `bson:"target-cacert"`

	// TargetAuthTag holds a string representation of the tag to
	// authenticate to the target controller with.
	TargetAuthTag string `bson:"target-entity"`

	// TargetPassword holds the password to use with TargetAuthTag
	// when authenticating.
	TargetPassword string `bson:"target-password,omitempty"`

	// TargetMacaroons holds the macaroons to use with TargetAuthTag
	// when authenticating.
	TargetMacaroons string `bson:"target-macaroons,omitempty"`

	// The list of users and their access-level to the model being migrated.
	ModelUsers []modelMigUserDoc `bson:"model-users,omitempty"`
}

type modelMigUserDoc struct {
	UserID string            `bson:"user_id"`
	Access permission.Access `bson:"access"`
}

// modelMigStatusDoc tracks the progress of a migration attempt for a
// model. These are written into migrationsStatusC.
//
// There is exactly one document in migrationsStatusC for each
// document in migrationsC. Separating them allows for watching
// for new model migrations without being woken up for each model
// migration status change.
type modelMigStatusDoc struct {
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
}

type modelMigStatusMessageDoc struct {
	// These are the same as the ids as migrationsC.
	// "uuid:sequence".
	Id string `bson:"_id"`

	// StatusMessage holds a human readable message about the
	// progress of the migration.
	StatusMessage string `bson:"status-message"`
}

type modelMigMinionSyncDoc struct {
	Id          string `bson:"_id"`
	MigrationId string `bson:"migration-id"`
	Phase       string `bson:"phase"`
	EntityKey   string `bson:"entity-key"`
	Time        int64  `bson:"time"`
	Success     bool   `bson:"success"`
}

// Id implements ModelMigration.
func (mig *modelMigration) Id() string {
	return mig.doc.Id
}

// ModelUUID implements ModelMigration.
func (mig *modelMigration) ModelUUID() string {
	return mig.doc.ModelUUID
}

// Attempt implements ModelMigration.
func (mig *modelMigration) Attempt() int {
	return mig.doc.Attempt
}

// StartTime implements ModelMigration.
func (mig *modelMigration) StartTime() time.Time {
	return unixNanoToTime0(mig.statusDoc.StartTime)
}

// SuccessTime implements ModelMigration.
func (mig *modelMigration) SuccessTime() time.Time {
	return unixNanoToTime0(mig.statusDoc.SuccessTime)
}

// EndTime implements ModelMigration.
func (mig *modelMigration) EndTime() time.Time {
	return unixNanoToTime0(mig.statusDoc.EndTime)
}

// Phase implements ModelMigration.
func (mig *modelMigration) Phase() (migration.Phase, error) {
	phase, ok := migration.ParsePhase(mig.statusDoc.Phase)
	if !ok {
		return phase, errors.Errorf("invalid phase in DB: %v", mig.statusDoc.Phase)
	}
	return phase, nil
}

// PhaseChangedTime implements ModelMigration.
func (mig *modelMigration) PhaseChangedTime() time.Time {
	return unixNanoToTime0(mig.statusDoc.PhaseChangedTime)
}

// StatusMessage implements ModelMigration.
func (mig *modelMigration) StatusMessage() string {
	return mig.statusMessageDoc.StatusMessage
}

// InitiatedBy implements ModelMigration.
func (mig *modelMigration) InitiatedBy() string {
	return mig.doc.InitiatedBy
}

// TargetInfo implements ModelMigration.
func (mig *modelMigration) TargetInfo() (*migration.TargetInfo, error) {
	authTag, err := names.ParseUserTag(mig.doc.TargetAuthTag)
	if err != nil {
		return nil, errors.Trace(err)
	}
	macs, err := jsonToMacaroons(mig.doc.TargetMacaroons)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &migration.TargetInfo{
		ControllerTag:   names.NewControllerTag(mig.doc.TargetController),
		ControllerAlias: mig.doc.TargetControllerAlias,
		Addrs:           mig.doc.TargetAddrs,
		CACert:          mig.doc.TargetCACert,
		AuthTag:         authTag,
		Password:        mig.doc.TargetPassword,
		Macaroons:       macs,
	}, nil
}

// SetPhase implements ModelMigration.
func (mig *modelMigration) SetPhase(nextPhase migration.Phase) error {
	now := mig.st.clock().Now().UnixNano()

	phase, err := mig.Phase()
	if err != nil {
		return errors.Trace(err)
	}

	if nextPhase == phase {
		return nil // Already at that phase. Nothing to do.
	}
	if !phase.CanTransitionTo(nextPhase) {
		return errors.Errorf("illegal phase change: %s -> %s", phase, nextPhase)
	}

	nextDoc := mig.statusDoc
	nextDoc.Phase = nextPhase.String()
	nextDoc.PhaseChangedTime = now
	update := bson.M{
		"phase":              nextDoc.Phase,
		"phase-changed-time": now,
	}
	if nextPhase == migration.SUCCESS {
		nextDoc.SuccessTime = now
		update["success-time"] = now
	}

	ops, err := migStatusHistoryAndOps(mig.st, nextPhase, now, mig.StatusMessage())
	if err != nil {
		return errors.Trace(err)
	}

	// If the migration aborted, make the model active again.
	if nextPhase == migration.ABORTDONE {
		ops = append(ops, txn.Op{
			C:      modelsC,
			Id:     mig.doc.ModelUUID,
			Assert: txn.DocExists,
			Update: bson.M{
				"$set": bson.M{"migration-mode": MigrationModeNone},
			},
		})
	}

	// Set end timestamps and mark migration as no longer active if a
	// terminal phase is hit.
	if nextPhase.IsTerminal() {
		nextDoc.EndTime = now
		update["end-time"] = now
		ops = append(ops, txn.Op{
			C:      migrationsActiveC,
			Id:     mig.doc.ModelUUID,
			Assert: txn.DocExists,
			Remove: true,
		})
	}

	ops = append(ops, txn.Op{
		C:      migrationsStatusC,
		Id:     mig.statusDoc.Id,
		Update: bson.M{"$set": update},
		// Ensure phase hasn't changed underneath us
		Assert: bson.M{"phase": mig.statusDoc.Phase},
	})

	if err := mig.st.db().RunTransaction(ops); err == txn.ErrAborted {
		return errors.New("phase already changed")
	} else if err != nil {
		return errors.Annotate(err, "failed to update phase")
	}

	mig.statusDoc = nextDoc
	return nil
}

// migStatusHistoryAndOps sets the model's status history and returns ops for
// setting model status according to the phase and message.
func migStatusHistoryAndOps(st *State, phase migration.Phase, now int64, msg string) ([]txn.Op, error) {
	switch phase {
	case migration.REAP, migration.DONE:
		// if we're reaping/have reaped the model, setting status on it is both
		// pointless and potentially problematic.
		return nil, nil
	}
	model, err := st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	globalKey := model.globalKey()
	modelStatus := status.Busy
	if phase.IsTerminal() {
		modelStatus = status.Available
	}
	if msg != "" {
		msg = "migrating: " + msg
	}
	doc := statusDoc{
		Status:     modelStatus,
		StatusInfo: msg,
		Updated:    now,
	}

	ops, err := statusSetOps(st.db(), doc, globalKey)
	if err != nil {
		return nil, errors.Trace(err)
	}
	_, _ = probablyUpdateStatusHistory(st.db(), globalKey, doc)
	return ops, nil
}

// SetStatusMessage implements ModelMigration.
func (mig *modelMigration) SetStatusMessage(text string) error {
	phase, err := mig.Phase()
	if err != nil {
		return errors.Trace(err)
	}

	ops, err := migStatusHistoryAndOps(mig.st, phase, mig.st.clock().Now().UnixNano(), text)
	if err != nil {
		return errors.Trace(err)
	}

	ops = append(ops, txn.Op{
		C:      migrationsStatusMessageC,
		Id:     mig.statusDoc.Id,
		Update: bson.M{"$set": bson.M{"status-message": text}},
		Assert: txn.DocExists,
	})
	if err := mig.st.db().RunTransaction(ops); err != nil {
		return errors.Annotate(err, "failed to set migration status")
	}
	mig.statusMessageDoc.StatusMessage = text
	return nil
}

// SubmitMinionReport implements ModelMigration.
func (mig *modelMigration) SubmitMinionReport(tag names.Tag, phase migration.Phase, success bool) error {
	globalKey, err := agentTagToGlobalKey(tag)
	if err != nil {
		return errors.Trace(err)
	}
	docID := mig.minionReportId(phase, globalKey)
	doc := modelMigMinionSyncDoc{
		Id:          docID,
		MigrationId: mig.Id(),
		Phase:       phase.String(),
		EntityKey:   globalKey,
		Time:        mig.st.clock().Now().UnixNano(),
		Success:     success,
	}
	ops := []txn.Op{{
		C:      migrationsMinionSyncC,
		Id:     docID,
		Insert: doc,
		Assert: txn.DocMissing,
	}}
	err = mig.st.db().RunTransaction(ops)
	if errors.Cause(err) == txn.ErrAborted {
		coll, closer := mig.st.db().GetCollection(migrationsMinionSyncC)
		defer closer()
		var existingDoc modelMigMinionSyncDoc
		err := coll.FindId(docID).Select(bson.M{"success": 1}).One(&existingDoc)
		if err != nil {
			return errors.Annotate(err, "checking existing report")
		}
		if existingDoc.Success != success {
			return errors.Errorf("conflicting reports received for %s/%s/%s",
				mig.Id(), phase.String(), tag)
		}
		return nil
	} else if err != nil {
		return errors.Trace(err)
	}
	return nil
}

// MinionReports implements ModelMigration.
func (mig *modelMigration) MinionReports() (*MinionReports, error) {
	all, err := mig.getAllAgents()
	if err != nil {
		return nil, errors.Trace(err)
	}

	phase, err := mig.Phase()
	if err != nil {
		return nil, errors.Annotate(err, "retrieving phase")
	}

	coll, closer := mig.st.db().GetCollection(migrationsMinionSyncC)
	defer closer()
	query := coll.Find(bson.M{"_id": bson.M{
		"$regex": "^" + mig.minionReportId(phase, ".+"),
	}})
	query = query.Select(bson.M{
		"entity-key": 1,
		"success":    1,
	})
	var docs []bson.M
	if err := query.All(&docs); err != nil {
		return nil, errors.Annotate(err, "retrieving minion reports")
	}

	succeeded := names.NewSet()
	failed := names.NewSet()
	for _, doc := range docs {
		entityKey, ok := doc["entity-key"].(string)
		if !ok {
			return nil, errors.Errorf("unexpected entity-key %v", doc["entity-key"])
		}
		tag, err := globalKeyToAgentTag(entityKey)
		if err != nil {
			return nil, errors.Trace(err)
		}
		success, ok := doc["success"].(bool)
		if !ok {
			return nil, errors.Errorf("unexpected success value: %v", doc["success"])
		}
		if success {
			succeeded.Add(tag)
		} else {
			failed.Add(tag)
		}
	}

	unknown := all.Difference(succeeded).Difference(failed)

	return &MinionReports{
		Succeeded: succeeded.Values(),
		Failed:    failed.Values(),
		Unknown:   unknown.Values(),
	}, nil
}

// WatchMinionReports implements ModelMigration.
func (mig *modelMigration) WatchMinionReports() (NotifyWatcher, error) {
	phase, err := mig.Phase()
	if err != nil {
		return nil, errors.Annotate(err, "retrieving phase")
	}
	prefix := mig.minionReportId(phase, "")
	filter := func(rawId interface{}) bool {
		id, ok := rawId.(string)
		if !ok {
			return false
		}
		return strings.HasPrefix(id, prefix)
	}
	return newNotifyCollWatcher(mig.st, migrationsMinionSyncC, filter), nil
}

func (mig *modelMigration) minionReportId(phase migration.Phase, globalKey string) string {
	return fmt.Sprintf("%s:%s:%s", mig.Id(), phase.String(), globalKey)
}

func (mig *modelMigration) getAllAgents() (names.Set, error) {
	agentTags := names.NewSet()
	machineTags, err := mig.loadAgentTags(machinesC, "machineid",
		func(id string) names.Tag { return names.NewMachineTag(id) },
	)
	if err != nil {
		return nil, errors.Annotate(err, "loading machine tags")
	}
	agentTags = agentTags.Union(machineTags)

	m, err := mig.st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	if m.Type() != ModelTypeCAAS {
		unitTags, err := mig.loadAgentTags(unitsC, "name",
			func(name string) names.Tag { return names.NewUnitTag(name) },
		)
		if err != nil {
			return nil, errors.Annotate(err, "loading unit names")
		}

		return agentTags.Union(unitTags), nil
	}

	applicationTags, err := mig.loadAgentTags(applicationsC, "name",
		func(name string) names.Tag { return names.NewApplicationTag(name) },
	)
	if err != nil {
		return nil, errors.Annotate(err, "loading application names")
	}
	for _, applicationTag := range applicationTags.Values() {
		app, err := mig.st.Application(applicationTag.Id())
		if err != nil {
			return nil, errors.Trace(err)
		}
		isSidecar, err := app.IsSidecar()
		if err != nil {
			return nil, errors.Trace(err)
		}
		if !isSidecar {
			agentTags.Add(applicationTag)
			continue
		}
		unitNames, err := app.UnitNames()
		if err != nil {
			return nil, errors.Trace(err)
		}
		for _, unitName := range unitNames {
			agentTags.Add(names.NewUnitTag(unitName))
		}
	}
	return agentTags, nil
}

func (mig *modelMigration) loadAgentTags(collName, fieldName string, convert func(string) names.Tag) (
	names.Set, error,
) {
	// During migrations we know that nothing there are no machines or
	// units being provisioned or destroyed so a simple query of the
	// collections will do.
	coll, closer := mig.st.db().GetCollection(collName)
	defer closer()
	var docs []bson.M
	err := coll.Find(nil).Select(bson.M{fieldName: 1}).All(&docs)
	if err != nil {
		return nil, errors.Trace(err)
	}

	out := names.NewSet()
	for _, doc := range docs {
		v, ok := doc[fieldName].(string)
		if !ok {
			return nil, errors.Errorf("invalid %s value: %v", fieldName, doc[fieldName])
		}
		out.Add(convert(v))
	}
	return out, nil
}

// Refresh implements ModelMigration.
func (mig *modelMigration) Refresh() error {
	// Only the status document is updated. The modelMigDoc is static
	// after creation.
	statusColl, closer := mig.st.db().GetCollection(migrationsStatusC)
	defer closer()
	var statusDoc modelMigStatusDoc
	err := statusColl.FindId(mig.doc.Id).One(&statusDoc)
	if err == mgo.ErrNotFound {
		return errors.NotFoundf("migration status")
	} else if err != nil {
		return errors.Annotate(err, "migration status lookup failed")
	}

	statusMessageColl, closer := mig.st.db().GetCollection(migrationsStatusMessageC)
	defer closer()
	var statusMessageDoc modelMigStatusMessageDoc
	err = statusMessageColl.FindId(mig.doc.Id).One(&statusMessageDoc)
	if err == mgo.ErrNotFound {
		return errors.NotFoundf("migration status message")
	} else if err != nil {
		return errors.Annotate(err, "migration status message lookup failed")
	}

	mig.statusDoc = statusDoc
	mig.statusMessageDoc = statusMessageDoc

	return nil
}

// ModelUserAccess implements ModelMigration.
func (mig *modelMigration) ModelUserAccess(tag names.Tag) permission.Access {
	id := tag.Id()
	for _, user := range mig.doc.ModelUsers {
		if user.UserID == id {
			return user.Access
		}
	}

	return permission.NoAccess
}

// MigrationSpec holds the information required to create a
// ModelMigration instance.
type MigrationSpec struct {
	InitiatedBy names.UserTag
	TargetInfo  migration.TargetInfo
}

// Validate returns an error if the MigrationSpec contains bad
// data. Nil is returned otherwise.
func (spec *MigrationSpec) Validate() error {
	if !names.IsValidUser(spec.InitiatedBy.Id()) {
		return errors.NotValidf("InitiatedBy")
	}
	return spec.TargetInfo.Validate()
}

// CreateMigration initialises state that tracks a model migration. It
// will return an error if there is already a model migration in
// progress.
func (st *State) CreateMigration(spec MigrationSpec) (ModelMigration, error) {
	if st.IsController() {
		return nil, errors.New("controllers can't be migrated")
	}
	if err := spec.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	if err := checkTargetController(st, spec.TargetInfo.ControllerTag); err != nil {
		return nil, errors.Trace(err)
	}

	now := st.clock().Now().UnixNano()
	modelUUID := st.ModelUUID()
	var doc modelMigDoc
	var statusDoc modelMigStatusDoc
	var statusMessageDoc modelMigStatusMessageDoc

	msg := "starting"
	ops, err := migStatusHistoryAndOps(st, migration.QUIESCE, now, msg)
	if err != nil {
		return nil, errors.Trace(err)
	}

	buildTxn := func(int) ([]txn.Op, error) {
		model, err := st.Model()
		if err != nil {
			return nil, errors.Annotate(err, "failed to load model")
		}
		if model.Life() != Alive {
			return nil, errors.New("model is not alive")
		}

		if isActive, err := st.IsMigrationActive(); err != nil {
			return nil, errors.Trace(err)
		} else if isActive {
			return nil, errors.New("already in progress")
		}

		macsJSON, err := macaroonsToJSON(spec.TargetInfo.Macaroons)
		if err != nil {
			return nil, errors.Trace(err)
		}

		attempt, err := sequence(st, "modelmigration")
		if err != nil {
			return nil, errors.Trace(err)
		}

		userDocs, err := modelUserDocs(model)
		if err != nil {
			return nil, errors.Trace(err)
		}

		id := fmt.Sprintf("%s:%d", modelUUID, attempt)
		doc = modelMigDoc{
			Id:                    id,
			ModelUUID:             modelUUID,
			Attempt:               attempt,
			InitiatedBy:           spec.InitiatedBy.Id(),
			TargetController:      spec.TargetInfo.ControllerTag.Id(),
			TargetControllerAlias: spec.TargetInfo.ControllerAlias,
			TargetAddrs:           spec.TargetInfo.Addrs,
			TargetCACert:          spec.TargetInfo.CACert,
			TargetAuthTag:         spec.TargetInfo.AuthTag.String(),
			TargetPassword:        spec.TargetInfo.Password,
			TargetMacaroons:       macsJSON,
			ModelUsers:            userDocs,
		}

		statusDoc = modelMigStatusDoc{
			Id:               id,
			StartTime:        now,
			Phase:            migration.QUIESCE.String(),
			PhaseChangedTime: now,
		}

		statusMessageDoc = modelMigStatusMessageDoc{
			Id:            id,
			StatusMessage: msg,
		}

		ops := append(ops, []txn.Op{{
			C:      migrationsC,
			Id:     doc.Id,
			Assert: txn.DocMissing,
			Insert: &doc,
		}, {
			C:      migrationsStatusC,
			Id:     statusDoc.Id,
			Assert: txn.DocMissing,
			Insert: &statusDoc,
		}, {
			C:      migrationsStatusMessageC,
			Id:     statusDoc.Id,
			Assert: txn.DocMissing,
			Insert: &statusMessageDoc,
		}, {
			C:      migrationsActiveC,
			Id:     modelUUID,
			Assert: txn.DocMissing,
			Insert: bson.M{"id": doc.Id},
		}, {
			C:      modelsC,
			Id:     modelUUID,
			Assert: txn.DocExists,
			Update: bson.M{"$set": bson.M{
				"migration-mode": MigrationModeExporting,
			}},
		}, model.assertActiveOp(),
		}...)
		return ops, nil
	}
	if err := st.db().Run(buildTxn); err != nil {
		return nil, errors.Annotate(err, "failed to create migration")
	}

	return &modelMigration{
		doc:              doc,
		statusDoc:        statusDoc,
		statusMessageDoc: statusMessageDoc,
		st:               st,
	}, nil
}

func modelUserDocs(m *Model) ([]modelMigUserDoc, error) {
	users, err := m.Users()
	if err != nil {
		return nil, err
	}

	var docs []modelMigUserDoc
	for _, user := range users {
		docs = append(docs, modelMigUserDoc{
			UserID: user.UserTag.Id(),
			Access: user.Access,
		})
	}

	return docs, nil
}

func macaroonsToJSON(m []macaroon.Slice) (string, error) {
	if len(m) == 0 {
		return "", nil
	}
	j, err := json.Marshal(m)
	if err != nil {
		return "", errors.Annotate(err, "marshalling macaroons")
	}
	return string(j), nil
}

func jsonToMacaroons(raw string) ([]macaroon.Slice, error) {
	if raw == "" {
		return nil, nil
	}
	var macs []macaroon.Slice
	if err := json.Unmarshal([]byte(raw), &macs); err != nil {
		return nil, errors.Annotate(err, "unmarshalling macaroon")
	}
	return macs, nil
}

func checkTargetController(st *State, targetControllerTag names.ControllerTag) error {
	if targetControllerTag.Id() == st.ControllerUUID() {
		return errors.New("model already attached to target controller")
	}
	return nil
}

// LatestMigration returns the most recent ModelMigration (if any) for a model
// that has not been removed from the state. Callers interested in
// ModelMigrations for models that have been removed after a successful
// migration to another controller should use CompletedMigration
// instead.
func (st *State) LatestMigration() (ModelMigration, error) {
	mig, phase, err := st.latestMigration(st.ModelUUID())
	if err != nil {
		return nil, err
	}

	// Hide previous migrations for models which have been migrated
	// away from a model and then migrated back.
	if phase == migration.DONE {
		model, err := st.Model()
		if errors.Is(err, errors.NotFound) {
			// The model not being found breaks the precondition of this
			// function, the model should be in state. However, to make this
			// function more resilient and allow it to be called when the models
			// existence is unknown, we do not return an error here.
			logger.Debugf("checking latest migration: migrated model has been removed from the state")
			return mig, nil
		} else if err != nil {
			return nil, errors.Trace(err)
		}
		if model != nil && model.MigrationMode() == MigrationModeNone {
			return nil, errors.NotFoundf("migration")
		}
	}

	return mig, nil
}

// CompletedMigration returns the most recent migration for this state's
// model if it reached the DONE phase and caused the model to be relocated.
func (st *State) CompletedMigration() (ModelMigration, error) {
	mig, err := st.CompletedMigrationForModel(st.ModelUUID())
	return mig, errors.Trace(err)
}

// CompletedMigrationForModel returns the most recent migration for the
// input model UUID if it reached the DONE phase and caused the model
// to be relocated.
func (st *State) CompletedMigrationForModel(modelUUID string) (ModelMigration, error) {
	mig, phase, err := st.latestMigration(modelUUID)
	if err != nil {
		return nil, errors.Trace(err)
	}
	// Return NotFound if the model still modelExists or the migration is not
	// flagged as completed.
	modelExists, err := st.ModelExists(modelUUID)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if phase != migration.DONE || modelExists {
		return nil, errors.NotFoundf("migration")
	}

	return mig, nil
}

// latestMigration returns the most recent ModelMigration for a model
// (if any).
func (st *State) latestMigration(modelUUID string) (ModelMigration, migration.Phase, error) {
	migColl, closer := st.db().GetCollection(migrationsC)
	defer closer()
	query := migColl.Find(bson.M{"model-uuid": modelUUID})
	query = query.Sort("-attempt").Limit(1)
	mig, err := st.migrationFromQuery(query)
	if err != nil {
		return nil, migration.UNKNOWN, errors.Trace(err)
	}

	// Hide previous migrations for models which have been migrated
	// away from a model and then migrated back.
	phase, err := mig.Phase()
	if err != nil {
		return nil, migration.UNKNOWN, errors.Trace(err)
	}
	return mig, phase, nil
}

// Migration retrieves a specific ModelMigration by its id. See also
// LatestMigration and LatestCompletedMigration.
func (st *State) Migration(id string) (ModelMigration, error) {
	migColl, closer := st.db().GetCollection(migrationsC)
	defer closer()
	mig, err := st.migrationFromQuery(migColl.FindId(id))
	if err != nil {
		return nil, errors.Trace(err)
	}
	return mig, nil
}

func (st *State) migrationFromQuery(query mongo.Query) (ModelMigration, error) {
	var doc modelMigDoc
	err := query.One(&doc)
	if err == mgo.ErrNotFound {
		return nil, errors.NotFoundf("migration")
	} else if err != nil {
		return nil, errors.Annotate(err, "migration lookup failed")
	}

	statusColl, closer := st.db().GetCollection(migrationsStatusC)
	defer closer()
	var statusDoc modelMigStatusDoc
	err = statusColl.FindId(doc.Id).One(&statusDoc)
	if err == mgo.ErrNotFound {
		return nil, errors.NotFoundf("migration status")
	} else if err != nil {
		return nil, errors.Annotate(err, "migration status lookup failed")
	}

	statusMessageColl, closer := st.db().GetCollection(migrationsStatusMessageC)
	defer closer()
	var statusMessageDoc modelMigStatusMessageDoc
	err = statusMessageColl.FindId(doc.Id).One(&statusMessageDoc)
	if err == mgo.ErrNotFound {
		return nil, errors.NotFoundf("migration status message")
	} else if err != nil {
		return nil, errors.Annotate(err, "migration status message lookup failed")
	}

	return &modelMigration{
		doc:              doc,
		statusDoc:        statusDoc,
		statusMessageDoc: statusMessageDoc,
		st:               st,
	}, nil
}

// IsMigrationActive returns true if a migration is in progress for
// the model associated with the State.
func (st *State) IsMigrationActive() (bool, error) {
	return IsMigrationActive(st, st.ModelUUID())
}

// IsMigrationActive returns true if a migration is in progress for
// the model with the given UUID. The State provided need not be for
// the model in question.
func IsMigrationActive(st *State, modelUUID string) (bool, error) {
	active, closer := st.db().GetCollection(migrationsActiveC)
	defer closer()
	n, err := active.FindId(modelUUID).Count()
	if err != nil {
		return false, errors.Trace(err)
	}
	return n > 0, nil
}

func unixNanoToTime0(i int64) time.Time {
	if i == 0 {
		return time.Time{}
	}
	return time.Unix(0, i)
}

func agentTagToGlobalKey(tag names.Tag) (string, error) {
	switch t := tag.(type) {
	case names.MachineTag:
		return machineGlobalKey(t.Id()), nil
	case names.UnitTag:
		return unitAgentGlobalKey(t.Id()), nil
	case names.ApplicationTag:
		return applicationGlobalKey(t.Id()), nil
	default:
		return "", errors.Errorf("%s is not an agent tag", tag)
	}
}

func globalKeyToAgentTag(key string) (names.Tag, error) {
	parts := strings.SplitN(key, "#", 2)
	if len(parts) != 2 {
		return nil, errors.NotValidf("global key %q", key)
	}
	keyType, keyId := parts[0], parts[1]
	switch keyType {
	case "m":
		return names.NewMachineTag(keyId), nil
	case "u":
		return names.NewUnitTag(keyId), nil
	case "a":
		return names.NewApplicationTag(keyId), nil
	default:
		return nil, errors.NotValidf("global key type %q", keyType)
	}
}

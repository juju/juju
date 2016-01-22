// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names"
	jujutxn "github.com/juju/txn"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	migration "github.com/juju/juju/core/modelmigration"
	"github.com/juju/juju/network"
)

// This file contains functionality for managing the state documents
// used by Juju to track model migrations.

// ModelMigration represents the state of an migration attempt for a
// model.
type ModelMigration struct {
	st  *State
	doc modelMigrationDoc
}

// modelMigrationDoc tracks the state of a migration attempt for a
// model.
type modelMigrationDoc struct {
	// Id holds migration document key. It has the format
	// "uuid:sequence".
	Id string `bson:"_id"`

	// The UUID of the model being migrated.
	ModelUUID string `bson:"model-uuid"`

	// StartTime holds the time the migration started (stored as per
	// UnixNano).
	StartTime int64 `bson:"start-time"`

	// StartTime holds the time the migration reached the SUCCESS phase (stored as per
	// UnixNano).
	SuccessTime int64 `bson:"success-time"`

	// StartTime holds the time the migration reached a terminal (end)
	// phase (stored as per UnixNano).
	EndTime int64 `bson:"end-time"`

	// Phase holds the current migration phase. This should be one of
	// the string representations of the core/migrations.Phase
	// constants.
	Phase string `bson:"phase"`

	// StartTime holds the time that Phase last changed (stored as per
	// UnixNano).
	PhaseChangedTime int64 `bson:"phase-changed-time"`

	// StatusMessage holds a human readable message about the
	// migration's progress.
	StatusMessage string `bson:"status-message"`

	// InitiatedBy holds the username of the user that triggered the
	// migration. It should be in "user@domain" format.
	InitiatedBy string `bson:"initiatedBy"`

	// TargetController holds the UUID of the target controller.
	TargetController string `bson:"target-controller"`

	// TargetAddrs holds the host:port values for the target API
	// server.
	TargetAddrs []string `bson:"target-addrs"`

	// TargetCACert holds the certificate to validate the target API
	// server's TLS certificate.
	TargetCACert string `bson:"target-cacert"`

	// TargetEntityTag holds a string representation of the tag to
	// authenticate to the target controller with.
	TargetEntityTag string `bson:"target-entity"`

	// TargetPassword holds the password to use with TargetEntityTag
	// when authenticating.
	TargetPassword string `bson:"target-password"`
}

// Id returns a unique identifier for the model migration.
func (mig *ModelMigration) Id() string {
	return mig.doc.Id
}

// ModelUUID returns the UUID for the model being migrated.
func (mig *ModelMigration) ModelUUID() string {
	return mig.doc.ModelUUID
}

// StartTime returns the time when the migration was started.
func (mig *ModelMigration) StartTime() time.Time {
	return *unixNanoToTime0(mig.doc.StartTime)
}

// SuccessTime returns the time when the migration reached
// SUCCESS.
func (mig *ModelMigration) SuccessTime() time.Time {
	if mig.doc.SuccessTime == 0 {
		return time.Time{}
	}
	return *unixNanoToTime0(mig.doc.SuccessTime)
}

// EndTime returns the time when the migration reached DONE or
// REAPFAILED.
func (mig *ModelMigration) EndTime() time.Time {
	return *unixNanoToTime0(mig.doc.EndTime)
}

// Phase returns the migration's phase.
func (mig *ModelMigration) Phase() (migration.Phase, error) {
	phase, ok := migration.ParsePhase(mig.doc.Phase)
	if !ok {
		return phase, errors.Errorf("invalid phase in DB: %v", mig.doc.Phase)
	}
	return phase, nil
}

// PhaseChangedTime returns the time when the migration's phase last
// changed.
func (mig *ModelMigration) PhaseChangedTime() time.Time {
	return *unixNanoToTime0(mig.doc.PhaseChangedTime)
}

// StatusMessage returns human readable text about the current
// progress of the migration.
func (mig *ModelMigration) StatusMessage() string {
	return mig.doc.StatusMessage
}

// InitiatedBy returns username the initiated the migration.
func (mig *ModelMigration) InitiatedBy() string {
	return mig.doc.InitiatedBy
}

// TargetInfo returns the details required to connect to the
// migration's target controller.
func (mig *ModelMigration) TargetInfo() (*ModelMigTargetInfo, error) {
	entityTag, err := names.ParseTag(mig.doc.TargetEntityTag)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &ModelMigTargetInfo{
		ControllerTag: names.NewEnvironTag(mig.doc.TargetController),
		Addrs:         mig.doc.TargetAddrs,
		CACert:        mig.doc.TargetCACert,
		EntityTag:     entityTag,
		Password:      mig.doc.TargetPassword,
	}, nil
}

// SetPhase sets the phase of the migration. An error will be returned
// if the new phase does not follow the current phase or if the
// migration is no longer active.
func (mig *ModelMigration) SetPhase(nextPhase migration.Phase) error {
	now := GetClock().Now().UnixNano()

	buildTxn := func(attempt int) ([]txn.Op, error) {
		if attempt > 0 {
			if err := mig.Refresh(); err != nil {
				return nil, errors.Trace(err)
			}
		}

		phase, err := mig.Phase()
		if err != nil {
			return nil, errors.Trace(err)
		}
		if nextPhase == phase {
			// Already at that phase. Nothing to do.
			return nil, jujutxn.ErrNoOperations
		}
		if !phase.CanTransitionTo(nextPhase) {
			return nil, errors.Errorf("illegal change: %s -> %s", mig.doc.Phase, nextPhase)
		}

		var ops []txn.Op
		update := bson.M{
			"phase":              nextPhase.String(),
			"phase-changed-time": now,
		}
		if nextPhase == migration.SUCCESS {
			update["success-time"] = now
		}
		if nextPhase.IsTerminal() {
			update["end-time"] = now
			ops = append(ops, txn.Op{
				C:      activeModelMigrationsC,
				Id:     mig.doc.ModelUUID,
				Assert: txn.DocExists,
				Remove: true,
			})
		}
		ops = append(ops, txn.Op{
			C:      modelMigrationsC,
			Id:     mig.doc.Id,
			Update: bson.M{"$set": update},
			// Ensure phase hasn't changed underneath us
			Assert: bson.M{"phase": mig.doc.Phase},
		})
		return ops, nil
	}
	if err := mig.st.run(buildTxn); err != nil {
		return errors.Annotate(err, "failed to update phase")
	}
	return errors.Trace(mig.Refresh())
}

// SetStatusMessage sets some human readable text about the current
// progress of the migration.
func (mig *ModelMigration) SetStatusMessage(text string) error {
	ops := []txn.Op{{
		C:      modelMigrationsC,
		Id:     mig.doc.Id,
		Update: bson.M{"$set": bson.M{"status-message": text}},
		Assert: txn.DocExists,
	}}
	if err := mig.st.runTransaction(ops); err != nil {
		return errors.Annotate(err, "failed to set migration status")
	}
	mig.doc.StatusMessage = text
	return nil
}

// Refresh updates the contents of the ModelMigration from the underlying
// state.
func (mig *ModelMigration) Refresh() error {
	migColl, closer := mig.st.getCollection(modelMigrationsC)
	defer closer()

	var doc modelMigrationDoc
	err := migColl.FindId(mig.doc.Id).One(&doc)
	if err == mgo.ErrNotFound {
		return errors.NotFoundf("migration")
	} else if err != nil {
		return errors.Annotate(err, "migration lookup failed")
	}

	mig.doc = doc
	return nil
}

// ModelMigrationSpec holds the information required to create an
// ModelMigration instance.
type ModelMigrationSpec struct {
	InitiatedBy string
	TargetInfo  ModelMigTargetInfo
}

// ModelMigTargetInfo holds the details required to connect to a
// migration's target controller.
//
// TODO(mjs) - Note the similarity to api.Info. It would be nice
// to be able to use api.Info here but state can't import api and
// moving api.Info to live under the core package is too big a project
// to be done right now.
type ModelMigTargetInfo struct {
	// ControllerTag holds tag for the target controller.
	ControllerTag names.EnvironTag

	// Addrs holds the addresses and ports of the target controller's
	// API servers.
	Addrs []string

	// CACert holds the CA certificate that will be used to validate
	// the target API server's certificate, in PEM format.
	CACert string

	// EntityTag holds the entity to authenticate with to the target
	// controller.
	EntityTag names.Tag

	// Password holds the password to use with TargetEntityTag.
	Password string
}

func (spec *ModelMigrationSpec) Validate() error {
	if spec.InitiatedBy == "" {
		return errors.NotValidf("empty InitiatedBy")
	}

	target := &spec.TargetInfo
	if !names.IsValidEnvironment(target.ControllerTag.Id()) {
		return errors.NotValidf("ControllerTag")
	}

	if target.Addrs == nil {
		return errors.NotValidf("nil Addrs")
	}
	if len(target.Addrs) < 1 {
		return errors.NotValidf("empty Addrs")
	}
	for _, addr := range target.Addrs {
		_, err := network.ParseHostPort(addr)
		if err != nil {
			return errors.NotValidf("%q in Addrs", addr)
		}
	}

	if target.CACert == "" {
		return errors.NotValidf("empty CACert")
	}

	if target.EntityTag.Id() == "" {
		return errors.NotValidf("empty EntityTag")
	}

	if target.Password == "" {
		return errors.NotValidf("empty Password")
	}

	return nil
}

// CreateModelMigration initialises state that tracks a model
// migration. It will return an error if there is already an
// environment migration in progress.
func CreateModelMigration(st *State, spec ModelMigrationSpec) (*ModelMigration, error) {
	if st.IsStateServer() {
		return nil, errors.New("controllers can't be migrated")
	}
	if err := spec.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	now := GetClock().Now().UnixNano()
	modelUUID := st.EnvironUUID()
	var doc modelMigrationDoc
	buildTxn := func(int) ([]txn.Op, error) {
		if isActive, err := IsModelMigrationActive(st, modelUUID); err != nil {
			return nil, errors.Trace(err)
		} else if isActive {
			return nil, errors.New("already in progress")
		}

		seq, err := st.sequence("modelmigration")
		if err != nil {
			return nil, errors.Trace(err)
		}

		doc = modelMigrationDoc{
			Id:               fmt.Sprintf("%s:%d", modelUUID, seq),
			ModelUUID:        modelUUID,
			InitiatedBy:      spec.InitiatedBy,
			StartTime:        now,
			Phase:            migration.QUIESCE.String(),
			PhaseChangedTime: now,
			TargetController: spec.TargetInfo.ControllerTag.Id(),
			TargetAddrs:      spec.TargetInfo.Addrs,
			TargetCACert:     spec.TargetInfo.CACert,
			TargetEntityTag:  spec.TargetInfo.EntityTag.String(),
			TargetPassword:   spec.TargetInfo.Password,
		}
		return []txn.Op{{
			C:      modelMigrationsC,
			Id:     doc.Id,
			Assert: txn.DocMissing,
			Insert: &doc,
		}, {
			C:      activeModelMigrationsC,
			Id:     modelUUID,
			Assert: txn.DocMissing,
			Insert: bson.M{"id": doc.Id},
		}}, nil
	}
	if err := st.run(buildTxn); err != nil {
		return nil, errors.Annotate(err, "failed to create migration")
	}

	return &ModelMigration{
		doc: doc,
		st:  st,
	}, nil
}

// GetModelMigration returns the most recent ModelMigration for a
// model (if any).
func GetModelMigration(st *State) (*ModelMigration, error) {
	migColl, closer := st.getCollection(modelMigrationsC)
	defer closer()

	query := migColl.Find(bson.M{"model-uuid": st.EnvironUUID()})
	query = query.Sort("-_id").Limit(1)
	var doc modelMigrationDoc
	err := query.One(&doc)
	if err == mgo.ErrNotFound {
		return nil, errors.NotFoundf("migration")
	} else if err != nil {
		return nil, errors.Annotate(err, "migration lookup failed")
	}

	return &ModelMigration{st: st, doc: doc}, nil
}

// IsModelMigrationActive return true if a migration is in progress for
// the given model.
func IsModelMigrationActive(st *State, modelUUID string) (bool, error) {
	active, closer := st.getCollection(activeModelMigrationsC)
	defer closer()
	n, err := active.FindId(modelUUID).Count()
	if err != nil {
		return false, errors.Trace(err)
	}
	return n > 0, nil
}

func unixNanoToTime0(i int64) *time.Time {
	if i == 0 {
		return new(time.Time)
	}
	return unixNanoToTime(i)
}

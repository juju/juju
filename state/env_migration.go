// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

// XXX "environment" vs "model"

import (
	"fmt"
	"time"

	"github.com/juju/errors"
	jujutxn "github.com/juju/txn"
	"github.com/juju/utils/clock"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	migration "github.com/juju/juju/core/envmigration"
	"github.com/juju/juju/network"
)

// This file contains functionality for managing the state documents
// used by Juju to track model migrations.

// EnvMigration represents the state of an migration attempt for an
// environment.
type EnvMigration struct {
	st    *State
	doc   envMigrationDoc
	clock clock.Clock
}

// envMigrationDoc tracks the state of a migration attempt for an
// environment.
type envMigrationDoc struct {
	Id                 string    `bson:"_id"`
	EnvUUID            string    `bson:"env-uuid"`
	StartTime          time.Time `bson:"start-time"`
	SuccessTime        time.Time `bson:"success-time"`
	EndTime            time.Time `bson:"end-time"`
	Phase              string    `bson:"phase"`
	PhaseChangedTime   time.Time `bson:"phase-changed-time"`
	StatusMessage      string    `bson:"status-message"`
	Owner              string    `bson:"owner"`
	TargetController   string    `bson:"target-controller"`
	TargetAPIAddresses []string  `bson:"target-api-addresses"`
	TargetUser         string    `bson:"target-user"`
	TargetPassword     string    `bson:"target-password"`
}

// Id returns a unique identifier for the environment migration.
func (mig *EnvMigration) Id() string {
	return mig.doc.Id
}

// EnvUUID returns the environment UUID for the environment being
// migrated.
func (mig *EnvMigration) EnvUUID() string {
	return mig.doc.EnvUUID
}

// StartTime returns the time when the migration was started.
func (mig *EnvMigration) StartTime() time.Time {
	return mig.doc.StartTime
}

// SuccessTime returns the time when the migration reached
// SUCCESS.
func (mig *EnvMigration) SuccessTime() time.Time {
	return mig.doc.SuccessTime
}

// EndTime returns the time when the migration reached DONE or
// REAPFAILED
func (mig *EnvMigration) EndTime() time.Time {
	return mig.doc.EndTime
}

// Phase returns the migration's phase.
func (mig *EnvMigration) Phase() (migration.Phase, error) {
	phase, ok := migration.ParsePhase(mig.doc.Phase)
	if !ok {
		return phase, errors.Errorf("invalid phase in DB: %v", mig.doc.Phase)
	}
	return phase, nil
}

// PhaseChangedTime returns the time when the migration's phase last
// changed.
func (mig *EnvMigration) PhaseChangedTime() time.Time {
	return mig.doc.PhaseChangedTime
}

// StatusMessage returns human readable text about the current
// progress of the migration.
func (mig *EnvMigration) StatusMessage() string {
	return mig.doc.StatusMessage
}

// Owner returns user the initiated the migration.
func (mig *EnvMigration) Owner() string {
	return mig.doc.Owner
}

// TargetController returns UUID of the controller being migrated to.
func (mig *EnvMigration) TargetController() string {
	return mig.doc.TargetController
}

// TargetAPIAddresses returns IP addresses (and ports) of controller
// being migrated to.
func (mig *EnvMigration) TargetAPIAddresses() ([]network.HostPort, error) {
	out, err := network.ParseHostPorts(mig.doc.TargetAPIAddresses...)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return out, nil
}

// TargetAuthInfo returns username and password to use to authenticate
// to the target controller.
func (mig *EnvMigration) TargetAuthInfo() (string, string) {
	return mig.doc.TargetUser, mig.doc.TargetPassword
}

// SetPhase sets the phase of the migration. An error will be returned
// if the new phase does not follow the current phase or if the
// migration is no longer active.
func (mig *EnvMigration) SetPhase(nextPhase migration.Phase) error {
	now := mig.clock.Now()

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
		if !phase.IsNext(nextPhase) {
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
				C:      activeEnvMigrationsC,
				Id:     mig.doc.EnvUUID,
				Assert: txn.DocExists,
				Remove: true,
			})
		}
		ops = append(ops, txn.Op{
			C:      envMigrationsC,
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

// Refresh updates the contents of the EnvMigration from the underlying
// state.
func (mig *EnvMigration) Refresh() error {
	migColl, closer := mig.st.getCollection(envMigrationsC)
	defer closer()

	var doc envMigrationDoc
	err := migColl.FindId(mig.doc.Id).One(&doc)
	if err == mgo.ErrNotFound {
		return errors.NotFoundf("migration")
	} else if err != nil {
		return errors.Annotate(err, "migration lookup failed")
	}

	mig.doc = doc
	return nil
}

// EnvMigrationArgs defines the arguments required to create an
// EnvMigration instance.
type EnvMigrationArgs struct {
	Owner              string
	TargetController   string
	TargetAPIAddresses []network.HostPort
	TargetUser         string
	TargetPassword     string
	Clock              clock.Clock
}

func (a *EnvMigrationArgs) checkAndNormalise() error {
	if a.Owner == "" {
		return errors.NotValidf("empty Owner")
	}
	if a.TargetController == "" {
		return errors.NotValidf("empty TargetController")
	}
	if a.TargetAPIAddresses == nil {
		return errors.NotValidf("empty TargetAPIAddresses")
	}
	if len(a.TargetAPIAddresses) < 1 {
		return errors.NotValidf("empty TargetAPIAddresses")
	}
	if a.TargetUser == "" {
		return errors.NotValidf("empty TargetUser")
	}
	if a.TargetPassword == "" {
		return errors.NotValidf("empty TargetPassword")
	}
	if a.Clock == nil {
		a.Clock = clock.WallClock
	}
	return nil
}

// CreateEnvMigration initialises state that tracks an environment
// migration. It will return an error if there is already an
// environment migration in progress.
func CreateEnvMigration(st *State, args EnvMigrationArgs) (*EnvMigration, error) {
	if err := args.checkAndNormalise(); err != nil {
		return nil, errors.Trace(err)
	}
	if st.IsStateServer() {
		return nil, errors.New("controllers can't be migrated")
	}

	t0 := args.Clock.Now()
	envUUID := st.EnvironUUID()
	var doc envMigrationDoc
	buildTxn := func(int) ([]txn.Op, error) {
		if isActive, err := IsEnvMigrationActive(st, envUUID); err != nil {
			return nil, errors.Trace(err)
		} else if isActive {
			return nil, errors.New("already in progress")
		}

		seq, err := st.sequence("envmigration")
		if err != nil {
			return nil, errors.Trace(err)
		}

		doc = envMigrationDoc{
			Id:               fmt.Sprintf("%s:%d", envUUID, seq),
			EnvUUID:          envUUID,
			Owner:            args.Owner,
			StartTime:        t0,
			Phase:            migration.QUIESCE.String(),
			PhaseChangedTime: t0,
			TargetController: args.TargetController,
			TargetUser:       args.TargetUser,
			TargetPassword:   args.TargetPassword,
		}
		for _, address := range args.TargetAPIAddresses {
			doc.TargetAPIAddresses = append(doc.TargetAPIAddresses, address.String())
		}
		return []txn.Op{
			{
				C:      envMigrationsC,
				Id:     doc.Id,
				Assert: txn.DocMissing,
				Insert: &doc,
			},
			{
				C:      activeEnvMigrationsC,
				Id:     envUUID,
				Assert: txn.DocMissing,
				Insert: bson.M{"id": doc.Id},
			},
		}, nil
	}
	if err := st.run(buildTxn); err != nil {
		return nil, errors.Annotate(err, "failed to create migration")
	}

	return &EnvMigration{
		doc:   doc,
		st:    st,
		clock: args.Clock,
	}, nil
}

// GetEnvMigration returns the most recent EnvMigration for an environment (if any).
func GetEnvMigration(st *State, refClock clock.Clock) (*EnvMigration, error) {
	migColl, closer := st.getCollection(envMigrationsC)
	defer closer()

	var doc envMigrationDoc
	err := migColl.Find(bson.M{"env-uuid": st.EnvironUUID()}).One(&doc)
	if err == mgo.ErrNotFound {
		return nil, errors.NotFoundf("migration")
	} else if err != nil {
		return nil, errors.Annotate(err, "migration lookup failed")
	}

	return &EnvMigration{st: st, doc: doc, clock: refClock}, nil
}

// IsEnvMigrationActive return true if a migration is in progress for
// the given environment.
func IsEnvMigrationActive(st *State, envUUID string) (bool, error) {
	active, closer := st.getCollection(activeEnvMigrationsC)
	defer closer()
	n, err := active.FindId(envUUID).Count()
	if err != nil {
		return false, errors.Trace(err)
	}
	return n > 0, nil
}

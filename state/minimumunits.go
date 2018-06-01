// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/errors"
	jujutxn "github.com/juju/txn"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"
)

// minUnitsDoc keeps track of relevant changes on the application's MinUnits field
// and on the number of alive units for the application.
// A new document is created when MinUnits is set to a non zero value.
// A document is deleted when either the associated application is destroyed
// or MinUnits is restored to zero. The Revno is increased when either MinUnits
// for a application is increased or a unit is destroyed.
// TODO(frankban): the MinUnitsWatcher reacts to changes by sending events,
// each one describing one or more application. A worker reacts to those events
// ensuring the number of units for the application is never less than the actual
// alive units: new units are added if required.
type minUnitsDoc struct {
	// ApplicationName is safe to be used here in place of its globalKey, since
	// the referred entity type is always the Application.
	DocID           string `bson:"_id"`
	ApplicationName string
	ModelUUID       string `bson:"model-uuid"`
	Revno           int
}

// SetMinUnits changes the number of minimum units required by the application.
func (a *Application) SetMinUnits(minUnits int) (err error) {
	defer errors.DeferredAnnotatef(&err, "cannot set minimum units for application %q", a)
	defer func() {
		if err == nil {
			a.doc.MinUnits = minUnits
		}
	}()
	if minUnits < 0 {
		return errors.New("cannot set a negative minimum number of units")
	}
	app := &Application{st: a.st, doc: a.doc}
	// Removing the document never fails. Racing clients trying to create the
	// document generate one failure, but the second attempt should succeed.
	// If one client tries to update the document, and a racing client removes
	// it, the former should be able to re-create the document in the second
	// attempt. If the referred-to application advanced its life cycle to a not
	// alive state, an error is returned after the first failing attempt.
	buildTxn := func(attempt int) ([]txn.Op, error) {
		if attempt > 0 {
			if err := app.Refresh(); err != nil {
				return nil, err
			}
		}
		if app.doc.Life != Alive {
			return nil, errors.New("application is no longer alive")
		}
		if minUnits == app.doc.MinUnits {
			return nil, jujutxn.ErrNoOperations
		}
		return setMinUnitsOps(app, minUnits), nil
	}
	return a.st.db().Run(buildTxn)
}

// setMinUnitsOps returns the operations required to set MinUnits on the
// application and to create/update/remove the minUnits document in MongoDB.
func setMinUnitsOps(app *Application, minUnits int) []txn.Op {
	state := app.st
	applicationname := app.Name()
	ops := []txn.Op{{
		C:      applicationsC,
		Id:     state.docID(applicationname),
		Assert: isAliveDoc,
		Update: bson.D{{"$set", bson.D{{"minunits", minUnits}}}},
	}}
	if app.doc.MinUnits == 0 {
		return append(ops, txn.Op{
			C:      minUnitsC,
			Id:     state.docID(applicationname),
			Assert: txn.DocMissing,
			Insert: &minUnitsDoc{
				ApplicationName: applicationname,
				ModelUUID:       app.st.ModelUUID(),
			},
		})
	}
	if minUnits == 0 {
		return append(ops, minUnitsRemoveOp(state, applicationname))
	}
	if minUnits > app.doc.MinUnits {
		op := minUnitsTriggerOp(state, applicationname)
		op.Assert = txn.DocExists
		return append(ops, op)
	}
	return ops
}

// minUnitsTriggerOp returns the operation required to increase the minimum
// units revno for the application in MongoDB, ignoring the case of document not
// existing. This is included in the operations performed when a unit is
// destroyed: if the document exists, then we need to update the Revno.
// If the application does not require a minimum number of units, then the
// operation is a noop.
func minUnitsTriggerOp(st *State, applicationname string) txn.Op {
	return txn.Op{
		C:      minUnitsC,
		Id:     st.docID(applicationname),
		Update: bson.D{{"$inc", bson.D{{"revno", 1}}}},
	}
}

// minUnitsRemoveOp returns the operation required to remove the minimum
// units document from MongoDB.
func minUnitsRemoveOp(st *State, applicationname string) txn.Op {
	return txn.Op{
		C:      minUnitsC,
		Id:     st.docID(applicationname),
		Remove: true,
	}
}

// MinUnits returns the minimum units count for the application.
func (a *Application) MinUnits() int {
	return a.doc.MinUnits
}

// EnsureMinUnits adds new units if the application's MinUnits value is greater
// than the number of alive units.
func (a *Application) EnsureMinUnits() (err error) {
	defer errors.DeferredAnnotatef(&err, "cannot ensure minimum units for application %q", a)
	app := &Application{st: a.st, doc: a.doc}
	for {
		// Ensure the application is alive.
		if app.doc.Life != Alive {
			return errors.New("application is not alive")
		}
		// Exit without errors if the MinUnits for the application is not set.
		if app.doc.MinUnits == 0 {
			return nil
		}
		// Retrieve the number of alive units for the application.
		aliveUnits, err := aliveUnitsCount(app)
		if err != nil {
			return err
		}
		// Calculate the number of required units to be added.
		missing := app.doc.MinUnits - aliveUnits
		if missing <= 0 {
			return nil
		}
		name, ops, err := ensureMinUnitsOps(app)
		if err != nil {
			return err
		}
		// Add missing unit.
		switch err := a.st.db().RunTransaction(ops); err {
		case nil:
			// Assign the new unit.
			unit, err := a.st.Unit(name)
			if err != nil {
				return err
			}
			if err := app.st.AssignUnit(unit, AssignNew); err != nil {
				return err
			}
			// No need to proceed and refresh the application if this was the
			// last/only missing unit.
			if missing == 1 {
				return nil
			}
		case txn.ErrAborted:
			// Refresh the application and restart the loop.
		default:
			return err
		}
		if err := app.Refresh(); err != nil {
			return err
		}
	}
}

// aliveUnitsCount returns the number a alive units for the application.
func aliveUnitsCount(app *Application) (int, error) {
	units, closer := app.st.db().GetCollection(unitsC)
	defer closer()

	query := bson.D{{"application", app.doc.Name}, {"life", Alive}}
	return units.Find(query).Count()
}

// ensureMinUnitsOps returns the operations required to add a unit for the
// application in MongoDB and the name for the new unit. The resulting transaction
// will be aborted if the application document changes when running the operations.
func ensureMinUnitsOps(app *Application) (string, []txn.Op, error) {
	asserts := bson.D{{"txn-revno", app.doc.TxnRevno}}
	return app.addUnitOps("", AddUnitParams{}, asserts)
}

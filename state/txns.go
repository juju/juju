// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"reflect"

	"github.com/juju/errors"
	jujutxn "github.com/juju/txn"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"
)

const (
	txnAssertEnvIsAlive    = true
	txnAssertEnvIsNotAlive = false
)

// runTransaction is a convenience method delegating to the state's Database.
func (st *State) runTransaction(ops []txn.Op) error {
	runner, closer := st.database.TransactionRunner()
	defer closer()
	return runner.RunTransaction(ops)
}

// runRawTransaction is a convenience method that will run a single
// transaction using a "raw" transaction runner that won't perform
// environment filtering.
func (st *State) runRawTransaction(ops []txn.Op) error {
	runner, closer := st.database.TransactionRunner()
	defer closer()
	if multiRunner, ok := runner.(*multiEnvRunner); ok {
		runner = multiRunner.rawRunner
	}
	return runner.RunTransaction(ops)
}

// run is a convenience method delegating to the state's Database.
func (st *State) run(transactions jujutxn.TransactionSource) error {
	runner, closer := st.database.TransactionRunner()
	defer closer()
	return runner.Run(transactions)
}

// ResumeTransactions resumes all pending transactions.
func (st *State) ResumeTransactions() error {
	runner, closer := st.database.TransactionRunner()
	defer closer()
	return runner.ResumeTransactions()
}

// MaybePruneTransactions removes data for completed transactions.
func (st *State) MaybePruneTransactions() error {
	runner, closer := st.database.TransactionRunner()
	defer closer()
	// Prune txns only when txn count has doubled since last prune.
	return runner.MaybePruneTransactions(2.0)
}

type multiEnvRunner struct {
	rawRunner jujutxn.Runner
	schema    collectionSchema
	envUUID   string
}

// RunTransaction is part of the jujutxn.Runner interface. Operations
// that affect multi-environment collections will be modified in-place
// to ensure correct interaction with these collections.
func (r *multiEnvRunner) RunTransaction(ops []txn.Op) error {
	ops, err := r.updateOps(ops)
	if err != nil {
		return errors.Trace(err)
	}
	return r.rawRunner.RunTransaction(ops)
}

// Run is part of the jujutxn.Runner interface. Operations returned by
// the given "transactions" function that affect multi-environment
// collections will be modified in-place to ensure correct interaction
// with these collections.
func (r *multiEnvRunner) Run(transactions jujutxn.TransactionSource) error {
	return r.rawRunner.Run(func(attempt int) ([]txn.Op, error) {
		ops, err := transactions(attempt)
		if err != nil {
			// Don't use Trace here as jujutxn doens't use juju/errors
			// and won't deal correctly with some returned errors.
			return nil, err
		}
		ops, err = r.updateOps(ops)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return ops, nil
	})
}

// ResumeTransactions is part of the jujutxn.Runner interface.
func (r *multiEnvRunner) ResumeTransactions() error {
	return r.rawRunner.ResumeTransactions()
}

// MaybePruneTransactions is part of the jujutxn.Runner interface.
func (r *multiEnvRunner) MaybePruneTransactions(pruneFactor float32) error {
	return r.rawRunner.MaybePruneTransactions(pruneFactor)
}

func (r *multiEnvRunner) updateOps(ops []txn.Op) ([]txn.Op, error) {
	var referencesEnviron bool
	var insertsEnvironSpecificDocs bool
	for i, op := range ops {
		info, found := r.schema[op.C]
		if !found {
			return nil, errors.Errorf("forbidden transaction: references unknown collection %q", op.C)
		}
		if info.rawAccess {
			return nil, errors.Errorf("forbidden transaction: references raw-access collection %q", op.C)
		}
		if !info.global {
			// TODO(fwereade): this interface implies we're returning a copy
			// of the transactions -- as I think we should be -- rather than
			// rewriting them in place (which IMO breaks client expectations
			// pretty hard, not to mention rendering us unable to accept any
			// structs passed by value, or which lack an env-uuid field).
			//
			// The counterargument is that it's convenient to use rewritten
			// docs directly to construct entities; I think that's suboptimal,
			// because the cost of a DB read to just grab the actual data pales
			// in the face of the transaction operation itself, and it's a
			// small price to pay for a safer implementation.
			var docID interface{}
			if id, ok := op.Id.(string); ok {
				docID = addEnvUUID(r.envUUID, id)
				ops[i].Id = docID
			} else {
				docID = op.Id
			}
			if op.Insert != nil {
				var err error
				switch doc := op.Insert.(type) {
				case bson.D:
					ops[i].Insert, err = r.updateBsonD(doc, docID)
				case bson.M:
					err = r.updateBsonM(doc, docID)
				case map[string]interface{}:
					err = r.updateBsonM(bson.M(doc), docID)
				default:
					err = r.updateStruct(doc, docID)
				}
				if err != nil {
					return nil, errors.Annotatef(err, "cannot insert into %q", op.C)
				}
				if !info.insertWithoutEnvironment {
					insertsEnvironSpecificDocs = true
				}
			}
		}
		if op.C == environmentsC {
			if op.Id == r.envUUID {
				referencesEnviron = true
			}
		}
	}
	if insertsEnvironSpecificDocs && !referencesEnviron {
		// TODO(fwereade): This serializes a large proportion of operations
		// that could otherwise run in parallel. it's quite nice to be able
		// to run more than one transaction per environment at once...
		//
		// Consider representing environ life with a collection of N docs,
		// and selecting different ones per transaction, so as to claw back
		// parallelism of up to N. (Environ dying would update all docs and
		// thus end up serializing everything, but at least we get some
		// benefits for the bulk of an environment's lifetime.)
		ops = append(ops, assertEnvAliveOp(r.envUUID))
	}
	logger.Tracef("rewrote transaction: %#v", ops)
	return ops, nil
}

func assertEnvAliveOp(envUUID string) txn.Op {
	return txn.Op{
		C:      environmentsC,
		Id:     envUUID,
		Assert: isEnvAliveDoc,
	}
}

func (r *multiEnvRunner) updateBsonD(doc bson.D, docID interface{}) (bson.D, error) {
	idSeen := false
	envUUIDSeen := false
	for i, elem := range doc {
		switch elem.Name {
		case "_id":
			idSeen = true
			doc[i].Value = docID
		case "env-uuid":
			envUUIDSeen = true
			if elem.Value != r.envUUID {
				return nil, errors.Errorf(`bad "env-uuid" value: expected %s, got %s`, r.envUUID, elem.Value)
			}
		}
	}
	if !idSeen {
		doc = append(doc, bson.DocElem{"_id", docID})
	}
	if !envUUIDSeen {
		doc = append(doc, bson.DocElem{"env-uuid", r.envUUID})
	}
	return doc, nil
}

func (r *multiEnvRunner) updateBsonM(doc bson.M, docID interface{}) error {
	idSeen := false
	envUUIDSeen := false
	for key, value := range doc {
		switch key {
		case "_id":
			idSeen = true
			doc[key] = docID
		case "env-uuid":
			envUUIDSeen = true
			if value != r.envUUID {
				return errors.Errorf(`bad "env-uuid" value: expected %s, got %s`, r.envUUID, value)
			}
		}
	}
	if !idSeen {
		doc["_id"] = docID
	}
	if !envUUIDSeen {
		doc["env-uuid"] = r.envUUID
	}
	return nil
}

func (r *multiEnvRunner) updateStruct(doc, docID interface{}) error {
	v := reflect.ValueOf(doc)
	t := v.Type()

	if t.Kind() == reflect.Ptr {
		v = v.Elem()
		t = v.Type()
	}

	if t.Kind() != reflect.Struct {
		return errors.Errorf("unknown type %s", t)
	}

	envUUIDSeen := false
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		var err error
		switch f.Tag.Get("bson") {
		case "_id":
			err = r.updateStructField(v, f.Name, docID, overrideField)
		case "env-uuid":
			err = r.updateStructField(v, f.Name, r.envUUID, fieldMustMatch)
			envUUIDSeen = true
		}
		if err != nil {
			return errors.Trace(err)
		}
	}
	if !envUUIDSeen {
		return errors.Errorf(`struct lacks field with bson:"env-uuid" tag`)
	}
	return nil
}

const overrideField = "override"
const fieldMustMatch = "mustmatch"

func (r *multiEnvRunner) updateStructField(v reflect.Value, name string, newValue interface{}, updateType string) error {
	fv := v.FieldByName(name)
	if fv.Interface() != newValue {
		if updateType == fieldMustMatch && fv.String() != "" {
			return errors.Errorf("bad %q field value: expected %s, got %s", name, newValue, fv.String())
		}
		if fv.CanSet() {
			fv.Set(reflect.ValueOf(newValue))
		} else {
			return errors.Errorf("cannot set %q field: struct passed by value", name)
		}
	}
	return nil
}

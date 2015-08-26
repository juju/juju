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

// readTxnRevno is a convenience method delegating to the state's Database.
func (st *State) readTxnRevno(collectionName string, id interface{}) (int64, error) {
	collection, closer := st.database.GetCollection(collectionName)
	defer closer()
	query := collection.FindId(id).Select(bson.D{{"txn-revno", 1}})
	var result struct {
		TxnRevno int64 `bson:"txn-revno"`
	}
	err := query.One(&result)
	return result.TxnRevno, errors.Trace(err)
}

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

// updateOps modifies the Insert and Update fields in a slice of
// txn.Ops to ensure they are multi-environment safe where possible.
//
// Note that the input slice is actually modified in-place (but see
// TODO below).
func (r *multiEnvRunner) updateOps(ops []txn.Op) ([]txn.Op, error) {
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
				docID = ensureEnvUUID(r.envUUID, id)
				ops[i].Id = docID
			} else {
				docID = op.Id
			}
			if op.Insert != nil {
				newInsert, err := r.mungeInsert(op.Insert, docID)
				if err != nil {
					return nil, errors.Annotatef(err, "cannot insert into %q", op.C)
				}
				ops[i].Insert = newInsert
			}
			if op.Update != nil {
				newUpdate, err := r.mungeUpdate(op.Update, docID)
				if err != nil {
					return nil, errors.Annotatef(err, "cannot update %q", op.C)
				}
				ops[i].Update = newUpdate
			}
		}
	}
	logger.Tracef("rewrote transaction: %#v", ops)
	return ops, nil
}

// mungeInsert takes the value of an txn.Op Insert field and modifies
// it to be multi-environment safe, returning the modified document.
func (r *multiEnvRunner) mungeInsert(doc interface{}, docID interface{}) (interface{}, error) {
	switch doc := doc.(type) {
	case bson.D:
		return r.mungeBsonD(doc, docID)
	case bson.M:
		return doc, r.mungeBsonM(doc, docID)
	case map[string]interface{}:
		return doc, r.mungeBsonM(bson.M(doc), docID)
	default:
		return doc, r.mungeStruct(doc, docID)
	}
}

// mungeBsonD takes the value of a txn.Op field expressed as a bson.D
// and modifies it to be multi-environment safe.
func (r *multiEnvRunner) mungeBsonD(doc bson.D, docID interface{}) (bson.D, error) {
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

// mungeBsonM takes the value of a txn.Op field expressed as a bson.M
// and modifies it to be multi-environment safe. The map is modified
// in-place.
func (r *multiEnvRunner) mungeBsonM(doc bson.M, docID interface{}) error {
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

// mungeStruct takes the value of a txn.Op field expressed as some
// struct and modifies it to be multi-environment safe. The struct is
// modified in-place.
func (r *multiEnvRunner) mungeStruct(doc, docID interface{}) error {
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
			err = r.mungeStructField(v, f.Name, docID, overrideField)
		case "env-uuid":
			err = r.mungeStructField(v, f.Name, r.envUUID, fieldMustMatch)
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

// mungeStructFIeld updates the field of a struct to a new value. If
// updateType is overrideField == fieldMustMatch then the field must
// match the value given, if present.
func (r *multiEnvRunner) mungeStructField(v reflect.Value, name string, newValue interface{}, updateType string) error {
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

// mungeInsert takes the value of an txn.Op Update field and modifies
// it to be multi-environment safe, returning the modified document.
func (r *multiEnvRunner) mungeUpdate(updateDoc, docID interface{}) (interface{}, error) {
	switch doc := updateDoc.(type) {
	case bson.D:
		return r.mungeBsonDUpdate(doc, docID)
	case bson.M:
		return r.mungeBsonMUpdate(doc, docID)
	default:
		return nil, errors.Errorf("don't know how to handle %T", updateDoc)
	}
}

// mungeBsonDUpdate modifies Update field values expressed as a bson.D
// and attempts to make them multi-environment safe.
func (r *multiEnvRunner) mungeBsonDUpdate(updateDoc bson.D, docID interface{}) (bson.D, error) {
	outDoc := make(bson.D, 0, len(updateDoc))
	for _, elem := range updateDoc {
		if elem.Name == "$set" {
			// TODO(mjs) - only worry about structs for now. This is
			// enough to fix LP #1474606 and a more extensive change
			// to simplify the multi-env txn layer and correctly
			// handle all cases is coming soon.
			newSetDoc, err := r.mungeStructOnly(elem.Value, docID)
			if err != nil {
				return nil, errors.Trace(err)
			}
			outDoc = append(outDoc, bson.DocElem{elem.Name, newSetDoc})
		} else {
			outDoc = append(outDoc, elem)
		}
	}
	return outDoc, nil
}

// mungeBsonMUpdate modifies Update field values expressed as a bson.M
// and attempts to make them multi-environment safe.
func (r *multiEnvRunner) mungeBsonMUpdate(updateDoc bson.M, docID interface{}) (bson.M, error) {
	outDoc := make(bson.M)
	for name, elem := range updateDoc {
		if name == "$set" {
			// TODO(mjs) - as above.
			newSetDoc, err := r.mungeStructOnly(elem, docID)
			if err != nil {
				return nil, errors.Trace(err)
			}
			outDoc[name] = newSetDoc
		} else {
			outDoc[name] = elem
		}
	}
	return outDoc, nil
}

// mungeStructOnly modifies the input document to address
// multi-environment concerns, but only if it's a struct.
func (r *multiEnvRunner) mungeStructOnly(doc interface{}, docID interface{}) (interface{}, error) {
	switch doc := doc.(type) {
	case bson.D, bson.M, map[string]interface{}:
		return doc, nil
	default:
		return doc, r.mungeStruct(doc, docID)
	}
}

// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"reflect"

	jujutxn "github.com/juju/txn"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"
)

// txnRunner returns a jujutxn.Runner instance.
//
// If st.transactionRunner is non-nil, then that will be
// returned and the session argument will be ignored. This
// is the case in tests only, when we want to test concurrent
// operations.
//
// If st.transactionRunner is nil, then we create a new
// transaction runner with the provided session and return
// that.
func (st *State) txnRunner(session *mgo.Session) jujutxn.Runner {
	if st.transactionRunner != nil {
		return st.transactionRunner
	}
	return newMultiEnvRunner(st.EnvironUUID(), st.db.With(session))
}

// runTransaction is a convenience method delegating to transactionRunner.
func (st *State) runTransaction(ops []txn.Op) error {
	session := st.db.Session.Copy()
	defer session.Close()
	return st.txnRunner(session).RunTransaction(ops)
}

// run is a convenience method delegating to transactionRunner.
func (st *State) run(transactions jujutxn.TransactionSource) error {
	session := st.db.Session.Copy()
	defer session.Close()
	return st.txnRunner(session).Run(transactions)
}

// ResumeTransactions resumes all pending transactions.
func (st *State) ResumeTransactions() error {
	session := st.db.Session.Copy()
	defer session.Close()
	return st.txnRunner(session).ResumeTransactions()
}

func newMultiEnvRunner(envUUID string, db *mgo.Database) jujutxn.Runner {
	return &multiEnvRunner{
		rawRunner: jujutxn.NewRunner(jujutxn.RunnerParams{Database: db}),
		envUUID:   envUUID,
	}
}

type multiEnvRunner struct {
	rawRunner jujutxn.Runner
	envUUID   string
}

// RunTransaction is part of the jujutxn.Runner interface. Operations
// that affect multi-environment collections will be modified in-place
// to ensure correct interaction with these collections.
func (r *multiEnvRunner) RunTransaction(ops []txn.Op) error {
	r.updateOps(ops)
	return r.rawRunner.RunTransaction(ops)
}

// Run is part of the jujutxn.Run interface. Operations returned by
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
		r.updateOps(ops)
		return ops, nil
	})
}

// Run is part of the jujutxn.Run interface.
func (r *multiEnvRunner) ResumeTransactions() error {
	return r.rawRunner.ResumeTransactions()
}

func (r *multiEnvRunner) updateOps(ops []txn.Op) {
	for i, op := range ops {
		if multiEnvCollections.Contains(op.C) {
			var docID interface{}
			if id, ok := op.Id.(string); ok {
				docID = addEnvUUID(r.envUUID, id)
				ops[i].Id = docID
			} else {
				docID = op.Id
			}

			if op.Insert != nil {
				switch doc := op.Insert.(type) {
				case bson.D:
					ops[i].Insert = r.updateBsonD(doc, docID, op.C)
				case bson.M:
					r.updateBsonM(doc, docID, op.C)
				default:
					r.updateStruct(doc, docID, op.C)
				}
			}
		}
	}
}

func (r *multiEnvRunner) updateBsonD(doc bson.D, docID interface{}, collName string) bson.D {
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
				panic(fmt.Sprintf("environment UUID for document to insert into "+
					"%s does not match state", collName))
			}
		}
	}
	if !idSeen {
		doc = append(doc, bson.DocElem{"_id", docID})
	}
	if !envUUIDSeen {
		doc = append(doc, bson.DocElem{"env-uuid", r.envUUID})
	}
	return doc
}

func (r *multiEnvRunner) updateBsonM(doc bson.M, docID interface{}, collName string) {
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
				panic(fmt.Sprintf("environment UUID for document to insert into "+
					"%s does not match state", collName))
			}
		}
	}
	if !idSeen {
		doc["_id"] = docID
	}
	if !envUUIDSeen {
		doc["env-uuid"] = r.envUUID
	}
}

func (r *multiEnvRunner) updateStruct(doc, docID interface{}, collName string) {
	v := reflect.ValueOf(doc)
	t := v.Type()

	if t.Kind() == reflect.Ptr {
		v = v.Elem()
		t = v.Type()
	}

	if t.Kind() == reflect.Struct {
		envUUIDSeen := false
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			switch f.Tag.Get("bson") {
			case "_id":
				r.updateStructField(v, f.Name, docID, collName, overrideField)
			case "env-uuid":
				r.updateStructField(v, f.Name, r.envUUID, collName, fieldMustMatch)
				envUUIDSeen = true
			}
		}
		if !envUUIDSeen {
			panic(fmt.Sprintf("struct for insert into %s is missing an env-uuid field", collName))
		}
	}

}

const overrideField = "override"
const fieldMustMatch = "mustmatch"

func (r *multiEnvRunner) updateStructField(v reflect.Value, name string, newValue interface{}, collName, updateType string) {
	fv := v.FieldByName(name)
	if fv.Interface() != newValue {
		if updateType == fieldMustMatch && fv.String() != "" {
			panic(fmt.Sprintf("%s for insert into %s does not match expected value",
				name, collName))
		}
		if fv.CanSet() {
			fv.Set(reflect.ValueOf(newValue))
		} else {
			panic(fmt.Sprintf("struct for insert into %s requires "+
				"%s change but was passed by value", collName, name))
		}
	}
}

// rawTxnRunner returns a transaction runner that won't perform
// automatic addition of environment UUIDs into transaction
// operations, even for collections that contain documents for
// multiple environments. It should be used rarely.
func (st *State) rawTxnRunner(session *mgo.Session) jujutxn.Runner {
	if st.transactionRunner != nil {
		return getRawRunner(st.transactionRunner)
	}
	return jujutxn.NewRunner(jujutxn.RunnerParams{
		Database: st.db.With(session),
	})
}

// runRawTransaction is a convenience method that will run a single
// transaction using a "raw" transaction runner, as returned by
// rawTxnRunner.
func (st *State) runRawTransaction(ops []txn.Op) error {
	session := st.db.Session.Copy()
	defer session.Close()
	runner := st.rawTxnRunner(session)
	return runner.RunTransaction(ops)
}

// getRawRunner returns the underlying "raw" transaction runner from
// the passed transaction runner.
func getRawRunner(runner jujutxn.Runner) jujutxn.Runner {
	if runner, ok := runner.(*multiEnvRunner); ok {
		return runner.rawRunner
	}
	return runner
}

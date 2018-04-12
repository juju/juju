// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"runtime/debug"
	"strings"
	"time"

	"github.com/juju/errors"
	jujutxn "github.com/juju/txn"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"
)

func readTxnRevno(db Database, collectionName string, id interface{}) (int64, error) {
	collection, closer := db.GetCollection(collectionName)
	defer closer()
	query := collection.FindId(id).Select(bson.D{{"txn-revno", 1}})
	var result struct {
		TxnRevno int64 `bson:"txn-revno"`
	}
	err := query.One(&result)
	return result.TxnRevno, errors.Trace(err)
}

func (st *State) runRawTransaction(ops []txn.Op) error {
	return st.database.RunRawTransaction(ops)
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
	// Prune txns when txn count has increased by 10% since last prune.
	return runner.MaybePruneTransactions(jujutxn.PruneOptions{
		PruneFactor:        1.1,
		MinNewTransactions: 1000,
		MaxNewTransactions: 100000,
		MaxTime:            time.Now().Add(-time.Hour),
	})
}

type multiModelRunner struct {
	rawRunner jujutxn.Runner
	schema    collectionSchema
	modelUUID string
}

func shortStack() string {
	var rv []string
	for _, line := range strings.Split(string(debug.Stack()), "\n") {
		if len(line) > 0 && line[0] == '\t' {
			rv = append(rv, strings.Split(line[1:], " ")[0])
		}
	}
	// We definitely have at least 3 objects in rv - debug.Stack(), this function and the one that called it.
	return strings.Join(rv[3:], " ")
}

var seenShortStacks = make(map[string]bool)

// RunTransaction is part of the jujutxn.Runner interface. Operations
// that affect multi-model collections will be modified to
// ensure correct interaction with these collections.
func (r *multiModelRunner) RunTransaction(ops []txn.Op) error {
	if len(ops) == 0 {
		stack := shortStack()
		// It is a warning that should be reported to us, but we definitely
		// don't want to clutter up logs so we'll only write it once.
		if !seenShortStacks[stack] {
			seenShortStacks[stack] = true
			logger.Warningf("Running no-op transaction - called by %s", stack)
		}
	}
	newOps, err := r.updateOps(ops)
	if err != nil {
		return errors.Trace(err)
	}
	return r.rawRunner.RunTransaction(newOps)
}

// Run is part of the jujutxn.Runner interface. Operations returned by
// the given "transactions" function that affect multi-model
// collections will be modified to ensure correct interaction with
// these collections.
func (r *multiModelRunner) Run(transactions jujutxn.TransactionSource) error {
	return r.rawRunner.Run(func(attempt int) ([]txn.Op, error) {
		ops, err := transactions(attempt)
		if err != nil {
			// Don't use Trace here as jujutxn doens't use juju/errors
			// and won't deal correctly with some returned errors.
			return nil, err
		}
		newOps, err := r.updateOps(ops)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return newOps, nil
	})
}

// ResumeTransactions is part of the jujutxn.Runner interface.
func (r *multiModelRunner) ResumeTransactions() error {
	return r.rawRunner.ResumeTransactions()
}

// MaybePruneTransactions is part of the jujutxn.Runner interface.
func (r *multiModelRunner) MaybePruneTransactions(opts jujutxn.PruneOptions) error {
	return r.rawRunner.MaybePruneTransactions(opts)
}

// updateOps modifies the Insert and Update fields in a slice of
// txn.Ops to ensure they are multi-model safe where
// possible. The returned []txn.Op is a new copy of the input (with
// changes).
func (r *multiModelRunner) updateOps(ops []txn.Op) ([]txn.Op, error) {
	var outOps []txn.Op
	for _, op := range ops {
		collInfo, found := r.schema[op.C]
		if !found {
			return nil, errors.Errorf("forbidden transaction: references unknown collection %q", op.C)
		}
		if collInfo.rawAccess {
			return nil, errors.Errorf("forbidden transaction: references raw-access collection %q", op.C)
		}
		outOp := op
		if !collInfo.global {
			outOp.Id = ensureModelUUIDIfString(r.modelUUID, op.Id)
			if op.Insert != nil {
				newInsert, err := mungeDocForMultiEnv(op.Insert, r.modelUUID, modelUUIDRequired)
				if err != nil {
					return nil, errors.Annotatef(err, "cannot insert into %q", op.C)
				}
				outOp.Insert = newInsert
			}
			if op.Update != nil {
				newUpdate, err := r.mungeUpdate(op.Update)
				if err != nil {
					return nil, errors.Annotatef(err, "cannot update %q", op.C)
				}
				outOp.Update = newUpdate
			}
		}
		outOps = append(outOps, outOp)
	}
	return outOps, nil
}

// mungeUpdate takes the value of an txn.Op Update field and modifies
// it to be multi-model safe, returning the modified document.
func (r *multiModelRunner) mungeUpdate(updateDoc interface{}) (interface{}, error) {
	switch doc := updateDoc.(type) {
	case bson.D:
		return r.mungeBsonDUpdate(doc)
	case bson.M:
		return r.mungeBsonMUpdate(doc)
	default:
		return nil, errors.Errorf("don't know how to handle %T", updateDoc)
	}
}

// mungeBsonDUpdate modifies a txn.Op's Update field values expressed
// as a bson.D and attempts to make it multi-model safe.
//
// Currently, only $set operations are munged.
func (r *multiModelRunner) mungeBsonDUpdate(updateDoc bson.D) (bson.D, error) {
	outDoc := make(bson.D, 0, len(updateDoc))
	for _, elem := range updateDoc {
		if elem.Name == "$set" {
			newSetDoc, err := mungeDocForMultiEnv(elem.Value, r.modelUUID, 0)
			if err != nil {
				return nil, errors.Trace(err)
			}
			elem = bson.DocElem{elem.Name, newSetDoc}
		}
		outDoc = append(outDoc, elem)
	}
	return outDoc, nil
}

// mungeBsonMUpdate modifies a txn.Op's Update field values expressed
// as a bson.M and attempts to make it multi-model safe.
//
// Currently, only $set operations are munged.
func (r *multiModelRunner) mungeBsonMUpdate(updateDoc bson.M) (bson.M, error) {
	outDoc := make(bson.M)
	for name, elem := range updateDoc {
		if name == "$set" {
			var err error
			elem, err = mungeDocForMultiEnv(elem, r.modelUUID, 0)
			if err != nil {
				return nil, errors.Trace(err)
			}
		}
		outDoc[name] = elem
	}
	return outDoc, nil
}

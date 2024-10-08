// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"sort"
	
	"github.com/juju/errors"
	"github.com/juju/mgo/v3/txn"
	jujutxn "github.com/juju/txn/v3"
)

// ModelOperation is a high-level model operation,
// encapsulating the logic required to apply a change
// to a model.
type ModelOperation interface {
	// Build builds the low-level database transaction operations required
	// to apply the change. If the transaction operations fail (e.g. due
	// to concurrent changes), then Build may be called again. The attempt
	// number, starting at zero, is passed in.
	//
	// Build is treated as a jujutxn.TransactionSource, so the errors
	// in the jujutxn package may be returned by Build to influence
	// transaction execution.
	Build(attempt int) ([]txn.Op, error)

	// Done is called after the operation is run, whether it succeeds or
	// not. The result of running the operation is passed in, and the Done
	// method may annotate the error; or run additional, non-transactional
	// logic depending on the outcome.
	Done(error) error
}

// modelOperationFunc is an adapter for composing a txn builder and done
// function/closure into a type that implements ModelOperation.
type modelOperationFunc struct {
	buildFn func(attempt int) ([]txn.Op, error)
	doneFn  func(err error) error
}

// Build implements ModelOperation.
func (mof modelOperationFunc) Build(attempt int) ([]txn.Op, error) {
	if mof.buildFn == nil {
		return nil, nil
	}
	return mof.buildFn(attempt)
}

// Done implements ModelOperation.
func (mof modelOperationFunc) Done(err error) error {
	if mof.doneFn == nil {
		return err
	}
	return mof.doneFn(err)
}

// ComposeModelOperations returns a ModelOperation which composes multiple
// ModelOperations and executes them in a single transaction. If any of the
// provided ModelOperations are nil, they will be automatically ignored.
func ComposeModelOperations(modelOps ...ModelOperation) ModelOperation {
	return modelOperationFunc{
		buildFn: func(attempt int) ([]txn.Op, error) {
			var ops []txn.Op
			for _, modelOp := range modelOps {
				if modelOp == nil {
					continue
				}
				childOps, err := modelOp.Build(attempt)
				if err != nil && err != jujutxn.ErrNoOperations {
					return nil, errors.Trace(err)
				}
				ops = append(ops, childOps...)
			}
			return ops, nil
		},
		doneFn: func(err error) error {
			// Unfortunately, we cannot detect the exact
			// ModelOperation that caused the error. For now, just
			// pass the error to each done method and ignore the
			// return value. Then, return the original error back
			// to the caller.
			//
			// A better approach would be to compare each Done
			// method's return value to the original error and
			// record it if different. Unfortunately, we don't have
			// a multi-error type to represent a collection of
			// errors.
			for _, modelOp := range modelOps {
				if modelOp == nil {
					continue
				}
				_ = modelOp.Done(err)
			}

			return err
		},
	}
}

// ApplyOperation applies a given ModelOperation to the model.
//
// NOTE(axw) when all model-specific types and methods are moved
// to Model, then this should move also.
func (st *State) ApplyOperation(op ModelOperation) error {
	err := st.db().Run(op.Build)
	return op.Done(err)
}

// sortRemoveOpsLast re-orders a slice of transaction operations so that any
// document removals occur at the end of the slice.
// This is important for server-side transactions because of two execution
// characteristics:
// 1. All assertions are verified first.
// 2. We can read our own writes inside the transaction.
// This means it is possible for a removal and a subsequent update operation
// on the same document to pass the assertions, then fail with "not found" upon
// actual processing.
// Legacy client-side transactions do not exhibit this behaviour.
func sortRemoveOpsLast(ops []txn.Op) {
	sort.Slice(ops, func(i, j int) bool {
		return !ops[i].Remove && ops[j].Remove
	})
}

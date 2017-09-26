// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"gopkg.in/mgo.v2/txn"
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

// ApplyOperation applies a given ModelOperation to the model.
//
// NOTE(axw) when all model-specific types and methods are moved
// to Model, then this should move also.
func (st *State) ApplyOperation(op ModelOperation) error {
	err := st.db().Run(op.Build)
	return op.Done(err)
}

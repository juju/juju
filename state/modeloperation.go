// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

type Op struct {
}

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
	Build(attempt int) ([]Op, error)

	// Done is called after the operation is run, whether it succeeds or
	// not. The result of running the operation is passed in, and the Done
	// method may annotate the error; or run additional, non-transactional
	// logic depending on the outcome.
	Done(error) error
}

// modelOperationFunc is an adaptor for composing a txn builder and done
// function/closure into a type that implements ModelOperation.
type modelOperationFunc struct {
	buildFn func(attempt int) ([]Op, error)
	doneFn  func(err error) error
}

// Build implements ModelOperation.
func (mof modelOperationFunc) Build(attempt int) ([]Op, error) {
	return nil, nil
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
		buildFn: func(attempt int) ([]Op, error) {
			return nil, nil
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
	return op.Done(nil)
}

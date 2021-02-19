// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package operation

import (
	"fmt"
)

type noOpFinishUpgradeSeries struct {
	Operation
}

// String is part of the Operation interface.
func (op *noOpFinishUpgradeSeries) String() string {
	return fmt.Sprint("complete upgrade series")
}

// Commit is part of the Operation interface.
func (op *noOpFinishUpgradeSeries) Commit(state State) (*State, error) {
	// make no change to state
	return &state, nil
}

// WrappedOperation is part of the WrappedOperation interface.
func (op *noOpFinishUpgradeSeries) WrappedOperation() Operation {
	return op.Operation
}

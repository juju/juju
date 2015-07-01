// Copyright 2014-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package operation

import (
	"fmt"
)

type skipOperation struct {
	Operation
}

// String is part of the Operation interface.
func (op *skipOperation) String() string {
	return fmt.Sprintf("skip %s", op.Operation)
}

// NeedsGlobalMachineLock is part of the Operation interface.
func (op *skipOperation) NeedsGlobalMachineLock() bool {
	return false
}

// Prepare is part of the Operation interface.
func (op *skipOperation) Prepare(state State) (*State, error) {
	return nil, ErrSkipExecute
}

// Execute is part of the Operation interface.
func (op *skipOperation) Execute(state State) (*State, error) {
	return nil, ErrSkipExecute
}

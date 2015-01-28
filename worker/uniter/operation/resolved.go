// Copyright 2014-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package operation

import (
	"fmt"

	"github.com/juju/errors"
)

type resolvedOperation struct {
	Operation
	callbacks Callbacks
}

// String is part of the Operation interface.
func (op *resolvedOperation) String() string {
	return fmt.Sprintf("clear resolved flag and %s", op.Operation)
}

// Prepare is part of the Operation interface.
func (op *resolvedOperation) Prepare(state State) (*State, error) {
	if err := op.callbacks.ClearResolvedFlag(); err != nil {
		return nil, errors.Trace(err)
	}
	return op.Operation.Prepare(state)
}

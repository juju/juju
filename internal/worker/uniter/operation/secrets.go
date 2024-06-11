// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package operation

import (
	"context"
	"fmt"

	"github.com/juju/errors"
)

type noOpSecretsRemoved struct {
	Operation
	uris      []string
	callbacks Callbacks
}

// String is part of the Operation interface.
func (op *noOpSecretsRemoved) String() string {
	return fmt.Sprintf("process removed secrets: %v", op.uris)
}

// Commit is part of the Operation interface.
func (op *noOpSecretsRemoved) Commit(ctx context.Context, state State) (*State, error) {
	if err := op.callbacks.SecretsRemoved(ctx, op.uris); err != nil {
		return nil, errors.Trace(err)
	}
	// make no change to state
	return &state, nil
}

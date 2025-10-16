// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package operation

import (
	"context"
	"fmt"
	"strings"

	"github.com/juju/errors"
)

type noOpSecretsRemoved struct {
	Operation
	deletedRevisions         map[string][]int
	deletedObsoleteRevisions map[string][]int
	callbacks                Callbacks
}

// String is part of the Operation interface.
func (op *noOpSecretsRemoved) String() string {
	var lines []string
	for uri, revs := range op.deletedRevisions {
		lines = append(lines, fmt.Sprintf("%s: %v", uri, revs))
	}
	for uri, revs := range op.deletedObsoleteRevisions {
		lines = append(lines, fmt.Sprintf("%s: %v", uri, revs))
	}
	return fmt.Sprintf("remove deleted secrets:\n%v", strings.Join(lines, "\n"))
}

// Commit is part of the Operation interface.
func (op *noOpSecretsRemoved) Commit(ctx context.Context, state State) (*State, error) {
	err := op.callbacks.SecretsRemoved(ctx, op.deletedRevisions, op.deletedObsoleteRevisions)
	if err != nil {
		return nil, errors.Trace(err)
	}
	// make no change to state
	return &state, nil
}

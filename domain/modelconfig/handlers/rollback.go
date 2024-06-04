// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package handlers

import (
	"context"
	"errors"
)

// RollbackFunc defines a function used to rollback
// an OnSave operation.
type RollbackFunc func(context.Context) error

func noopRollback(_ context.Context) error {
	return nil
}

// Rollbacks represents a list of functions to run
// to rollback the changes made by an OnSave handler.
type Rollbacks []RollbackFunc

// Rollback runs each rollback func, accumulating any errors encountered.
func (rb Rollbacks) Rollback(ctx context.Context) error {
	var re []error
	for _, r := range rb {
		if err := r(ctx); err != nil {
			re = append(re, err)
		}
	}
	return errors.Join(re...)
}

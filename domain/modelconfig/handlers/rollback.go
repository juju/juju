// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package handlers

import (
	"context"
	"strings"
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

type rollbackError []error

func (re rollbackError) Error() string {
	errStrings := make([]string, len(re))
	for i, err := range re {
		errStrings[i] = err.Error()
	}
	return strings.Join(errStrings, "\n")
}

// Rollback runs each rollback func, accumulating any errors encountered.
func (rb Rollbacks) Rollback(ctx context.Context) error {
	var re rollbackError
	for _, r := range rb {
		if err := r(ctx); err != nil {
			re = append(re, err)
		}
	}
	if len(re) == 0 {
		return nil
	}
	return re
}

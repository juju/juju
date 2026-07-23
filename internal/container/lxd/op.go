// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd

type lxdOperation interface {
	Wait() error
}

// WaitOp waits for the operation to complete if err is nil.
func WaitOp(op lxdOperation, err error) error {
	if err != nil {
		return err
	}
	return op.Wait()
}

// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

// FIXME(nvinuesa): This `ValueResource` should be removed and they should not
// be registered.
// ValueResource is a Resource with a no-op Kill and Wait method, containing an
// T value.
type ValueResource[T any] struct {
	Value T
}

// Kill implements worker.Worker. interface.
func (ValueResource[T]) Kill() {}

// Wait implements worker.Worker. interface.
func (ValueResource[T]) Wait() error {
	return nil
}

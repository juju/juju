// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machinelock

import "github.com/juju/mutex/v2"

func NewTestLock(config Config, acquire func(mutex.Spec) (mutex.Releaser, error)) (*lock, error) {
	lock, err := New(config)
	if lock != nil {
		lock.acquire = acquire
	}
	return lock, err
}

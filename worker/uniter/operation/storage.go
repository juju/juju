// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package operation

import (
	"fmt"

	"github.com/juju/names"
)

type updateStorage struct {
	tags []names.StorageTag

	storageUpdater StorageUpdater

	DoesNotRequireMachineLock
}

// String is part of the Operation interface.
func (u *updateStorage) String() string {
	return fmt.Sprintf("update storage %v", u.tags)
}

// Prepare does nothing; it is part of the Operation interface.
func (u *updateStorage) Prepare(_ State) (*State, error) {
	return nil, nil
}

// Execute ensures the operation's storage tags are known and tracked. This
// doesn't directly change any persistent state; state is updated when
// storage hooks are executed.
//
// Execute is part of the Operation interface.
func (u *updateStorage) Execute(_ State) (*State, error) {
	return nil, u.storageUpdater.UpdateStorage(u.tags)
}

// Commit does nothing; it is part of the Operation interface.
func (u *updateStorage) Commit(_ State) (*State, error) {
	return nil, nil
}

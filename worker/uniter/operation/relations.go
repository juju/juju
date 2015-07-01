// Copyright 2014-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package operation

import (
	"fmt"
)

type updateRelations struct {
	ids []int

	callbacks Callbacks

	DoesNotRequireMachineLock
}

// String is part of the Operation interface.
func (ur *updateRelations) String() string {
	return fmt.Sprintf("update relations %v", ur.ids)
}

// Prepare does nothing.
// Prepare is part of the Operation interface.
func (ur *updateRelations) Prepare(_ State) (*State, error) {
	return nil, nil
}

// Execute ensures the operation's relation ids are known and tracked. This
// doesn't directly change any persistent state.
// Execute is part of the Operation interface.
func (ur *updateRelations) Execute(_ State) (*State, error) {
	return nil, ur.callbacks.UpdateRelations(ur.ids)
}

// Commit does nothing.
// Commit is part of the Operation interface.
func (ur *updateRelations) Commit(_ State) (*State, error) {
	return nil, nil
}

// Copyright 2014-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package operation

import (
	"fmt"
)

type updateRelations struct {
	ids []int

	callbacks Callbacks
}

func (ur *updateRelations) String() string {
	return fmt.Sprintf("update relations %v", ur.ids)
}

func (ur *updateRelations) Prepare(_ State) (*State, error) {
	return nil, nil
}

func (ur *updateRelations) Execute(_ State) (*State, error) {
	return nil, ur.callbacks.UpdateRelations(ur.ids)
}

func (ur *updateRelations) Commit(_ State) (*State, error) {
	return nil, nil
}

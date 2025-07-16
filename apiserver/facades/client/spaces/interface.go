// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package spaces

import (
	"context"
)

// BlockChecker defines the block-checking functionality required by
// the spaces facade. This is implemented by apiserver/common.BlockChecker.
type BlockChecker interface {
	ChangeAllowed(context.Context) error
}

// Constraints defines the methods supported by constraints used in the space context.
type Constraints interface {
	ID() string
}

// Backing describes the state methods used in this package.
type Backing interface {
	// ConstraintsBySpaceName returns constraints found by spaceName.
	ConstraintsBySpaceName(name string) ([]Constraints, error)

	// IsController returns true if this state instance
	// is for the controller model.
	IsController() bool
}

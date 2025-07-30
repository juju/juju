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

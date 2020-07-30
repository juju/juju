// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package persistence

import (
	"github.com/juju/juju/nustate/model"
	"github.com/juju/juju/nustate/persistence/transaction"
)

// An abstraction for a model backing store
type Store interface {
	FindMachinePortRanges(machineID string) (model.MachinePortRanges, error)

	// TODO: use a context.Context with embeddable *Context?
	ApplyTxn(transaction.ModelTxn) (transaction.Context, error)
}

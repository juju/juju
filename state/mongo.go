// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	jujutxn "github.com/juju/txn/v3"

	"github.com/juju/juju/internal/mongo"
)

// environMongo implements state/lease.Mongo to expose environ-filtered mongo
// capabilities to the sub-packages (e.g. lease, macaroonstorage).
type environMongo struct {
	state *State
}

// GetCollection is part of the lease.Mongo interface.
func (m *environMongo) GetCollection(name string) (mongo.Collection, func()) {
	return m.state.db().GetCollection(name)
}

// RunTransaction is part of the lease.Mongo interface.
func (m *environMongo) RunTransaction(buildTxn jujutxn.TransactionSource) error {
	return m.state.db().Run(buildTxn)
}

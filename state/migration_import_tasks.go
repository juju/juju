// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/description/v10"
	"github.com/juju/mgo/v3/txn"
)

// Migration import tasks provide a boundary of isolation between the
// description package and the state package. Input types are modelled as small
// descrete interfaces, that can be composed to provide more functionality.
// Output types, normally a transaction runner can then take the migrated
// description entity as a txn.Op.
//
// The goal of these input tasks are to be moved out of the state package into
// a similar setup as export migrations. That way we can isolate migrations away
// from state and start creating richer types.
//
// Modelling it this way should provide better test coverage and protection
// around state changes.

// TransactionRunner is an in-place usage for running transactions to a
// persistence store.
type TransactionRunner interface {
	RunTransaction([]txn.Op) error
}

// DocModelNamespace takes a document model ID and ensures it has a model id
// associated with the model.
type DocModelNamespace interface {
	DocID(string) string
}

// StateDocumentFactory creates documents that are useful with in the state
// package. In essence this just allows us to model our dependencies correctly
// without having to construct dependencies everywhere.
// Note: we need public methods here because gomock doesn't mock private methods
type StateDocumentFactory interface {
	MakeStatusDoc(description.Status) statusDoc
	MakeStatusOp(string, statusDoc) txn.Op
}

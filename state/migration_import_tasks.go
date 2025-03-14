// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/description/v9"
	"github.com/juju/errors"
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

type stateModelNamspaceShim struct {
	description.Model
	st *State
}

func (s stateModelNamspaceShim) DocID(localID string) string {
	return s.st.docID(localID)
}

// StateDocumentFactory creates documents that are useful with in the state
// package. In essence this just allows us to model our dependencies correctly
// without having to construct dependencies everywhere.
// Note: we need public methods here because gomock doesn't mock private methods
type StateDocumentFactory interface {
	MakeStatusDoc(description.Status) statusDoc
	MakeStatusOp(string, statusDoc) txn.Op
}

// RelationNetworksDescription defines an in-place usage for reading relation networks.
type RelationNetworksDescription interface {
	RelationNetworks() []description.RelationNetwork
}

// RelationNetworksInput describes the input used for migrating relation
// networks.
type RelationNetworksInput interface {
	DocModelNamespace
	RelationNetworksDescription
}

// ImportRelationNetworks describes a way to import relation networks from a
// description.
type ImportRelationNetworks struct{}

// Execute the import on the relation networks description, carefully modelling
// the dependencies we have.
func (ImportRelationNetworks) Execute(src RelationNetworksInput, runner TransactionRunner) error {
	relationNetworks := src.RelationNetworks()
	if len(relationNetworks) == 0 {
		return nil
	}

	ops := make([]txn.Op, len(relationNetworks))
	for i, entity := range relationNetworks {
		docID := src.DocID(entity.ID())
		ops[i] = txn.Op{
			C:      relationNetworksC,
			Id:     docID,
			Assert: txn.DocMissing,
			Insert: relationNetworksDoc{
				Id:          docID,
				RelationKey: entity.RelationKey(),
				CIDRS:       entity.CIDRS(),
			},
		}
	}

	if err := runner.RunTransaction(ops); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package mongo

import (
	"time"

	"github.com/juju/errors"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/nustate/model"
	"github.com/juju/juju/nustate/operation/precondition"
	"github.com/juju/juju/nustate/persistence"
	"github.com/juju/juju/nustate/persistence/transaction"
	"github.com/juju/juju/state"
)

var _ persistence.Store = (*Store)(nil)

type storeModel interface {
	persistOp() []txn.Op
}

type complexOperation interface {
	Changes(transaction.Context) (transaction.ModelTxn, error)
}

// Store implements a persistence layer backed by a mongoDB instance.
type Store struct {
	// TODO: this should be a mongo handle
	db state.Database
}

func (st *Store) FindMachinePortRanges(machineID string) (model.MachinePortRanges, error) {
	// TODO query and populate doc; we can probably wrap a db.Find and
	// unmarshal directly in the doc
	res := &machinePortRangesDoc{
		docExists: true, // doc was found in db
	}
	res.doc.DocID = "fake-it-till-you-make-it"
	res.doc.MachineID = machineID
	res.doc.UnitPortRanges = map[string]unitPortRangesDoc{
		"foo/0": {
			"": []network.PortRange{
				network.MustParsePortRange("1337/tcp"),
			},
		},
	}
	res.doc.TxnRevno = 42

	return res, nil
}

func (st *Store) ApplyTxn(modelTxn transaction.ModelTxn) (transaction.Context, error) {
	var ctx transaction.Context
	defer func(start time.Time) {
		ctx.ElapsedTime = time.Since(start)
	}(time.Now())

	ops, err := st.mapModelTxn(ctx, 0, modelTxn)
	if err != nil {
		return ctx, err
	}

	// TODO: catch assertion errors; bump ctx.Attempt and retry
	return ctx, st.db.RunTransaction(ops)
}

const maxDepth = 128

func (st *Store) mapModelTxn(ctx transaction.Context, depth int, modelTxn transaction.ModelTxn) ([]txn.Op, error) {
	if depth > maxDepth {
		return nil, errors.New("max recursion depth exceeded")
	}
	var out []txn.Op
	for _, txnElem := range modelTxn {
		// Is this a store model?
		if model, ok := txnElem.(storeModel); ok {
			out = append(out, model.persistOp()...)
		} else if operation, ok := txnElem.(complexOperation); ok { // Is it a complex operation?
			nestedModelTxn, err := operation.Changes(ctx)
			if err != nil {
				return nil, err
			}

			nestedOps, err := st.mapModelTxn(ctx, depth+1, nestedModelTxn)
			if err != nil {
				return nil, err
			}

			out = append(out, nestedOps...)
		} else { // It has to be a precondition; see if we know how to map it
			assertionOps, err := st.mapPrecondition(txnElem)
			if err != nil {
				return nil, err
			}

			out = append(out, assertionOps...)
		}
	}

	return out, nil
}

func (st *Store) mapPrecondition(txnElem transaction.Element) ([]txn.Op, error) {
	switch t := txnElem.(type) {
	case precondition.MachineAlivePrecondition:
		return []txn.Op{
			{
				C:      "machines",
				Id:     t.MachineID,
				Assert: isAliveDoc,
			},
		}, nil
	default:
		return nil, errors.Errorf("mongo store: unsupported txn element %#+v", t)
	}
}

var (
	isAliveDoc = bson.D{{"life", life.Alive}}
)

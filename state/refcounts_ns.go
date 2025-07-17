// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/mgo/v3/txn"

	"github.com/juju/juju/internal/mongo"
)

// refcountDoc holds a reference coun
// nsRefcounts exposes methods for safely manipulating reference count
// documents. (You can also manipulate them unsafely via the Just*
// methods that don't keep track of DB state.)
var nsRefcounts = nsRefcounts_{}

// nsRefcounts_ backs nsRefcounts.
type nsRefcounts_ struct{}

// CreateOrIncRefOp returns a txn.Op that creates a refcount document as
// configured with a specified value; or increments any such refcount doc
// that already exists.
func (ns nsRefcounts_) CreateOrIncRefOp(coll mongo.Collection, key string, n int) (txn.Op, error) {
	return txn.Op{}, nil
}

// DyingDecRefOp returns a txn.Op that decrements the value of a
// refcount doc and deletes it if the count reaches 0; if the Op will
// cause a delete, the bool result will be true. It will return an error
// if the doc does not exist or the count would go below 0.
func (ns nsRefcounts_) DyingDecRefOp(coll mongo.Collection, key string) (txn.Op, bool, error) {
	return txn.Op{}, false, nil
}

// RemoveOp returns a txn.Op that removes a refcount doc so long as its
// refcount is the supplied value, or an error.
func (ns nsRefcounts_) RemoveOp(coll mongo.Collection, key string, value int) (txn.Op, error) {
	return txn.Op{}, nil
}

// CurrentOp returns the current reference count value, and a txn.Op that
// asserts that the refcount has that value, or an error. If the refcount
// doc does not exist, then the op will assert that the document does not
// exist instead, and no error is returned.
func (ns nsRefcounts_) CurrentOp(coll mongo.Collection, key string) (txn.Op, int, error) {
	return txn.Op{}, 0, nil
}

// JustCreateOp returns a txn.Op that creates a refcount document as
// configured, *without* checking database state for sanity first.
// You should avoid using this method in most cases.
func (nsRefcounts_) JustCreateOp(collName, key string, value int) txn.Op {
	return txn.Op{}
}

// JustIncRefOp returns a txn.Op that increments a refcount document by
// the specified amount, as configured, *without* checking database state
// for sanity first. You should avoid using this method in most cases.
func (nsRefcounts_) JustIncRefOp(collName, key string, n int) txn.Op {
	return txn.Op{}
}

// JustRemoveOp returns a txn.Op that deletes a refcount doc so long as
// the refcount matches count. You should avoid using this method in
// most cases.
func (ns nsRefcounts_) JustRemoveOp(collName, key string, count int) txn.Op {
	op := txn.Op{}
	return op
}

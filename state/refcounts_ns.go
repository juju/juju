// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/errors"
	"github.com/juju/mgo/v3"
	"github.com/juju/mgo/v3/bson"
	"github.com/juju/mgo/v3/txn"

	"github.com/juju/juju/internal/mongo"
)

// refcountDoc holds a reference count. Refcounts are important to juju
// because mgo/txn offers no other mechanisms for safely coordinating
// deletion of unreferenced documents.
//
// TODO(fwereade) 2016-08-11 lp:1612163
//
// There are several places that use ad-hoc refcounts (application
// UnitCount and RelationCount; and model refs; and many many more)
// which should (1) be using separate refcount docs instead of dumping
// them in entity docs and (2) be using *this* refcount functionality
// rather than building their own ad-hoc variants.
type refcountDoc struct {

	// The _id field should hold some globalKey to identify what's
	// being referenced, but there's no reason to express it in this
	// document directly.

	// RefCount holds the reference count for whatever this doc is
	// referencing.
	RefCount int `bson:"refcount"`
}

var (
	errRefcountChanged     = errors.ConstError("refcount changed")
	errRefcountAlreadyZero = errors.ConstError("cannot decRef below 0")
)

// nsRefcounts exposes methods for safely manipulating reference count
// documents. (You can also manipulate them unsafely via the Just*
// methods that don't keep track of DB state.)
var nsRefcounts = nsRefcounts_{}

// nsRefcounts_ backs nsRefcounts.
type nsRefcounts_ struct{}

// LazyCreateOp returns a txn.Op that creates a refcount document; or
// false if the document already exists.
func (ns nsRefcounts_) LazyCreateOp(coll mongo.Collection, key string) (txn.Op, bool, error) {
	if exists, err := ns.exists(coll, key); err != nil {
		return txn.Op{}, false, errors.Trace(err)
	} else if exists {
		return txn.Op{}, false, nil
	}
	return ns.JustCreateOp(coll.Name(), key, 0), true, nil
}

// StrictCreateOp returns a txn.Op that creates a refcount document as
// configured, or an error if the document already exists.
func (ns nsRefcounts_) StrictCreateOp(coll mongo.Collection, key string, value int) (txn.Op, error) {
	if exists, err := ns.exists(coll, key); err != nil {
		return txn.Op{}, errors.Trace(err)
	} else if exists {
		return txn.Op{}, errors.New("refcount already exists")
	}
	return ns.JustCreateOp(coll.Name(), key, value), nil
}

// CreateOrIncRefOp returns a txn.Op that creates a refcount document as
// configured with a specified value; or increments any such refcount doc
// that already exists.
func (ns nsRefcounts_) CreateOrIncRefOp(coll mongo.Collection, key string, n int) (txn.Op, error) {
	if exists, err := ns.exists(coll, key); err != nil {
		return txn.Op{}, errors.Trace(err)
	} else if !exists {
		return ns.JustCreateOp(coll.Name(), key, n), nil
	}
	return ns.JustIncRefOp(coll.Name(), key, n), nil
}

// StrictIncRefOp returns a txn.Op that increments the value of a
// refcount doc, or returns an error if it does not exist.
func (ns nsRefcounts_) StrictIncRefOp(coll mongo.Collection, key string, n int) (txn.Op, error) {
	if exists, err := ns.exists(coll, key); err != nil {
		return txn.Op{}, errors.Trace(err)
	} else if !exists {
		return txn.Op{}, errors.New("does not exist")
	}
	return ns.JustIncRefOp(coll.Name(), key, n), nil
}

// AliveDecRefOp returns a txn.Op that decrements the value of a
// refcount doc, or an error if the doc does not exist or the count
// would go below 0.
func (ns nsRefcounts_) AliveDecRefOp(coll mongo.Collection, key string) (txn.Op, error) {
	if refcount, err := ns.read(coll, key); err != nil {
		return txn.Op{}, errors.Trace(err)
	} else if refcount < 1 {
		return txn.Op{}, errors.Annotatef(errRefcountAlreadyZero, "%s(%s)", coll.Name(), key)
	}
	return ns.justDecRefOp(coll.Name(), key, 0), nil
}

// DyingDecRefOp returns a txn.Op that decrements the value of a
// refcount doc and deletes it if the count reaches 0; if the Op will
// cause a delete, the bool result will be true. It will return an error
// if the doc does not exist or the count would go below 0.
func (ns nsRefcounts_) DyingDecRefOp(coll mongo.Collection, key string) (txn.Op, bool, error) {
	refcount, err := ns.read(coll, key)
	if err != nil {
		return txn.Op{}, false, errors.Trace(err)
	}
	if refcount < 1 {
		return txn.Op{}, false, errors.Annotatef(errRefcountAlreadyZero, "%s(%s)", coll.Name(), key)
	} else if refcount > 1 {
		return ns.justDecRefOp(coll.Name(), key, 1), false, nil
	}
	return ns.JustRemoveOp(coll.Name(), key, 1), true, nil
}

// RemoveOp returns a txn.Op that removes a refcount doc so long as its
// refcount is the supplied value, or an error.
func (ns nsRefcounts_) RemoveOp(coll mongo.Collection, key string, value int) (txn.Op, error) {
	refcount, err := ns.read(coll, key)
	if err != nil {
		return txn.Op{}, errors.Trace(err)
	}
	if refcount != value {
		logger.Tracef("reference of %s(%q) had %d refs, expected %d", coll.Name(), key, refcount, value)
		return txn.Op{}, errRefcountChanged
	}
	return ns.JustRemoveOp(coll.Name(), key, value), nil
}

// CurrentOp returns the current reference count value, and a txn.Op that
// asserts that the refcount has that value, or an error. If the refcount
// doc does not exist, then the op will assert that the document does not
// exist instead, and no error is returned.
func (ns nsRefcounts_) CurrentOp(coll mongo.Collection, key string) (txn.Op, int, error) {
	refcount, err := ns.read(coll, key)
	if errors.Is(err, errors.NotFound) {
		return txn.Op{
			C:      coll.Name(),
			Id:     key,
			Assert: txn.DocMissing,
		}, 0, nil
	}
	if err != nil {
		return txn.Op{}, -1, errors.Trace(err)
	}
	return txn.Op{
		C:      coll.Name(),
		Id:     key,
		Assert: bson.D{{"refcount", refcount}},
	}, refcount, nil
}

// JustCreateOp returns a txn.Op that creates a refcount document as
// configured, *without* checking database state for sanity first.
// You should avoid using this method in most cases.
func (nsRefcounts_) JustCreateOp(collName, key string, value int) txn.Op {
	return txn.Op{
		C:      collName,
		Id:     key,
		Assert: txn.DocMissing,
		Insert: bson.D{{"refcount", value}},
	}
}

// JustIncRefOp returns a txn.Op that increments a refcount document by
// the specified amount, as configured, *without* checking database state
// for sanity first. You should avoid using this method in most cases.
func (nsRefcounts_) JustIncRefOp(collName, key string, n int) txn.Op {
	return txn.Op{
		C:      collName,
		Id:     key,
		Assert: txn.DocExists,
		Update: bson.D{{"$inc", bson.D{{"refcount", n}}}},
	}
}

// JustRemoveOp returns a txn.Op that deletes a refcount doc so long as
// the refcount matches count. You should avoid using this method in
// most cases.
func (ns nsRefcounts_) JustRemoveOp(collName, key string, count int) txn.Op {
	op := txn.Op{
		C:      collName,
		Id:     key,
		Remove: true,
	}
	if count >= 0 {
		op.Assert = bson.D{{"refcount", count}}
	}
	return op
}

// justDecRefOp returns a txn.Op that decrements a refcount document by
// 1, as configured, allowing it to drop no lower than limit; which must
// not be less than zero. It's unexported, meaningless though that may
// be, to encourage clients to *really* not use it: too many ways to
// mess it up if you're not precisely aware of the context.
func (nsRefcounts_) justDecRefOp(collName, key string, limit int) txn.Op {
	return txn.Op{
		C:      collName,
		Id:     key,
		Assert: bson.D{{"refcount", bson.D{{"$gt", limit}}}},
		Update: bson.D{{"$inc", bson.D{{"refcount", -1}}}},
	}
}

// exists returns whether the identified refcount doc exists.
func (nsRefcounts_) exists(coll mongo.Collection, key string) (bool, error) {
	count, err := coll.FindId(key).Count()
	if err != nil {
		return false, errors.Trace(err)
	}
	return count != 0, nil
}

// read returns the value stored in the identified refcount doc.
func (nsRefcounts_) read(coll mongo.Collection, key string) (int, error) {
	var doc refcountDoc
	if err := coll.FindId(key).One(&doc); err == mgo.ErrNotFound {
		return 0, errors.NotFoundf("refcount %q", key)
	} else if err != nil {
		return 0, errors.Trace(err)
	}
	return doc.RefCount, nil
}

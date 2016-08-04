package state

import (
	"github.com/juju/errors"
	"github.com/juju/juju/mongo"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"
)

// refcountDoc holds reference counts.
type refcountDoc struct {
	// _id is some globalKey.
	RefCount int `bson:"refcount"`
}

// nsRefcounts_ backs nsRefcounts.
type nsRefcounts_ struct{}

// nsRefcounts exposes methods for safely manipulating reference count
// documents.
var nsRefcounts = nsRefcounts_{}

// JustCreateOp returns a txn.Op that creates a refcount document as
// configured, *without* checking database state for sanity first.
// You should prefer to avoid using this method in most cases.
func (nsRefcounts_) JustCreateOp(collName, key string, value int) txn.Op {
	return txn.Op{
		C:      collName,
		Id:     key,
		Assert: txn.DocMissing,
		Insert: bson.D{{"refcount", value}},
	}
}

// JustIncRefOp returns a txn.Op that increments a refcount document as
// configured, *without* checking database state for sanity first. You
// should prefer to avoid using this method in most cases.
func (nsRefcounts_) JustIncRefOp(collName, key string) txn.Op {
	return txn.Op{
		C:      collName,
		Id:     key,
		Assert: txn.DocExists,
		Update: bson.D{{"$inc", bson.D{{"refcount", 1}}}},
	}
}

// JustRemoveOp returns a txn.Op that deletes a refcount doc without
// *any validation at all*. You should *strongly* prefer to avoid using
// this method in most cases.
func (ns nsRefcounts_) JustRemoveOp(collName, key string) txn.Op {
	return txn.Op{
		C:      collName,
		Id:     key,
		Remove: true,
	}
}

// StrictCreateOp returns a txn.Op that creates a refcount document as
// configured, or an error if the document already exists.
func (ns nsRefcounts_) StrictCreateOp(refcounts mongo.Collection, key string, value int) (txn.Op, error) {
	if exists, err := ns.exists(refcounts, key); err != nil {
		return txn.Op{}, errors.Trace(err)
	} else if exists {
		return txn.Op{}, errors.New("refcount already exists")
	}
	return ns.JustCreateOp(refcounts.Name(), key, value), nil
}

// CreateIncrefOp returns a txn.Op that creates a refcount document as
// configured with a value of 1; or increments any such refcount doc
// that already exists.
func (ns nsRefcounts_) CreateIncRefOp(refcounts mongo.Collection, key string) (txn.Op, error) {
	if exists, err := ns.exists(refcounts, key); err != nil {
		return txn.Op{}, errors.Trace(err)
	} else if !exists {
		return ns.JustCreateOp(refcounts.Name(), key, 1), nil
	}
	return ns.JustIncRefOp(refcounts.Name(), key), nil
}

// StrictIncRefOp returns a txn.Op that increments the value of a
// refcount doc, or returns an error if it does not exist.
func (ns nsRefcounts_) StrictIncRefOp(refcounts mongo.Collection, key string) (txn.Op, error) {
	if exists, err := ns.exists(refcounts, key); err != nil {
		return txn.Op{}, errors.Trace(err)
	} else if !exists {
		return txn.Op{}, errors.New("refcount does not exist")
	}
	return ns.JustIncRefOp(refcounts.Name(), key), nil
}

// AliveDecRefOp returns a txn.Op that decrements the value of a
// refcount doc, or an error if the doc does not exist or the count
// would go below 0.
func (ns nsRefcounts_) AliveDecRefOp(refcounts mongo.Collection, key string) (txn.Op, error) {
	if refcount, err := ns.read(refcounts, key); err != nil {
		return txn.Op{}, errors.Trace(err)
	} else if refcount < 1 {
		return txn.Op{}, errors.New("cannot decRef below 0")
	}
	return txn.Op{
		C:      refcounts.Name(),
		Id:     key,
		Assert: bson.D{{"refcount", bson.D{{"$gt", 0}}}},
		Update: bson.D{{"$inc", bson.D{{"refcount", -1}}}},
	}, nil
}

// DyingDecRefOp returns a txn.Op that decrements the value of a
// refcount doc and deletes it if the count reaches 0; if the Op will
// cause a delete, the bool result will be true. It will return an error
// if the doc does not exist or the count would go below 0.
func (ns nsRefcounts_) DyingDecRefOp(refcounts mongo.Collection, key string) (txn.Op, bool, error) {
	refcount, err := ns.read(refcounts, key)
	if err != nil {
		return txn.Op{}, false, errors.Trace(err)
	}
	if refcount < 1 {
		return txn.Op{}, false, errors.New("cannot decRef below 0")
	} else if refcount > 1 {
		return txn.Op{
			C:      refcounts.Name(),
			Id:     key,
			Assert: bson.D{{"refcount", bson.D{{"$gt", 1}}}},
			Update: bson.D{{"$inc", bson.D{{"refcount", -1}}}},
		}, false, nil
	}
	return txn.Op{
		C:      refcounts.Name(),
		Id:     key,
		Assert: bson.D{{"refcount", 1}},
		Remove: true,
	}, true, nil
}

// exists returns whether the identified refcount doc exists.
func (nsRefcounts_) exists(refcounts mongo.Collection, key string) (bool, error) {
	count, err := refcounts.FindId(key).Count()
	if err != nil {
		return false, errors.Trace(err)
	}
	return count != 0, nil
}

// read returns the value stored in the identified refcount doc.
func (nsRefcounts_) read(refcounts mongo.Collection, key string) (int, error) {
	var doc refcountDoc
	if err := refcounts.FindId(key).One(&doc); err == mgo.ErrNotFound {
		return 0, errors.NotFoundf("refcount")
	} else if err != nil {
		return 0, errors.Trace(err)
	}
	return doc.RefCount, nil
}

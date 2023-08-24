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

// nsLife_ backs nsLife.
type nsLife_ struct{}

// nsLife namespaces low-level entity-life functionality. See the
// discussion in nsPayloads: this exists not to be the one place for
// life functionality (that would be a huge change), but to at least
// represent the parts we need for payloads in a consistent fashion.
//
// Both the namespacing and the explicit Collection->op approach seem
// to be good ideas, and should ideally be extended as we continue.
var nsLife = nsLife_{}

// destroyOp returns errNotAlive if the identified entity is not Alive;
// or a txn.Op that will fail if the condition no longer holds, and
// otherwise set Life to Dying and make any other updates supplied in
// update.
func (nsLife_) destroyOp(entities mongo.Collection, docID string, update bson.D) (txn.Op, error) {
	op, err := nsLife.aliveOp(entities, docID)
	if err != nil {
		return txn.Op{}, errors.Trace(err)
	}
	setDying := bson.D{{"$set", bson.D{{"life", Dying}}}}
	op.Update = append(setDying, update...)
	return op, nil
}

// aliveOp returns errNotAlive if the identified entity is not Alive; or
// a txn.Op that will fail if the condition no longer holds.
func (nsLife_) aliveOp(entities mongo.Collection, docID string) (txn.Op, error) {
	op, err := nsLife.checkOp(entities, docID, nsLife.alive())
	switch errors.Cause(err) {
	case nil:
	case errCheckFailed:
		return txn.Op{}, notAliveErr
	default:
		return txn.Op{}, errors.Trace(err)
	}
	return op, nil
}

// notDeadOp returns errDeadOrGone if the identified entity is not Alive
// or Dying, or a txn.Op that will fail if the condition no longer
// holds.
func (nsLife_) notDeadOp(entities mongo.Collection, docID string) (txn.Op, error) {
	op, err := nsLife.checkOp(entities, docID, nsLife.notDead())
	switch errors.Cause(err) {
	case nil:
	case errCheckFailed:
		return txn.Op{}, errDeadOrGone
	default:
		return txn.Op{}, errors.Trace(err)
	}
	return op, nil
}

var errCheckFailed = errors.New("check failed")

func (nsLife_) checkOp(entities mongo.Collection, docID string, check bson.D) (txn.Op, error) {
	sel := append(bson.D{{"_id", docID}}, check...)
	count, err := entities.Find(sel).Count()
	if err != nil {
		return txn.Op{}, errors.Trace(err)
	} else if count == 0 {
		return txn.Op{}, errCheckFailed
	}
	return txn.Op{
		C:      entities.Name(),
		Id:     docID,
		Assert: check,
	}, nil
}

func (nsLife_) read(entities mongo.Collection, docID string) (Life, error) {
	var doc struct {
		Life Life `bson:"life"`
	}
	err := entities.FindId(docID).One(&doc)
	switch errors.Cause(err) {
	case nil:
	case mgo.ErrNotFound:
		return Dead, errors.NotFoundf("entity")
	default:
		return Dead, errors.Trace(err)
	}
	return doc.Life, nil
}

// alive returns a selector that matches only documents whose life
// field is set to Alive.
func (nsLife_) alive() bson.D {
	return bson.D{{"life", Alive}}
}

// notDead returns a selector that matches only documents whose life
// field is not set to Dead.
func (nsLife_) notDead() bson.D {
	return bson.D{{"life", bson.D{{"$ne", Dead}}}}
}

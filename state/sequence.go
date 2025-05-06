// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/mgo/v3"
	"github.com/juju/mgo/v3/bson"
)

type sequenceDoc struct {
	DocID     string `bson:"_id"`
	Name      string `bson:"name"`
	ModelUUID string `bson:"model-uuid"`
	Counter   int    // This stores the *next* value to return.
}

// sequence safely increments a database backed sequence, returning
// the next value.
func sequence(mb modelBackend, name string) (int, error) {
	sequences, closer := mb.db().GetCollection(sequenceC)
	defer closer()
	query := sequences.FindId(name)
	inc := mgo.Change{
		Update: bson.M{
			"$set": bson.M{
				"name":       name,
				"model-uuid": mb.ModelUUID(),
			},
			"$inc": bson.M{"counter": 1},
		},
		Upsert: true,
	}
	result := &sequenceDoc{}
	_, err := query.Apply(inc, result)
	if err != nil {
		return -1, fmt.Errorf("cannot increment %q sequence number: %v", name, err)
	}
	return result.Counter, nil
}

func resetSequence(mb modelBackend, name string) error {
	sequences, closer := mb.db().GetCollection(sequenceC)
	defer closer()
	err := sequences.Writeable().RemoveId(name)
	if err != nil && errors.Cause(err) != mgo.ErrNotFound {
		return errors.Annotatef(err, "can not reset sequence for %q", name)
	}
	return nil
}

// sequenceWithMin safely increments a database backed sequence,
// allowing for a minimum value for the sequence to be specified. The
// minimum value is used as an initial value for the first use of a
// particular sequence. The minimum value will also cause a sequence
// value to jump ahead if the minimum is provided that is higher than
// the current sequence value.
//
// The data manipulated by `sequence` and `sequenceWithMin` is the
// same. It is safe to mix the 2 methods for the same sequence.
//
// `sequence` is more efficient than `sequenceWithMin` and should be
// preferred if there is no minimum value requirement.
func sequenceWithMin(mb modelBackend, name string, minVal int) (int, error) {
	sequences, closer := mb.db().GetRawCollection(sequenceC)
	defer closer()
	updater := newDbSeqUpdater(sequences, mb.ModelUUID(), name)
	return updateSeqWithMin(updater, minVal)
}

// seqUpdater abstracts away the database operations required for
// updating a sequence.
type seqUpdater interface {
	// read returns the current value of the sequence. If the sequence
	// doesn't exist yet it returns 0.
	read() (int, error)

	// create attempts to create a new sequence with the initial value
	// provided. It returns (true, nil) on success, (false, nil) if
	// the sequence already existed and (false, <some error>) if any
	// other error occurred.
	create(value int) (bool, error)

	// set attempts to update the sequence value to a new value. It
	// takes the expected current value of the sequence as well as the
	// new value to set it to. (true, nil) is returned if the value
	// was updated successfully. (false, nil) is returned if the
	// sequence was not at the expected value (indicating a concurrent
	// update). (false, <some error>) is returned for any other
	// problem.
	set(expected, next int) (bool, error)
}

// Sequences returns the model's sequence names and their next values.
func (st *State) Sequences() (map[string]int, error) {
	sequences, closer := st.db().GetCollection(sequenceC)
	defer closer()

	var docs []sequenceDoc
	if err := sequences.Find(nil).All(&docs); err != nil {
		return nil, errors.Trace(err)
	}

	result := make(map[string]int)
	for _, doc := range docs {
		result[doc.Name] = doc.Counter
	}
	return result, nil
}

const maxSeqRetries = 20

// updateSeqWithMin implements the abstract logic for incrementing a
// database backed sequence in a concurrency aware way.
//
// It is complicated because MongoDB's atomic update primitives don't
// provide a way to upsert a counter while also providing an initial
// value.  Instead, a number of database operations are used for each
// sequence update, relying on the atomicity guarantees that MongoDB
// offers. Optimistic database updates are attempted with retries when
// contention is observed.
func updateSeqWithMin(sequence seqUpdater, minVal int) (int, error) {
	for try := 0; try < maxSeqRetries; try++ {
		curVal, err := sequence.read()
		if err != nil {
			return -1, errors.Annotate(err, "could not read sequence")
		}
		if curVal == 0 {
			// No sequence document exists, create one.
			ok, err := sequence.create(minVal + 1)
			if err != nil {
				return -1, errors.Annotate(err, "could not create sequence")
			}
			if ok {
				return minVal, nil
			}
			// Someone else created the sequence document at the same
			// time, try again.
		} else {
			// Increment an existing sequence document, respecting the
			// minimum value provided.
			nextVal := curVal + 1
			if nextVal <= minVal {
				nextVal = minVal + 1
			}
			ok, err := sequence.set(curVal, nextVal)
			if err != nil {
				return -1, errors.Annotate(err, "could not set sequence")
			}
			if ok {
				return nextVal - 1, nil
			}
			// Someone else incremented the sequence at the same time,
			// try again.
		}
	}
	return -1, errors.New("too much contention while updating sequence")
}

// dbSeqUpdater implements seqUpdater.
type dbSeqUpdater struct {
	coll      *mgo.Collection
	modelUUID string
	name      string
	id        string
}

func newDbSeqUpdater(coll *mgo.Collection, modelUUID, name string) *dbSeqUpdater {
	return &dbSeqUpdater{
		coll:      coll,
		modelUUID: modelUUID,
		name:      name,
		id:        modelUUID + ":" + name,
	}
}

func (su *dbSeqUpdater) read() (int, error) {
	var doc bson.M
	err := su.coll.FindId(su.id).One(&doc)
	if errors.Cause(err) == mgo.ErrNotFound {
		return 0, nil
	} else if err != nil {
		return -1, errors.Trace(err)
	}
	return doc["counter"].(int), nil
}

func (su *dbSeqUpdater) create(value int) (bool, error) {
	err := su.coll.Insert(bson.M{
		"_id":        su.id,
		"name":       su.name,
		"model-uuid": su.modelUUID,
		"counter":    value,
	})
	if mgo.IsDup(errors.Cause(err)) {
		return false, nil
	} else if err != nil {
		return false, errors.Trace(err)
	}
	return true, nil
}

func (su *dbSeqUpdater) set(expected, next int) (bool, error) {
	err := su.coll.Update(
		bson.M{"_id": su.id, "counter": expected},
		bson.M{"$set": bson.M{"counter": next}},
	)
	if errors.Cause(err) == mgo.ErrNotFound {
		return false, nil
	} else if err != nil {
		return false, errors.Trace(err)
	}
	return true, nil
}

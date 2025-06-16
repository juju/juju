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

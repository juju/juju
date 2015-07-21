// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"

	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

type sequenceDoc struct {
	DocID   string `bson:"_id"`
	Name    string `bson:"name"`
	EnvUUID string `bson:"env-uuid"`
	Counter int
}

func (s *State) sequence(name string) (int, error) {
	sequences, closer := s.getCollection(sequenceC)
	defer closer()
	query := sequences.FindId(name)
	inc := mgo.Change{
		Update: bson.M{
			"$set": bson.M{
				"name":     name,
				"env-uuid": s.EnvironUUID(),
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

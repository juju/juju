// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"

	"labix.org/v2/mgo"
	"labix.org/v2/mgo/bson"
)

type sequenceDoc struct {
	Name    string `bson:"_id"`
	Counter int
}

func (s *State) sequence(name string) (int, error) {
	query := s.db.C("sequence").Find(D{{"_id", name}})
	inc := mgo.Change{
		Update: bson.M{"$inc": bson.M{"counter": 1}},
		Upsert: true,
	}
	result := &sequenceDoc{}
	_, err := query.Apply(inc, result)
	if err != nil {
		return -1, fmt.Errorf("cannot increment %q sequence number: %v", name, err)
	}
	return result.Counter, nil
}

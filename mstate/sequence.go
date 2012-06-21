package mstate

import (
	"fmt"
	"launchpad.net/mgo"
	"launchpad.net/mgo/bson"
)

type sequenceDoc struct {
	Name    string `bson:"_id"`
	Counter int
}

func (s *State) sequence(name string) (int, error) {
	query := s.db.C("sequence").Find(bson.D{{"_id", name}})
	inc := mgo.Change{
		Update: bson.M{"$inc": bson.M{"counter": 1}},
		Upsert: true,
	}
	result := &sequenceDoc{}
	err := query.Modify(inc, result)
	if err != nil {
		return -1, fmt.Errorf("can't increment %q sequence number: %v", name, err)
	}
	return result.Counter, nil
}

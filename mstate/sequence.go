package mstate

import (
	"launchpad.net/mgo"
	"launchpad.net/mgo/bson"
)

type sequenceDoc struct {
	Collection string `bson:"_id"`
	Counter    int
}

func (s *State) sequence(name string) (int, error) {
	query := s.db.C("sequence").Find(bson.D{{"_id", name}})
	inc := mgo.Change{Update: bson.M{"$inc": bson.M{"counter": 1}}, Upsert: true}
	result := &sequenceDoc{}
	err := query.Modify(inc, result)
	return result.Counter, err
}

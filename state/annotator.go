package state

import (
	"fmt"
	"labix.org/v2/mgo/txn"
)

type annotator struct {
	annotations *map[string]string
	st          *State
	coll        string
	id          string
}

func (a annotator) SetAnnotation(key, value string) error {
	ops := []txn.Op{{
		C:      a.coll,
		Id:     a.id,
		Assert: isAliveDoc,
		Update: D{{"$set", D{{"annotations." + key, value}}}},
	}}
	// TODO key should not contain dots
	if err := a.st.runner.Run(ops, "", nil); err != nil {
		return fmt.Errorf("cannot set annotation %q = %q: %v", key, value, onAbort(err, errNotAlive))
	}
	if *a.annotations == nil {
		*a.annotations = make(map[string]string)
	}
	(*a.annotations)[key] = value
	return nil
}

func (a annotator) Annotation(key string) string {
	return (*a.annotations)[key]
}

func (a annotator) RemoveAnnotation(key string) error {
	if _, ok := (*a.annotations)[key]; ok {
		ops := []txn.Op{{
			C:      a.coll,
			Id:     a.id,
			Assert: isAliveDoc,
			Update: D{{"$unset", D{{"annotations." + key, true}}}},
		}}
		if err := a.st.runner.Run(ops, "", nil); err != nil {
			return fmt.Errorf("cannot remove annotation %q: %v", key, onAbort(err, errNotAlive))
		}
		delete(*a.annotations, key)
	}
	return nil
}

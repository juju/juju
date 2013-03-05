package state

import (
	"fmt"
	"labix.org/v2/mgo/txn"
	"strings"
)

// annotator stores annotations and information required to query MongoDB.
type annotator struct {
	annotations *map[string]string
	st          *State
	coll        string
	id          string
}

// SetAnnotation adds a key/value pair to annotations in MongoDB and the annotator.
func (a *annotator) SetAnnotation(key, value string) error {
	if strings.Contains(key, ".") {
		return fmt.Errorf("invalid key %q", key)
	}
	if *a.annotations == nil {
		*a.annotations = make(map[string]string)
	}
	if value == "" {
		// Delete a key/value pair in MongoDB.
		ops := []txn.Op{{
			C:      a.coll,
			Id:     a.id,
			Assert: isAliveDoc,
			Update: D{{"$unset", D{{"annotations." + key, true}}}},
		}}
		if err := a.st.runner.Run(ops, "", nil); err != nil {
			return fmt.Errorf("cannot delete annotation %q: %v", key, onAbort(err, errNotAlive))
		}
		delete(*a.annotations, key)
	} else {
		// Set a key/value pair in MongoDB.
		ops := []txn.Op{{
			C:      a.coll,
			Id:     a.id,
			Assert: isAliveDoc,
			Update: D{{"$set", D{{"annotations." + key, value}}}},
		}}
		if err := a.st.runner.Run(ops, "", nil); err != nil {
			return fmt.Errorf("cannot set annotation %q = %q: %v", key, value, onAbort(err, errNotAlive))
		}
		(*a.annotations)[key] = value
	}
	return nil
}

// Annotation returns the annotation value corresponding to the given key.
func (a annotator) Annotation(key string) string {
	return (*a.annotations)[key]
}

package state

import (
	"fmt"
	"labix.org/v2/mgo"
	"labix.org/v2/mgo/txn"
	"strings"
)

// annDoc represents the internal state of annotations for an Entity in MongoDB.
type annDoc struct {
	EntityName  string `bson:"_id"`
	Annotations map[string]string
}

// annotator stores information required to query MongoDB.
type annotator struct {
	entityName string
	st         *State
}

// SetAnnotation adds a key/value pair to annotations in MongoDB.
func (a *annotator) SetAnnotation(key, value string) error {
	if strings.Contains(key, ".") {
		return fmt.Errorf("invalid key %q", key)
	}
	id := a.entityName
	coll := a.st.annotations.Name
	if value == "" {
		// Delete a key/value pair in MongoDB.
		ops := []txn.Op{{
			C:      coll,
			Id:     id,
			Assert: isAliveDoc,
			Update: D{{"$unset", D{{"annotations." + key, true}}}},
		}}
		if err := a.st.runner.Run(ops, "", nil); err != nil {
			return fmt.Errorf("cannot delete annotation %q on %s: %v", key, id, onAbort(err, errNotAlive))
		}
	} else {
		// Set a key/value pair in MongoDB.
		doc := &annDoc{
			EntityName:  id,
			Annotations: map[string]string{key: value},
		}
		ops := []txn.Op{{
			C:      coll,
			Id:     id,
			Assert: txn.DocMissing,
			Insert: doc,
		}, {
			C:      coll,
			Id:     id,
			Assert: isAliveDoc,
			Update: D{{"$set", D{{"annotations." + key, value}}}},
		}}
		if err := a.st.runner.Run(ops, "", nil); err != nil {
			return fmt.Errorf("cannot set annotation %q = %q on %s: %v", key, value, id, onAbort(err, errNotAlive))
		}
	}
	return nil
}

// Annotations returns all the annotations corresponding to an entity.
func (a annotator) Annotations() (map[string]string, error) {
	doc := new(annDoc)
	err := a.st.annotations.FindId(a.entityName).One(doc)
	if err == mgo.ErrNotFound {
		return make(map[string]string), nil
	}
	if err != nil {
		return make(map[string]string), err
	}
	return doc.Annotations, nil
}

// Annotation returns the annotation value corresponding to the given key.
func (a annotator) Annotation(key string) (string, error) {
	ann, err := a.Annotations()
	if err != nil {
		return "", err
	}
	return ann[key], nil
}

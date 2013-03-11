package state

import (
	"fmt"
	"labix.org/v2/mgo"
	"labix.org/v2/mgo/txn"
	"strings"
)

// annotatorDoc represents the internal state of annotations for an Entity in
// MongoDB. Note that the annotations map is not maintained in local storage
// due to the fact that it is not accessed directly, but through
// Annotations/Annotation below.
type annotatorDoc struct {
	EntityName  string `bson:"_id"`
	Annotations map[string]string
}

// annotator stores information required to query MongoDB.
type annotator struct {
	st         *State
	entityName string
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
			Assert: txn.DocExists,
			Update: D{{"$unset", D{{"annotations." + key, true}}}},
		}}
		if err := a.st.runner.Run(ops, "", nil); err != nil {
			return fmt.Errorf("cannot delete annotation %q on %s: %v", key, id, onAbort(err, errNotAlive))
		}
	} else {
		// Set a key/value pair in MongoDB.
		var op txn.Op
		if count, err := a.st.annotations.FindId(id).Count(); err != nil {
			return err
		} else if count != 0 {
			op = txn.Op{
				C:      coll,
				Id:     id,
				Assert: txn.DocExists,
				Update: D{{"$set", D{{"annotations." + key, value}}}},
			}
		} else {
			op = txn.Op{
				C:      coll,
				Id:     id,
				Assert: txn.DocMissing,
				Insert: &annotatorDoc{id, map[string]string{key: value}},
			}
		}
		if err := a.st.runner.Run([]txn.Op{op}, "", nil); err != nil {
			return fmt.Errorf("cannot set annotation %q = %q on %s: %v", key, value, id, err)
		}
	}
	return nil
}

// Annotations returns all the annotations corresponding to an entity.
func (a annotator) Annotations() (map[string]string, error) {
	doc := new(annotatorDoc)
	err := a.st.annotations.FindId(a.entityName).One(doc)
	if err == mgo.ErrNotFound {
		// Returning an empty map if there are no annotations.
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return doc.Annotations, nil
}

// Annotation returns the annotation value corresponding to the given key.
func (a annotator) Annotation(key string) (string, error) {
	ann, err := a.Annotations()
	if err != nil {
		return "", err
	}
	// Returning an empty string if the requested annotation is not found.
	return ann[key], nil
}

// annotationRemoveOps returns an operation to remove a given annotation document from MongoDB.
func annotationRemoveOps(st *State, id string) txn.Op {
	return txn.Op{
		C:      st.annotations.Name,
		Id:     id,
		Remove: true,
	}
}

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
	GlobalKey   string `bson:"_id"`
	EntityName  string
	Annotations map[string]string
}

// annotator implements annotation-related methods
// for any entity that wishes to use it.
type annotator struct {
	globalKey  string
	entityName string
	st         *State
}

// SetAnnotations adds key/value pairs to annotations in MongoDB.
func (a *annotator) SetAnnotations(pairs map[string]string) error {
	if len(pairs) == 0 {
		return nil
	}
	// Collect in separate maps pairs to be inserted/updated or removed.
	toRemove := make(map[string]bool)
	toInsert := make(map[string]string)
	toUpdate := make(map[string]string)
	for key, value := range pairs {
		if strings.Contains(key, ".") {
			return fmt.Errorf("invalid key %q", key)
		}
		if value == "" {
			toRemove["annotations."+key] = true
		} else {
			toInsert[key] = value
			toUpdate["annotations."+key] = value
		}
	}
	// Perform two attempts to update annotations in order to avoid possible
	// race conditions when the document is missing.
	for i := 0; i < 2; i++ {
		op, err := a.setAnnotationsOp(toInsert, toUpdate, toRemove)
		if err != nil {
			return err
		}
		if err := a.st.runner.Run([]txn.Op{op}, "", nil); err == nil {
			return nil
		} else if err != txn.ErrAborted {
			return fmt.Errorf("cannot update annotations on %s: %v", a.globalKey, err)
		}
	}
	// Check that the annotator entity was not previously destroyed.
	if count, err := a.st.annotations.FindId(a.globalKey).Count(); err != nil {
		return err
	} else if count == 0 {
		return fmt.Errorf("%s no longer exists", a.entityName)
	}
	return fmt.Errorf("cannot update annotations on %s: %v", a.globalKey, ErrExcessiveContention)
}

// setAnnotationsOp returns the operation required to insert, update or
// remove annotations in MongoDB.
func (a *annotator) setAnnotationsOp(toInsert, toUpdate map[string]string, toRemove map[string]bool) (txn.Op, error) {
	id := a.globalKey
	coll := a.st.annotations.Name
	if count, err := a.st.annotations.FindId(id).Count(); err != nil {
		return txn.Op{}, err
	} else if count == 0 {
		// The document is missing: no need to remove pairs.
		// Insert pairs as required.
		return txn.Op{
			C:      coll,
			Id:     id,
			Assert: txn.DocMissing,
			Insert: &annotatorDoc{id, a.entityName, toInsert},
		}, nil
	}
	// The document exists: update and remove pairs.
	return txn.Op{
		C:      coll,
		Id:     id,
		Assert: txn.DocExists,
		Update: D{{"$set", toUpdate}, {"$unset", toRemove}},
	}, nil
}

// Annotations returns all the annotations corresponding to an entity.
func (a *annotator) Annotations() (map[string]string, error) {
	doc := new(annotatorDoc)
	err := a.st.annotations.FindId(a.globalKey).One(doc)
	if err == mgo.ErrNotFound {
		// Returning an empty map if there are no annotations.
		return make(map[string]string), nil
	}
	if err != nil {
		return nil, err
	}
	return doc.Annotations, nil
}

// Annotation returns the annotation value corresponding to the given key.
// If the requested annotation is not found, an empty string is returned.
func (a *annotator) Annotation(key string) (string, error) {
	ann, err := a.Annotations()
	if err != nil {
		return "", err
	}
	return ann[key], nil
}

// annotationRemoveOp returns an operation to remove a given annotation
// document from MongoDB.
func annotationRemoveOp(st *State, id string) txn.Op {
	return txn.Op{
		C:      st.annotations.Name,
		Id:     id,
		Remove: true,
	}
}

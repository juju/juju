// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"strings"

	"labix.org/v2/mgo"
	"labix.org/v2/mgo/txn"

	"launchpad.net/juju-core/utils"
)

// annotatorDoc represents the internal state of annotations for an Entity in
// MongoDB. Note that the annotations map is not maintained in local storage
// due to the fact that it is not accessed directly, but through
// Annotations/Annotation below.
// Note also the correspondence with AnnotationInfo in state/api/params.
type annotatorDoc struct {
	GlobalKey   string `bson:"_id"`
	Tag         string
	Annotations map[string]string
}

// annotator implements annotation-related methods
// for any entity that wishes to use it.
type annotator struct {
	globalKey string
	tag       string
	st        *State
}

// SetAnnotations adds key/value pairs to annotations in MongoDB.
func (a *annotator) SetAnnotations(pairs map[string]string) (err error) {
	defer utils.ErrorContextf(&err, "cannot update annotations on %s", a.tag)
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
	// Two attempts should be enough to update annotations even with racing
	// clients - if the document does not already exist, one of the clients
	// will create it and the others will fail, then all the rest of the
	// clients should succeed on their second attempt. If the referred-to
	// entity has disappeared, and removed its annotations in the meantime,
	// we consider that worthy of an error (will be fixed when new entities
	// can never share names with old ones).
	for i := 0; i < 2; i++ {
		var ops []txn.Op
		if count, err := a.st.annotations.FindId(a.globalKey).Count(); err != nil {
			return err
		} else if count == 0 {
			// Check that the annotator entity was not previously destroyed.
			if i != 0 {
				return fmt.Errorf("%s no longer exists", a.tag)
			}
			ops, err = a.insertOps(toInsert)
			if err != nil {
				return err
			}
		} else {
			ops = a.updateOps(toUpdate, toRemove)
		}
		if err := a.st.runTransaction(ops); err == nil {
			return nil
		} else if err != txn.ErrAborted {
			return err
		}
	}
	return ErrExcessiveContention
}

// insertOps returns the operations required to insert annotations in MongoDB.
func (a *annotator) insertOps(toInsert map[string]string) ([]txn.Op, error) {
	tag := a.tag
	ops := []txn.Op{{
		C:      a.st.annotations.Name,
		Id:     a.globalKey,
		Assert: txn.DocMissing,
		Insert: &annotatorDoc{a.globalKey, tag, toInsert},
	}}
	if strings.HasPrefix(tag, "environment-") {
		return ops, nil
	}
	// If the entity is not the environment, add a DocExists check on the
	// entity document, in order to avoid possible races between entity
	// removal and annotation creation.
	coll, id, err := a.st.parseTag(tag)
	if err != nil {
		return nil, err
	}
	return append(ops, txn.Op{
		C:      coll,
		Id:     id,
		Assert: txn.DocExists,
	}), nil
}

// updateOps returns the operations required to update or remove annotations in MongoDB.
func (a *annotator) updateOps(toUpdate map[string]string, toRemove map[string]bool) []txn.Op {
	return []txn.Op{{
		C:      a.st.annotations.Name,
		Id:     a.globalKey,
		Assert: txn.DocExists,
		Update: D{{"$set", toUpdate}, {"$unset", toRemove}},
	}}
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

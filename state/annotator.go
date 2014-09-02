// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/names"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"
)

// annotatorDoc represents the internal state of annotations for an Entity in
// MongoDB. Note that the annotations map is not maintained in local storage
// due to the fact that it is not accessed directly, but through
// Annotations/Annotation below.
// Note also the correspondence with AnnotationInfo in apiserver/params.
type annotatorDoc struct {
	GlobalKey   string `bson:"_id"`
	Tag         string
	Annotations map[string]string
}

// annotator implements annotation-related methods
// for any entity that wishes to use it.
type annotator struct {
	globalKey string
	tag       names.Tag
	st        *State
}

// SetAnnotations adds key/value pairs to annotations in MongoDB.
func (a *annotator) SetAnnotations(pairs map[string]string) (err error) {
	defer errors.Maskf(&err, "cannot update annotations on %s", a.tag)
	if len(pairs) == 0 {
		return nil
	}
	// Collect in separate maps pairs to be inserted/updated or removed.
	toRemove := make(bson.M)
	toInsert := make(map[string]string)
	toUpdate := make(bson.M)
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
	// Set up and call the necessary transactions - if the document does not
	// already exist, one of the clients will create it and the others will
	// fail, then all the rest of the clients should succeed on their second
	// attempt. If the referred-to entity has disappeared, and removed its
	// annotations in the meantime, we consider that worthy of an error
	// (will be fixed when new entities can never share names with old ones).
	buildTxn := func(attempt int) ([]txn.Op, error) {
		annotations, closer := a.st.getCollection(annotationsC)
		defer closer()
		if count, err := annotations.FindId(a.globalKey).Count(); err != nil {
			return nil, err
		} else if count == 0 {
			// Check that the annotator entity was not previously destroyed.
			if attempt != 0 {
				return nil, fmt.Errorf("%s no longer exists", a.tag)
			}
			return a.insertOps(toInsert)
		}
		return a.updateOps(toUpdate, toRemove), nil
	}
	return a.st.run(buildTxn)
}

// insertOps returns the operations required to insert annotations in MongoDB.
func (a *annotator) insertOps(toInsert map[string]string) ([]txn.Op, error) {
	tag := a.tag
	ops := []txn.Op{{
		C:      annotationsC,
		Id:     a.globalKey,
		Assert: txn.DocMissing,
		Insert: &annotatorDoc{a.globalKey, tag.String(), toInsert},
	}}
	switch tag.(type) {
	case names.EnvironTag:
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
func (a *annotator) updateOps(toUpdate, toRemove bson.M) []txn.Op {
	return []txn.Op{{
		C:      annotationsC,
		Id:     a.globalKey,
		Assert: txn.DocExists,
		Update: setUnsetUpdate(toUpdate, toRemove),
	}}
}

// Annotations returns all the annotations corresponding to an entity.
func (a *annotator) Annotations() (map[string]string, error) {
	doc := new(annotatorDoc)
	annotations, closer := a.st.getCollection(annotationsC)
	defer closer()
	err := annotations.FindId(a.globalKey).One(doc)
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
		C:      annotationsC,
		Id:     id,
		Remove: true,
	}
}

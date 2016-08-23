// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import "github.com/juju/errors"

// DumpAll returns a map of collection names to a slice of documents
// in that collection. Every document that is related to the current
// model is returned in the map.
func (st *State) DumpAll() (map[string]interface{}, error) {
	result := make(map[string]interface{})
	// Add in the model document itself.
	doc, err := getModelDoc(st)
	if err != nil {
		return nil, err
	}
	result[modelsC] = doc
	for name, info := range allCollections() {
		if !info.global {
			docs, err := getAllModelDocs(st, name)
			if err != nil {
				return nil, errors.Trace(err)
			}
			if len(docs) > 0 {
				result[name] = docs
			}
		}
	}
	return result, nil
}

func getModelDoc(st *State) (map[string]interface{}, error) {
	coll, closer := st.getCollection(modelsC)
	defer closer()

	var doc map[string]interface{}
	if err := coll.FindId(st.ModelUUID()).One(&doc); err != nil {
		return nil, errors.Annotatef(err, "reading model %q", st.ModelUUID())
	}
	return doc, nil

}

func getAllModelDocs(st *State, collectionName string) ([]map[string]interface{}, error) {
	coll, closer := st.getCollection(collectionName)
	defer closer()

	var (
		result []map[string]interface{}
		doc    map[string]interface{}
	)
	// Always output in id order.
	iter := coll.Find(nil).Sort("_id").Iter()
	defer iter.Close()
	for iter.Next(&doc) {
		result = append(result, doc)
		doc = nil
	}

	if err := iter.Err(); err != nil {
		return nil, errors.Annotatef(err, "reading collection %q", collectionName)
	}
	return result, nil
}

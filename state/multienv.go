// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"strings"

	"github.com/juju/errors"
	"gopkg.in/mgo.v2/bson"
)

// This file contains utility functions related to documents and
// collections that contain data for multiple models.

// ensureModelUUID returns an model UUID prefixed document ID. The
// prefix is only added if it isn't already there.
func ensureModelUUID(modelUUID, id string) string {
	prefix := modelUUID + ":"
	if strings.HasPrefix(id, prefix) {
		return id
	}
	return prefix + id
}

// ensureModelUUIDIfString will call ensureModelUUID, but only if the id
// is a string. The id will be left untouched otherwise.
func ensureModelUUIDIfString(modelUUID string, id interface{}) interface{} {
	if id, ok := id.(string); ok {
		return ensureModelUUID(modelUUID, id)
	}
	return id
}

// splitDocID returns the 2 parts of model UUID prefixed
// document ID. If the id is not in the expected format the final
// return value will be false.
func splitDocID(id string) (string, string, bool) {
	parts := strings.SplitN(id, ":", 2)
	if len(parts) != 2 {
		return "", "", false
	}
	return parts[0], parts[1], true
}

const modelUUIDRequired = 1
const noModelUUIDInInput = 2

// mungeDocForMultiEnv takes the value of an txn.Op Insert or $set
// Update and modifies it to be multi-model safe, returning the
// modified document.
func mungeDocForMultiEnv(doc interface{}, modelUUID string, modelUUIDFlags int) (bson.D, error) {
	var bDoc bson.D
	var err error
	if doc != nil {
		bDoc, err = toBsonD(doc)
		if err != nil {
			return nil, errors.Trace(err)
		}
	}

	modelUUIDSeen := false
	for i, elem := range bDoc {
		switch elem.Name {
		case "_id":
			if id, ok := elem.Value.(string); ok {
				bDoc[i].Value = ensureModelUUID(modelUUID, id)
			} else if subquery, ok := elem.Value.(bson.D); ok {
				munged, err := mungeIDSubQueryForMultiEnv(subquery, modelUUID)
				if err != nil {
					return nil, errors.Trace(err)
				}
				bDoc[i].Value = munged
			}
		case "model-uuid":
			if modelUUIDFlags&noModelUUIDInInput > 0 {
				return nil, errors.New("model-uuid is added automatically and should not be provided")
			}
			modelUUIDSeen = true
			if elem.Value == "" {
				bDoc[i].Value = modelUUID
			} else if elem.Value != modelUUID {
				return nil, errors.Errorf(`bad "model-uuid" value: expected %s, got %s`, modelUUID, elem.Value)
			}
		}
	}
	if modelUUIDFlags&modelUUIDRequired > 0 && !modelUUIDSeen {
		bDoc = append(bDoc, bson.DocElem{"model-uuid", modelUUID})
	}
	return bDoc, nil
}

func mungeIDSubQueryForMultiEnv(doc interface{}, modelUUID string) (bson.D, error) {
	var bDoc bson.D
	var err error
	if doc != nil {
		bDoc, err = toBsonD(doc)
		if err != nil {
			return nil, errors.Trace(err)
		}
	}

	for i, elem := range bDoc {
		switch elem.Name {
		case "$in":
			var ids []string
			switch values := elem.Value.(type) {
			case []string:
				ids = values
			case []interface{}:
				for _, value := range values {
					id, ok := value.(string)
					if !ok {
						continue
					}
					ids = append(ids, id)
				}
				if len(ids) != len(values) {
					// We expect the type to be consistently string, so...
					continue
				}
			default:
				continue
			}

			var fullIDs []string
			for _, id := range ids {
				fullID := ensureModelUUID(modelUUID, id)
				fullIDs = append(fullIDs, fullID)
			}
			bDoc[i].Value = fullIDs
		}
	}
	return bDoc, nil
}

// toBsonD converts an arbitrary value to a bson.D via marshaling
// through BSON. This is still done even if the input is already a
// bson.D so that we end up with a copy of the input.
func toBsonD(doc interface{}) (bson.D, error) {
	bytes, err := bson.Marshal(doc)
	if err != nil {
		return nil, errors.Annotate(err, "bson marshaling failed")
	}
	var out bson.D
	err = bson.Unmarshal(bytes, &out)
	if err != nil {
		return nil, errors.Annotate(err, "bson unmarshaling failed")
	}
	return out, nil
}

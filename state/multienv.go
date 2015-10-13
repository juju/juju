// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"strings"

	"github.com/juju/errors"
	"gopkg.in/mgo.v2/bson"
)

// This file contains utility functions related to documents and
// collections that contain data for multiple environments.

// ensureEnvUUID returns an environment UUID prefixed document ID. The
// prefix is only added if it isn't already there.
func ensureEnvUUID(envUUID, id string) string {
	prefix := envUUID + ":"
	if strings.HasPrefix(id, prefix) {
		return id
	}
	return prefix + id
}

// ensureEnvUUIDIfString will call ensureEnvUUID, but only if the id
// is a string. The id will be left untouched otherwise.
func ensureEnvUUIDIfString(envUUID string, id interface{}) interface{} {
	if id, ok := id.(string); ok {
		return ensureEnvUUID(envUUID, id)
	}
	return id
}

// splitDocID returns the 2 parts of environment UUID prefixed
// document ID. If the id is not in the expected format the final
// return value will be false.
func splitDocID(id string) (string, string, bool) {
	parts := strings.SplitN(id, ":", 2)
	if len(parts) != 2 {
		return "", "", false
	}
	return parts[0], parts[1], true
}

const envUUIDRequired = 1
const noEnvUUIDInInput = 2

// mungeDocForMultiEnv takes the value of an txn.Op Insert or $set
// Update and modifies it to be multi-environment safe, returning the
// modified document.
func mungeDocForMultiEnv(doc interface{}, envUUID string, envUUIDFlags int) (bson.D, error) {
	var bDoc bson.D
	var err error
	if doc != nil {
		bDoc, err = toBsonD(doc)
		if err != nil {
			return nil, errors.Trace(err)
		}
	}

	envUUIDSeen := false
	for i, elem := range bDoc {
		switch elem.Name {
		case "_id":
			bDoc[i].Value = ensureEnvUUIDIfString(envUUID, elem.Value)
		case "env-uuid":
			if envUUIDFlags&noEnvUUIDInInput > 0 {
				return nil, errors.New("env-uuid is added automatically and should not be provided")
			}
			envUUIDSeen = true
			if elem.Value == "" {
				bDoc[i].Value = envUUID
			} else if elem.Value != envUUID {
				return nil, errors.Errorf(`bad "env-uuid" value: expected %s, got %s`, envUUID, elem.Value)
			}
		}
	}
	if envUUIDFlags&envUUIDRequired > 0 && !envUUIDSeen {
		bDoc = append(bDoc, bson.DocElem{"env-uuid", envUUID})
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

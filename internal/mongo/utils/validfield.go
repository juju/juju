// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package utils

import (
	"strings"

	"github.com/juju/errors"
	"github.com/juju/mgo/v3/bson"
)

// IsValidFieldName returns true if the given name is acceptable for
// use as a MongoDB field name.
func IsValidFieldName(name string) bool {
	if len(name) == 0 {
		return false
	}
	if strings.HasPrefix(name, "$") {
		return false
	}
	if strings.Contains(name, ".") {
		return false
	}
	return true
}

// CheckStorable returns an error if the given document - or any of
// it's subdocuments - contains field names which are not valid for
// storage into MongoDB.
func CheckStorable(inDoc interface{}) error {
	// Normalise document to bson.M
	bytes, err := bson.Marshal(inDoc)
	if err != nil {
		return errors.Annotate(err, "marshalling")
	}
	var doc bson.M
	if err := bson.Unmarshal(bytes, &doc); err != nil {
		return errors.Annotate(err, "unmarshalling")
	}
	// Check it.
	return errors.Trace(checkDoc(doc))
}

func checkDoc(doc bson.M) error {
	for name, value := range doc {
		if !IsValidFieldName(name) {
			return errors.Errorf("%q is not a valid field name", name)
		}
		// If the field is a subdocument, recurse into it.
		if subDoc, ok := value.(bson.M); ok {
			if err := checkDoc(subDoc); err != nil {
				return errors.Trace(err)
			}
		}
	}
	return nil
}

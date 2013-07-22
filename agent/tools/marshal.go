// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package tools

import (
	"labix.org/v2/mgo/bson"

	"launchpad.net/juju-core/version"
)

type toolsDoc struct {
	Version version.Binary
	URL     string
}

// GetBSON returns the structure to be serialized for the tools as a generic
// interface.
func (t *Tools) GetBSON() (interface{}, error) {
	if t == nil {
		return nil, nil
	}
	return &toolsDoc{t.Binary, t.URL}, nil
}

// SetBSON updates the internal members with the data stored in the bson.Raw
// parameter.
func (t *Tools) SetBSON(raw bson.Raw) error {
	if raw.Kind == 10 {
		// Preserve the nil value in that case.
		return bson.SetZero
	}
	var doc toolsDoc
	if err := raw.Unmarshal(&doc); err != nil {
		return err
	}
	t.Binary = doc.Version
	t.URL = doc.URL
	return nil
}

// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package tools

import (
	"github.com/juju/mgo/v3/bson"

	"github.com/juju/juju/core/semversion"
)

type toolsDoc struct {
	Version semversion.Binary
	URL     string
	Size    int64
	SHA256  string
}

// GetBSON returns the structure to be serialized for the tools as a generic
// interface.
func (t *Tools) GetBSON() (interface{}, error) {
	if t == nil {
		return nil, nil
	}
	return &toolsDoc{t.Version, t.URL, t.Size, t.SHA256}, nil
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
	t.Version = doc.Version
	t.URL = doc.URL
	t.Size = doc.Size
	t.SHA256 = doc.SHA256
	return nil
}

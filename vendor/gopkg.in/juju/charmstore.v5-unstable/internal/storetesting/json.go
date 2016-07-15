// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storetesting // import "gopkg.in/juju/charmstore.v5-unstable/internal/storetesting"

import (
	"bytes"
	"encoding/json"
	"io"
)

// MustMarshalJSON marshals the specified value using json.Marshal and
// returns the corresponding byte slice. If there is an error marshalling
// the value then MustMarshalJSON will panic.
func MustMarshalJSON(v interface{}) []byte {
	data, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return data
}

// JSONReader creates an io.Reader which can read the Marshalled value of v.
func JSONReader(v interface{}) io.Reader {
	return bytes.NewReader(MustMarshalJSON(v))
}

// Copyright 2014 ALTOROS
// Licensed under the AGPLv3, see LICENSE file for details.

package data

import (
	"encoding/json"
	"errors"
	"io"
)

// ReadJSON from io.Reader to the interface
func ReadJSON(r io.Reader, v interface{}) error {
	dec := json.NewDecoder(r)
	err := dec.Decode(v)
	if err == io.EOF {
		return nil
	}
	return err
}

type failReader struct{}

// Read implements test io.Reader interface always returning error.
func (failReader) Read(p []byte) (n int, err error) {
	return 0, errors.New("test error")
}

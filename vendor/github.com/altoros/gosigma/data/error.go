// Copyright 2014 ALTOROS
// Licensed under the AGPLv3, see LICENSE file for details.

package data

import (
	"bytes"
	"io"
	"io/ioutil"
	"strings"
)

// Error contains error information from server reply
type Error struct {
	Point   string `json:"error_point"`
	Type    string `json:"error_type"`
	Message string `json:"error_message"`
}

// ReadError reads and unmarshalls information about cloud server error JSON stream
func ReadError(r io.Reader) ([]Error, error) {
	bb, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, err
	}

	// try to read slice first (as JSON array)
	var ee []Error
	if err := ReadJSON(bytes.NewReader(bb), &ee); err == nil {
		return ee, nil
	}

	// try to read single object
	var e Error
	if err := ReadJSON(bytes.NewReader(bb), &e); err != nil {
		return nil, err
	}

	return []Error{e}, nil
}

func (e Error) Error() string {
	var rr []string
	if e.Point != "" {
		rr = append(rr, e.Point)
	}
	if e.Type != "" {
		rr = append(rr, e.Type)
	}
	if e.Message != "" {
		rr = append(rr, e.Message)
	}
	return strings.Join(rr, ", ")
}

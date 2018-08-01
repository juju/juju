// Copyright 2016 Canonical Ltd.
// Licensed under the LGPLv3, see LICENSE file for details.

package cmd

import (
	"errors"
	"strings"
)

// StringMap is a type that deserializes a CLI string using gnuflag's Value
// semantics.  It expects a key=value pair, and supports multiple copies of the
// flag adding more pairs, though the keys must be unique, and both keys and
// values must be non-empty.
type StringMap struct {
	Mapping *map[string]string
}

// Set implements gnuflag.Value's Set method.
func (m StringMap) Set(s string) error {
	if *m.Mapping == nil {
		*m.Mapping = map[string]string{}
	}
	// make a copy so the following code is less ugly with dereferencing.
	mapping := *m.Mapping

	// Note that gnuflag will prepend the bad argument to the error message, so
	// we don't need to restate it here.
	vals := strings.SplitN(s, "=", 2)
	if len(vals) != 2 {
		return errors.New("expected key=value format")
	}
	key, value := vals[0], vals[1]
	if len(key) == 0 || len(value) == 0 {
		return errors.New("key and value must be non-empty")
	}
	if _, ok := mapping[key]; ok {
		return errors.New("duplicate key specified")
	}
	mapping[key] = value
	return nil
}

// String implements gnuflag.Value's String method
func (m StringMap) String() string {
	pairs := make([]string, 0, len(*m.Mapping))
	for key, value := range *m.Mapping {
		pairs = append(pairs, key+"="+value)
	}
	return strings.Join(pairs, ";")
}

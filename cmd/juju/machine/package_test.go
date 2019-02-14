// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine_test

import (
	"encoding/json"
	"testing"

	gc "gopkg.in/check.v1"
)

// None of the tests in this package require mongo.

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

func unmarshalStringAsJSON(str string) (interface{}, error) {
	var v interface{}
	if err := json.Unmarshal([]byte(str), &v); err != nil {
		return struct{}{}, err
	}
	return v, nil
}

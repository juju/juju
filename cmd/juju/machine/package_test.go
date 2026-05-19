// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine_test

import (
	"encoding/json"
)

//go:generate go run github.com/canonical/gomock/mockgen -package mocks -destination mocks/removemachine_api_mock.go github.com/juju/juju/cmd/juju/machine RemoveMachineAPI
//go:generate go run github.com/canonical/gomock/mockgen -package mocks -destination mocks/modelconfig_api_mock.go github.com/juju/juju/cmd/juju/machine ModelConfigAPI

// None of the tests in this package require mongo.

func unmarshalStringAsJSON(str string) (any, error) {
	var v any
	if err := json.Unmarshal([]byte(str), &v); err != nil {
		return struct{}{}, err
	}
	return v, nil
}

// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine_test

import (
	"encoding/json"
	"testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/upgrademachine_api_mock.go github.com/juju/juju/cmd/juju/machine UpgradeMachineAPI
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/status_api_mock.go github.com/juju/juju/cmd/juju/machine StatusAPI
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/removemachine_api_mock.go github.com/juju/juju/cmd/juju/machine RemoveMachineAPI
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/modelconfig_api_mock.go github.com/juju/juju/cmd/juju/machine ModelConfigAPI

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

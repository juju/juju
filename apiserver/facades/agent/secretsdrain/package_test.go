// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretsdrain

import (
	"testing"

	"github.com/juju/tc"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/facade_mock.go github.com/juju/juju/apiserver/facade Authorizer

func TestPackage(t *testing.T) {
	tc.TestingT(t)
}

var NewSecretsDrainAPI = newSecretsDrainAPI

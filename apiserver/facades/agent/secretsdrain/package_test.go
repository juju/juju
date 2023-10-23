// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretsdrain

import (
	"testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/facade_mock.go github.com/juju/juju/apiserver/facade Context,Authorizer

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

var NewSecretsDrainAPI = newSecretsDrainAPI

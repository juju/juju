// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretexpire_test

import (
	stdtesting "testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/client_mock.go github.com/juju/juju/worker/secretexpire SecretManagerFacade

func TestPackage(t *stdtesting.T) {
	gc.TestingT(t)
}

// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodelsecrets_test

import (
	"testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/secretservice.go github.com/juju/juju/apiserver/facades/controller/crossmodelsecrets SecretService
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/statebackend.go github.com/juju/juju/apiserver/facades/controller/crossmodelsecrets StateBackend
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/crossmodel.go github.com/juju/juju/apiserver/facades/controller/crossmodelsecrets CrossModelState

func TestAll(t *testing.T) {
	gc.TestingT(t)
}

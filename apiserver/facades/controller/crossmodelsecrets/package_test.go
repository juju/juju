// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodelsecrets_test

import (
	"testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/secretsstate.go github.com/juju/juju/apiserver/facades/controller/crossmodelsecrets SecretsState
//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/statebackend.go github.com/juju/juju/apiserver/facades/controller/crossmodelsecrets StateBackend
//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/secretsconsumer.go github.com/juju/juju/apiserver/facades/controller/crossmodelsecrets SecretsConsumer
//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/crossmodel.go github.com/juju/juju/apiserver/facades/controller/crossmodelsecrets CrossModelState

func TestAll(t *testing.T) {
	gc.TestingT(t)
}

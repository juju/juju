// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client_test

import (
	stdtesting "testing"

	coretesting "github.com/juju/juju/testing"
)

func Test(t *stdtesting.T) {
	coretesting.MgoTestPackage(t)
}

//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/client_mock.go github.com/juju/juju/apiserver/facades/client/client Backend,Model
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/facade_mock.go github.com/juju/juju/apiserver/facade Authorizer
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/common_mock.go github.com/juju/juju/apiserver/common ToolsFinder

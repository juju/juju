// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelcmd_test

import (
	stdtesting "testing"

	"github.com/juju/tc"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/modelconfig_mock.go github.com/juju/juju/cmd/modelcmd ModelConfigAPI
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/modelconfig_mock.go github.com/juju/juju/cmd/modelcmd ModelConfigAPI
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/api_mock.go github.com/juju/juju/api Connection
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/sessionloginfactory_mock.go github.com/juju/juju/cmd/modelcmd SessionLoginFactory
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/loginprovider_mock.go github.com/juju/juju/api LoginProvider

func Test(t *stdtesting.T) {
	tc.TestingT(t)
}

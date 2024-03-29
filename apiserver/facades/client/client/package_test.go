// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client_test

import (
	stdtesting "testing"

	coretesting "github.com/juju/juju/testing"
)

//go:generate go run go.uber.org/mock/mockgen -package client_test -destination package_mock_test.go github.com/juju/juju/apiserver/facades/client/client Backend,Model,BlockDeviceService,ControllerConfigService,NetworkService
//go:generate go run go.uber.org/mock/mockgen -package client_test -destination facade_mock_test.go github.com/juju/juju/apiserver/facade Authorizer
//go:generate go run go.uber.org/mock/mockgen -package client_test -destination common_mock_test.go github.com/juju/juju/apiserver/common ToolsFinder
func TestPackage(t *stdtesting.T) {
	coretesting.MgoTestPackage(t)
}

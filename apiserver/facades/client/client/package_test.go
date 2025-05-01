// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client_test

import (
	stdtesting "testing"

	coretesting "github.com/juju/juju/internal/testing"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package client_test -destination package_mock_test.go github.com/juju/juju/apiserver/facades/client/client Backend
//go:generate go run go.uber.org/mock/mockgen -typed -package client_test -destination facade_mock_test.go github.com/juju/juju/apiserver/facade Authorizer
//go:generate go run go.uber.org/mock/mockgen -typed -package client_test -destination common_mock_test.go github.com/juju/juju/apiserver/common ToolsFinder
//go:generate go run go.uber.org/mock/mockgen -typed -package client -destination service_mock_test.go github.com/juju/juju/apiserver/facades/client/client BlockDeviceService,NetworkService,ModelInfoService,RelationService,StatusService
//go:generate go run go.uber.org/mock/mockgen -typed -package client -destination authorizer_mock_test.go github.com/juju/juju/apiserver/facade Authorizer

func TestPackage(t *stdtesting.T) {
	coretesting.MgoTestPackage(t)
}

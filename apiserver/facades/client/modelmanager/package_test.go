// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmanager

import (
	stdtesting "testing"

	"github.com/juju/juju/testing"
)

//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/common_mock.go github.com/juju/juju/apiserver/common BlockCheckerInterface
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/domain_mock.go github.com/juju/juju/apiserver/common ControllerConfigService
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/migrator_mock.go github.com/juju/juju/apiserver/facades/client/modelmanager ModelExporter

func TestAll(t *stdtesting.T) {
	testing.MgoTestPackage(t)
}

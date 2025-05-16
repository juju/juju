// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	stdtesting "testing"

	"github.com/juju/tc"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package service -destination package_mock_test.go github.com/juju/juju/domain/network/service State,ProviderWithNetworking,ProviderWithZones

func TestPackage(t *stdtesting.T) {
	tc.TestingT(t)
}

// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	stdtesting "testing"

	"github.com/juju/tc"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package service -destination service_mock_test.go github.com/juju/juju/domain/modelmigration/service InstanceProvider,ResourceProvider,State

func TestPackage(t *stdtesting.T) {
	tc.TestingT(t)
}

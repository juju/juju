// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -package service -destination package_mock_test.go github.com/juju/juju/domain/application/service State,Charm
//go:generate go run go.uber.org/mock/mockgen -package service -destination status_mock_test.go github.com/juju/juju/core/status StatusHistoryFactory,StatusHistorySetter

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package application -destination package_mock_test.go github.com/juju/juju/domain/application/service ExecTokenProvider,CAASApplicationProvider
//go:generate go run go.uber.org/mock/mockgen -typed -package application -destination caas_mock_test.go github.com/juju/juju/caas Application
//go:generate go run go.uber.org/mock/mockgen -typed -package application -destination leadership_mock_test.go github.com/juju/juju/core/leadership Revoker
//go:generate go run go.uber.org/mock/mockgen -typed -package application_test -destination lease_mock_test.go github.com/juju/juju/core/lease Checker

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

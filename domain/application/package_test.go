// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	stdtesting "testing"

	"github.com/juju/tc"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package application -destination package_mock_test.go github.com/juju/juju/domain/application/service Broker
//go:generate go run go.uber.org/mock/mockgen -typed -package application -destination caas_mock_test.go github.com/juju/juju/caas Application
//go:generate go run go.uber.org/mock/mockgen -typed -package application -destination leadership_mock_test.go github.com/juju/juju/core/leadership Revoker
//go:generate go run go.uber.org/mock/mockgen -typed -package application_test -destination lease_mock_test.go github.com/juju/juju/core/lease Checker


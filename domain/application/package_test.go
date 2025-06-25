// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application_test

//go:generate go run go.uber.org/mock/mockgen -typed -package application -destination caas_mock_test.go github.com/juju/juju/caas Application
//go:generate go run go.uber.org/mock/mockgen -typed -package application -destination leadership_mock_test.go github.com/juju/juju/core/leadership Revoker
//go:generate go run go.uber.org/mock/mockgen -typed -package application_test -destination lease_mock_test.go github.com/juju/juju/core/lease Checker
//go:generate go run go.uber.org/mock/mockgen -typed -package application_test -destination provider_mock_test.go github.com/juju/juju/domain/application/service Provider,CAASProvider

func ptr[T any](v T) *T {
	return &v
}

// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

//go:generate go run go.uber.org/mock/mockgen -typed -package service -destination package_mock_test.go github.com/juju/juju/domain/network/service State,ProviderWithNetworking,ProviderWithZones
//go:generate go run go.uber.org/mock/mockgen -typed -package service -destination internal_mock_test.go github.com/juju/juju/domain/network/internal CheckableMachine

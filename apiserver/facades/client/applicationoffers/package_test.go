// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package applicationoffers

//go:generate go run go.uber.org/mock/mockgen -typed -package applicationoffers -destination facade_mock_test.go github.com/juju/juju/apiserver/facade Authorizer
//go:generate go run go.uber.org/mock/mockgen -typed -package applicationoffers -destination package_mock_test.go github.com/juju/juju/apiserver/facades/client/applicationoffers AccessService,ModelService,OfferService

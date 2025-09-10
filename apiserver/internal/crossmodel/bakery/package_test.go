// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bakery

//go:generate go run go.uber.org/mock/mockgen -typed -package bakery -destination clock_mock_test.go github.com/juju/clock Clock
//go:generate go run go.uber.org/mock/mockgen -typed -package bakery -destination service_mock_test.go github.com/juju/juju/apiserver/internal/crossmodel/bakery BakeryStore,Oven
//go:generate go run go.uber.org/mock/mockgen -typed -package bakery -destination bakery_mock_test.go github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery OpsAuthorizer

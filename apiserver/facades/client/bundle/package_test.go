// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bundle_test

//go:generate go run go.uber.org/mock/mockgen -typed -package bundle_test -destination service_mock_test.go github.com/juju/juju/apiserver/facades/client/bundle NetworkService,ApplicationService
//go:generate go run go.uber.org/mock/mockgen -typed -package bundle_test -destination charm_mock_test.go github.com/juju/juju/internal/charm Charm

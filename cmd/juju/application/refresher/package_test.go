// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package refresher

import (
	stdtesting "testing"

	"github.com/juju/tc"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package refresher -destination refresher_mock_test.go github.com/juju/juju/cmd/juju/application/refresher RefresherFactory,Refresher,CharmResolver,CharmRepository
//go:generate go run go.uber.org/mock/mockgen -typed -package refresher -destination store_mock_test.go github.com/juju/juju/cmd/juju/application/store CharmAdder
//go:generate go run go.uber.org/mock/mockgen -typed -package refresher -destination charm_mock_test.go github.com/juju/juju/internal/charm Charm

func TestPackage(t *stdtesting.T) {
	tc.TestingT(t)
}

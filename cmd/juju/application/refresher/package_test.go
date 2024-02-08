// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package refresher

import (
	"testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -package refresher -destination refresher_mock_test.go github.com/juju/juju/cmd/juju/application/refresher RefresherFactory,Refresher,CharmResolver,CharmRepository
//go:generate go run go.uber.org/mock/mockgen -package refresher -destination store_mock_test.go github.com/juju/juju/cmd/juju/application/store CharmAdder
//go:generate go run go.uber.org/mock/mockgen -package refresher -destination charm_mock_test.go github.com/juju/charm/v13 Charm

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

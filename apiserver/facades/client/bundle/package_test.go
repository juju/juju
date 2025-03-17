// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bundle_test

import (
	stdtesting "testing"

	"github.com/juju/juju/internal/testing"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package bundle -destination service_mock_test.go github.com/juju/juju/apiserver/facades/client/bundle NetworkService,ApplicationService
//go:generate go run go.uber.org/mock/mockgen -typed -package bundle -destination migration_mock_test.go github.com/juju/juju/apiserver/facades/client/bundle ModelMigrationFactory
//go:generate go run go.uber.org/mock/mockgen -typed -package bundle -destination facade_mock_test.go github.com/juju/juju/apiserver/facade ModelExporter
//go:generate go run go.uber.org/mock/mockgen -typed -package bundle -destination charm_mock_test.go github.com/juju/juju/internal/charm Charm
//go:generate go run go.uber.org/mock/mockgen -typed -package bundle -destination objectstore_mock_test.go github.com/juju/juju/core/objectstore ObjectStore
func Test(t *stdtesting.T) {
	testing.MgoTestPackage(t)
}

// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package stub

import (
	"testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package stub -destination storage_mock_test.go github.com/juju/juju/core/storage ModelStorageRegistryGetter
//go:generate go run go.uber.org/mock/mockgen -typed -package stub -destination internal_storage_mock_test.go github.com/juju/juju/internal/storage ProviderRegistry

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

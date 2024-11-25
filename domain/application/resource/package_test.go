// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resource

import (
	"testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package resource -destination object_store_mock_test.go github.com/juju/juju/core/objectstore ObjectStore,ModelObjectStoreGetter

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

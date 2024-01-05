// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package objectstore

import (
	stdtesting "testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -package objectstore -destination state_mock_test.go github.com/juju/juju/internal/objectstore MongoSession,Locker,LockExtender
//go:generate go run go.uber.org/mock/mockgen -package objectstore -destination objectstore_mock_test.go github.com/juju/juju/core/objectstore ObjectStoreMetadata

func TestAll(t *stdtesting.T) {
	gc.TestingT(t)
}

// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package objectstore

import (
	stdtesting "testing"

	"go.uber.org/goleak"
	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -package objectstore -destination state_mock_test.go github.com/juju/juju/internal/objectstore MongoSession,Claimer,ClaimExtender
//go:generate go run go.uber.org/mock/mockgen -package objectstore -destination objectstore_mock_test.go github.com/juju/juju/core/objectstore ObjectStoreMetadata
//go:generate go run go.uber.org/mock/mockgen -package objectstore -destination clock_mock_test.go github.com/juju/clock Clock

func TestAll(t *stdtesting.T) {
	defer goleak.VerifyNone(t)

	gc.TestingT(t)
}

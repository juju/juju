// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package manifold_test

import (
	stdtesting "testing"

	coretesting "github.com/juju/juju/testing"
)

//go:generate go run github.com/golang/mock/mockgen -package manifold_test -destination db_mock_test.go github.com/juju/juju/core/db TrackedDB

func TestPackage(t *stdtesting.T) {
	coretesting.MgoTestPackage(t)
}

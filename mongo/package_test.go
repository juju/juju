// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package mongo_test

import (
	stdtesting "testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run github.com/golang/mock/mockgen -package mongotest -destination mongotest/mongoservice_mock.go github.com/juju/juju/mongo MongoSnapService

func Test(t *stdtesting.T) {
	gc.TestingT(t)
}

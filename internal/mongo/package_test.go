// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package mongo_test

import (
	stdtesting "testing"

	"github.com/juju/tc"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package mongotest -destination mongotest/mongoservice_mock.go github.com/juju/juju/internal/mongo MongoSnapService

func Test(t *stdtesting.T) {
	tc.TestingT(t)
}

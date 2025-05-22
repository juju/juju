// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package mongo_test

//go:generate go run go.uber.org/mock/mockgen -typed -package mongotest -destination mongotest/mongoservice_mock.go github.com/juju/juju/internal/mongo MongoSnapService

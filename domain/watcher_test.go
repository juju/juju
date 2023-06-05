// Copyright 2033 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package domain_test

import (
	"errors"

	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/domain"
	gc "gopkg.in/check.v1"
)

type watcherSuite struct{}

var (
	_           = gc.Suite(&watcherSuite{})
	failGetDBFn = func() (changestream.WatchableDB, error) {
		return nil, errors.New("fail getting db instance")
	}
)

func (*watcherSuite) TestNewUUIDsWatcherFail(c *gc.C) {
	factory := domain.NewWatcherFactory(failGetDBFn, nil)

	_, err := factory.NewUUIDsWatcher(changestream.All, "random_namespace")
	c.Assert(err, gc.ErrorMatches, "creating UUID watcher on namespace random_namespace: fail getting db instance")
}

func (*watcherSuite) TestNewKeysWatcherFail(c *gc.C) {
	factory := domain.NewWatcherFactory(failGetDBFn, nil)

	_, err := factory.NewKeysWatcher(changestream.All, "random_namespace", "key_value")
	c.Assert(err, gc.ErrorMatches, "creating keys watcher on namespace random_namespace and key key_value: fail getting db instance")
}

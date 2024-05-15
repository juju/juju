// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package objectstore_test

import (
	"context"
	"time"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/domain"
	objectstoreerrors "github.com/juju/juju/domain/objectstore/errors"
	"github.com/juju/juju/domain/objectstore/service"
	"github.com/juju/juju/domain/objectstore/state"
	changestreamtesting "github.com/juju/juju/internal/changestream/testing"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/testing"
)

type watcherSuite struct {
	changestreamtesting.ControllerSuite
}

var _ = gc.Suite(&watcherSuite{})

func (s *watcherSuite) TestWatchWithAdd(c *gc.C) {
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "objectstore")

	svc := service.NewWatchableService(state.NewState(func() (database.TxnRunner, error) { return factory() }),
		domain.NewWatcherFactory(factory,
			loggertesting.WrapCheckLog(c),
		),
	)
	watcher, err := svc.Watch()
	c.Assert(err, jc.ErrorIsNil)

	// Wait for the initial change.
	select {
	case <-watcher.Changes():
	case <-time.After(testing.LongWait):
		c.Fatalf("timed out waiting for change")
	}

	// Add a new object.
	metadata := objectstore.Metadata{
		Path: "foo",
		Hash: "hash",
		Size: 666,
	}
	err = svc.PutMetadata(context.Background(), metadata)
	c.Assert(err, jc.ErrorIsNil)

	// Get the change.
	select {
	case change := <-watcher.Changes():
		c.Assert(change, gc.DeepEquals, []string{metadata.Path})
	case <-time.After(testing.LongWait):
		c.Fatalf("timed out waiting for change")
	}
}

func (s *watcherSuite) TestWatchWithDelete(c *gc.C) {
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "objectstore")

	svc := service.NewWatchableService(state.NewState(func() (database.TxnRunner, error) { return factory() }),
		domain.NewWatcherFactory(factory,
			loggertesting.WrapCheckLog(c),
		),
	)
	watcher, err := svc.Watch()
	c.Assert(err, jc.ErrorIsNil)

	// Wait for the initial change.
	select {
	case <-watcher.Changes():
	case <-time.After(testing.LongWait):
		c.Fatalf("timed out waiting for change")
	}

	// Add a new object.
	metadata := objectstore.Metadata{
		Path: "foo",
		Hash: "hash",
		Size: 666,
	}
	err = svc.PutMetadata(context.Background(), metadata)
	c.Assert(err, jc.ErrorIsNil)

	// Get the change.
	select {
	case change := <-watcher.Changes():
		c.Assert(change, gc.DeepEquals, []string{metadata.Path})
	case <-time.After(testing.LongWait):
		c.Fatalf("timed out waiting for change")
	}

	// Remove the object.
	err = svc.RemoveMetadata(context.Background(), metadata.Path)
	c.Assert(err, jc.ErrorIsNil)

	// Get the change.
	select {
	case change := <-watcher.Changes():
		c.Assert(change, gc.DeepEquals, []string{metadata.Path})
	case <-time.After(testing.LongWait):
		c.Fatalf("timed out waiting for change")
	}

	_, err = svc.GetMetadata(context.Background(), metadata.Path)
	c.Assert(err, jc.ErrorIs, objectstoreerrors.ErrNotFound)
}

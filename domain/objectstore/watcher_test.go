// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package objectstore_test

import (
	stdtesting "testing"
	"time"

	"github.com/juju/tc"

	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/domain"
	objectstoreerrors "github.com/juju/juju/domain/objectstore/errors"
	"github.com/juju/juju/domain/objectstore/service"
	"github.com/juju/juju/domain/objectstore/state"
	changestreamtesting "github.com/juju/juju/internal/changestream/testing"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testing"
)

type watcherSuite struct {
	changestreamtesting.ControllerSuite
}

func TestWatcherSuite(t *stdtesting.T) { tc.Run(t, &watcherSuite{}) }
func (s *watcherSuite) TestWatchWithAdd(c *tc.C) {
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "objectstore")

	svc := service.NewWatchableService(state.NewState(func() (database.TxnRunner, error) { return factory() }),
		domain.NewWatcherFactory(factory,
			loggertesting.WrapCheckLog(c),
		),
	)
	watcher, err := svc.Watch()
	c.Assert(err, tc.ErrorIsNil)

	// Wait for the initial change.
	select {
	case <-watcher.Changes():
	case <-time.After(testing.LongWait):
		c.Fatalf("timed out waiting for change")
	}

	// Add a new object.
	metadata := objectstore.Metadata{
		Path:   "foo",
		SHA256: "hash256",
		SHA384: "hash384",
		Size:   666,
	}
	_, err = svc.PutMetadata(c.Context(), metadata)
	c.Assert(err, tc.ErrorIsNil)

	// Get the change.
	select {
	case change := <-watcher.Changes():
		c.Assert(change, tc.DeepEquals, []string{metadata.Path})
	case <-time.After(testing.LongWait):
		c.Fatalf("timed out waiting for change")
	}
}

func (s *watcherSuite) TestWatchWithDelete(c *tc.C) {
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "objectstore")

	svc := service.NewWatchableService(state.NewState(func() (database.TxnRunner, error) { return factory() }),
		domain.NewWatcherFactory(factory,
			loggertesting.WrapCheckLog(c),
		),
	)
	watcher, err := svc.Watch()
	c.Assert(err, tc.ErrorIsNil)

	// Wait for the initial change.
	select {
	case <-watcher.Changes():
	case <-time.After(testing.LongWait):
		c.Fatalf("timed out waiting for change")
	}

	// Add a new object.
	metadata := objectstore.Metadata{
		Path:   "foo",
		SHA256: "hash256",
		SHA384: "hash384",
		Size:   666,
	}
	_, err = svc.PutMetadata(c.Context(), metadata)
	c.Assert(err, tc.ErrorIsNil)

	// Get the change.
	select {
	case change := <-watcher.Changes():
		c.Assert(change, tc.DeepEquals, []string{metadata.Path})
	case <-time.After(testing.LongWait):
		c.Fatalf("timed out waiting for change")
	}

	// Remove the object.
	err = svc.RemoveMetadata(c.Context(), metadata.Path)
	c.Assert(err, tc.ErrorIsNil)

	// Get the change.
	select {
	case change := <-watcher.Changes():
		c.Assert(change, tc.DeepEquals, []string{metadata.Path})
	case <-time.After(testing.LongWait):
		c.Fatalf("timed out waiting for change")
	}

	_, err = svc.GetMetadata(c.Context(), metadata.Path)
	c.Assert(err, tc.ErrorIs, objectstoreerrors.ErrNotFound)
}

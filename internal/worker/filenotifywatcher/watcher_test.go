// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package filenotifywatcher

import (
	"fmt"
	"os"
	"path/filepath"
	stdtesting "testing"
	time "time"

	"github.com/juju/tc"
	"github.com/juju/worker/v4/workertest"

	"github.com/juju/juju/internal/testing"
)

type watcherSuite struct {
	baseSuite
}

func TestWatcherSuite(t *stdtesting.T) {
	tc.Run(t, &watcherSuite{})
}

func (s *watcherSuite) TestWatching(c *tc.C) {
	dir, err := os.MkdirTemp("", "inotify")
	c.Assert(err, tc.ErrorIsNil)
	defer os.RemoveAll(dir)

	w, err := NewWatcher("controller", WithPath(dir))
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	file := filepath.Join(dir, "controller")
	_, err = os.OpenFile(file, os.O_WRONLY|os.O_CREATE, 0666)
	c.Assert(err, tc.ErrorIsNil)

	defer os.Remove(file)

	select {
	case change := <-w.Changes():
		c.Assert(change, tc.IsTrue)
	case <-time.After(testing.LongWait):
		c.Fatalf("timed out waiting for file create changes")
	}

	err = os.Remove(file)
	c.Assert(err, tc.ErrorIsNil)

	select {
	case change := <-w.Changes():
		c.Assert(change, tc.IsFalse)
	case <-time.After(testing.LongWait):
		c.Fatalf("timed out waiting for file delete changes")
	}

	workertest.CleanKill(c, w)
}

func (s *watcherSuite) TestNotWatching(c *tc.C) {
	dir, err := os.MkdirTemp("", "inotify")
	c.Assert(err, tc.ErrorIsNil)
	defer os.RemoveAll(dir)

	w, err := NewWatcher("controller", WithPath(dir))
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	file := filepath.Join(dir, "controller")
	f, err := os.OpenFile(file, os.O_WRONLY|os.O_CREATE, 0666)
	c.Assert(err, tc.ErrorIsNil)

	defer os.Remove(file)

	select {
	case change := <-w.Changes():
		c.Assert(change, tc.IsTrue)
	case <-time.After(testing.LongWait):
		c.Fatalf("timed out waiting for file create changes")
	}

	_, err = fmt.Fprintln(f, "hello world")
	c.Assert(err, tc.ErrorIsNil)

	select {
	case <-w.Changes():
		c.Fatalf("unexpected change")
	case <-time.After(time.Second):
	}

	err = os.Remove(file)
	c.Assert(err, tc.ErrorIsNil)

	select {
	case change := <-w.Changes():
		c.Assert(change, tc.IsFalse)
	case <-time.After(testing.LongWait):
		c.Fatalf("timed out waiting for file delete changes")
	}

	workertest.CleanKill(c, w)
}

// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package filenotifywatcher

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	time "time"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4/workertest"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/internal/testing"
)

type watcherSuite struct {
	baseSuite
}

var _ = gc.Suite(&watcherSuite{})

func (s *watcherSuite) TestWatching(c *gc.C) {
	dir, err := os.MkdirTemp("", "inotify")
	c.Assert(err, jc.ErrorIsNil)
	defer os.RemoveAll(dir)

	w, err := NewWatcher(context.Background(), "controller", WithPath(dir))
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	file := filepath.Join(dir, "controller")
	_, err = os.OpenFile(file, os.O_WRONLY|os.O_CREATE, 0666)
	c.Assert(err, jc.ErrorIsNil)

	defer os.Remove(file)

	select {
	case change := <-w.Changes():
		c.Assert(change, jc.IsTrue)
	case <-time.After(testing.LongWait):
		c.Fatalf("timed out waiting for file create changes")
	}

	err = os.Remove(file)
	c.Assert(err, jc.ErrorIsNil)

	select {
	case change := <-w.Changes():
		c.Assert(change, jc.IsFalse)
	case <-time.After(testing.LongWait):
		c.Fatalf("timed out waiting for file delete changes")
	}

	workertest.CleanKill(c, w)
}

func (s *watcherSuite) TestNotWatching(c *gc.C) {
	dir, err := os.MkdirTemp("", "inotify")
	c.Assert(err, jc.ErrorIsNil)
	defer os.RemoveAll(dir)

	w, err := NewWatcher(context.Background(), "controller", WithPath(dir))
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	file := filepath.Join(dir, "controller")
	f, err := os.OpenFile(file, os.O_WRONLY|os.O_CREATE, 0666)
	c.Assert(err, jc.ErrorIsNil)

	defer os.Remove(file)

	select {
	case change := <-w.Changes():
		c.Assert(change, jc.IsTrue)
	case <-time.After(testing.LongWait):
		c.Fatalf("timed out waiting for file create changes")
	}

	_, err = fmt.Fprintln(f, "hello world")
	c.Assert(err, jc.ErrorIsNil)

	select {
	case <-w.Changes():
		c.Fatalf("unexpected change")
	case <-time.After(time.Second):
	}

	err = os.Remove(file)
	c.Assert(err, jc.ErrorIsNil)

	select {
	case change := <-w.Changes():
		c.Assert(change, jc.IsFalse)
	case <-time.After(testing.LongWait):
		c.Fatalf("timed out waiting for file delete changes")
	}

	workertest.CleanKill(c, w)
}

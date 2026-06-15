// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backends

import (
	"testing"
	"time"

	"github.com/juju/loggo/v3"
	"github.com/juju/tc"
	"github.com/juju/worker/v5/workertest"

	"github.com/juju/juju/internal/worker/logsender"
)

type drainSuite struct{}

func TestDrainSuite(t *testing.T) {
	tc.Run(t, &drainSuite{})
}

func (s *drainSuite) TestDrainsRecords(c *tc.C) {
	w, err := NewDrain(1)
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.DirtyKill(c, w)

	records := w.LogRecords()
	records <- &logsender.LogRecord{
		Time:    time.Now(),
		Module:  "test.module",
		Level:   loggo.INFO,
		Message: "first",
	}

	select {
	case records <- &logsender.LogRecord{
		Time:    time.Now(),
		Module:  "test.module",
		Level:   loggo.INFO,
		Message: "second",
	}:
	case <-c.Context().Done():
		c.Fatalf("timed out waiting for drain backend to consume records")
	}

	workertest.CleanKill(c, w)
}

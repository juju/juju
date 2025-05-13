// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logsendertest

import (
	"reflect"

	"github.com/juju/tc"

	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/worker/logsender"
)

// ExpectLogStats waits for the buffered log writer's
// statistics to match the expected value. This is
// provided because statistics are updated after
// log messages are handed off to the sink, and so
// tests must cater for the gap or races will occur.
func ExpectLogStats(c *tc.C, writer *logsender.BufferedLogWriter, expect logsender.LogStats) {
	var stats logsender.LogStats
	for a := testing.LongAttempt.Start(); a.Next(); {
		stats = writer.Stats()
		if reflect.DeepEqual(stats, expect) {
			return
		}
	}
	c.Errorf("timed out waiting for statistics: got %+v, expected %+v", stats, expect)
}

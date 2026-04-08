// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmrunner

import (
	"io"
	"sync"
	stdtesting "testing"

	"github.com/juju/tc"
)

type hookLoggerSuite struct{}

func TestHookLoggerSuite(t *stdtesting.T) {
	tc.Run(t, &hookLoggerSuite{})
}

func (*hookLoggerSuite) TestStopIsIdempotent(c *tc.C) {
	reader, writer := io.Pipe()
	logger := NewHookLogger(reader)

	runDone := make(chan struct{})
	go func() {
		defer close(runDone)
		logger.Run()
	}()

	stopDone := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			logger.Stop()
		}()
	}
	go func() {
		defer close(stopDone)
		wg.Wait()
	}()

	select {
	case <-stopDone:
	case <-c.Context().Done():
		c.Fatalf("timed out waiting for Stop calls to finish")
	}

	c.Assert(writer.Close(), tc.ErrorIsNil)

	select {
	case <-runDone:
	case <-c.Context().Done():
		c.Fatalf("timed out waiting for logger to stop")
	}
}

// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package toolsversionchecker_test

import (
	"context"
	"time"

	"github.com/juju/tc"

	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/worker/toolsversionchecker"
)

var _ = tc.Suite(&ToolsCheckerSuite{})

type ToolsCheckerSuite struct {
	coretesting.BaseSuite
}

type facade struct {
	called chan string
}

func (f *facade) UpdateToolsVersion(ctx context.Context) error {
	f.called <- "UpdateToolsVersion"
	return nil
}

func newFacade() *facade {
	f := &facade{
		called: make(chan string, 1),
	}
	return f
}

func (s *ToolsCheckerSuite) TestWorker(c *tc.C) {
	f := newFacade()
	params := &toolsversionchecker.VersionCheckerParams{
		CheckInterval: coretesting.ShortWait,
	}

	checker := toolsversionchecker.NewPeriodicWorkerForTests(
		f,
		params,
	)
	s.AddCleanup(func(c *tc.C) {
		checker.Kill()
		c.Assert(checker.Wait(), tc.ErrorIsNil)
	})

	select {
	case called := <-f.called:
		c.Assert(called, tc.Equals, "UpdateToolsVersion")
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting worker to seek new agent binaries versions")
	}

}

// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package toolsversionchecker_test

import (
	"time"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/internal/worker/toolsversionchecker"
	coretesting "github.com/juju/juju/testing"
)

var _ = gc.Suite(&ToolsCheckerSuite{})

type ToolsCheckerSuite struct {
	coretesting.BaseSuite
}

type facade struct {
	called chan string
}

func (f *facade) UpdateToolsVersion() error {
	f.called <- "UpdateToolsVersion"
	return nil
}

func newFacade() *facade {
	f := &facade{
		called: make(chan string, 1),
	}
	return f
}

func (s *ToolsCheckerSuite) TestWorker(c *gc.C) {
	f := newFacade()
	params := &toolsversionchecker.VersionCheckerParams{
		CheckInterval: coretesting.ShortWait,
	}

	checker := toolsversionchecker.NewPeriodicWorkerForTests(
		f,
		params,
	)
	s.AddCleanup(func(c *gc.C) {
		checker.Kill()
		c.Assert(checker.Wait(), jc.ErrorIsNil)
	})

	select {
	case called := <-f.called:
		c.Assert(called, gc.Equals, "UpdateToolsVersion")
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting worker to seek new agent binaries versions")
	}

}

// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package workertest_test

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/testing"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/worker/workertest"
)

type Suite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&Suite{})

func (s *Suite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.PatchValue(workertest.KillTimeout, time.Second)
}

func (s *Suite) CheckFailed(c *gc.C) {
	if c.Failed() {
		c.Succeed()
	} else {
		c.Errorf("expected failure; none observed")
	}
	c.Logf("-------------------------------")
}

func (s *Suite) TestCheckAliveSuccess(c *gc.C) {
	w := workertest.NewErrorWorker(nil)
	defer workertest.CleanKill(c, w)

	workertest.CheckAlive(c, w)
}

func (s *Suite) TestCheckAliveFailure(c *gc.C) {
	w := workertest.NewDeadWorker(nil)

	workertest.CheckAlive(c, w)
	s.CheckFailed(c)
}

func (s *Suite) TestCheckKilledSuccess(c *gc.C) {
	expect := errors.New("snifplog")
	w := workertest.NewErrorWorker(expect)
	defer workertest.DirtyKill(c, w)

	w.Kill()
	err := workertest.CheckKilled(c, w)
	c.Check(err, gc.Equals, expect)
}

func (s *Suite) TestCheckKilledTimeout(c *gc.C) {
	w := workertest.NewErrorWorker(nil)
	defer workertest.CleanKill(c, w)

	err := workertest.CheckKilled(c, w)
	s.CheckFailed(c)
	c.Check(err, gc.ErrorMatches, "workertest: worker not stopping")
}

func (s *Suite) TestCheckKillSuccess(c *gc.C) {
	expect := errors.New("fledbon")
	w := workertest.NewErrorWorker(expect)
	defer workertest.DirtyKill(c, w)

	err := workertest.CheckKill(c, w)
	c.Check(err, gc.Equals, expect)
}

func (s *Suite) TestCheckKillTimeout(c *gc.C) {
	w := workertest.NewForeverWorker(nil)
	defer w.ReallyKill()

	err := workertest.CheckKill(c, w)
	s.CheckFailed(c)
	c.Check(err, gc.ErrorMatches, "workertest: worker not stopping")
}

func (s *Suite) TestCleanKillSuccess(c *gc.C) {
	w := workertest.NewErrorWorker(nil)

	workertest.CleanKill(c, w)
}

func (s *Suite) TestCleanKillFailure(c *gc.C) {
	w := workertest.NewErrorWorker(errors.New("kebdrix"))

	workertest.CleanKill(c, w)
	s.CheckFailed(c)
}

func (s *Suite) TestCleanKillTimeout(c *gc.C) {
	w := workertest.NewForeverWorker(nil)
	defer w.ReallyKill()

	workertest.CleanKill(c, w)
	s.CheckFailed(c)
}

func (s *Suite) TestDirtyKillSuccess(c *gc.C) {
	w := workertest.NewErrorWorker(errors.New("hifstit"))

	workertest.DirtyKill(c, w)
}

func (s *Suite) TestDirtyKillTimeout(c *gc.C) {
	w := workertest.NewForeverWorker(nil)
	defer w.ReallyKill()

	workertest.DirtyKill(c, w)
	s.CheckFailed(c)
}

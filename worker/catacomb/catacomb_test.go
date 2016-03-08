// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package catacomb_test

import (
	"sync"

	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"launchpad.net/tomb"

	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/catacomb"
)

type CatacombSuite struct {
	testing.IsolationSuite
	fix *fixture
}

var _ = gc.Suite(&CatacombSuite{})

func (s *CatacombSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.fix = &fixture{cleaner: s}
}

func (s *CatacombSuite) TestStartsAlive(c *gc.C) {
	s.fix.run(c, func() {
		s.fix.assertNotDying(c)
		s.fix.assertNotDead(c)
	})
}

func (s *CatacombSuite) TestKillClosesDying(c *gc.C) {
	s.fix.run(c, func() {
		s.fix.catacomb.Kill(nil)
		s.fix.assertDying(c)
	})
}

func (s *CatacombSuite) TestKillDoesNotCloseDead(c *gc.C) {
	s.fix.run(c, func() {
		s.fix.catacomb.Kill(nil)
		s.fix.assertNotDead(c)
	})
}

func (s *CatacombSuite) TestFinishTaskStopsCompletely(c *gc.C) {
	s.fix.run(c, func() {})

	s.fix.assertDying(c)
	s.fix.assertDead(c)
}

func (s *CatacombSuite) TestKillNil(c *gc.C) {
	err := s.fix.run(c, func() {
		s.fix.catacomb.Kill(nil)
	})
	c.Check(err, jc.ErrorIsNil)
}

func (s *CatacombSuite) TestKillNonNilOverwritesNil(c *gc.C) {
	second := errors.New("blah")

	err := s.fix.run(c, func() {
		s.fix.catacomb.Kill(nil)
		s.fix.catacomb.Kill(second)
	})
	c.Check(err, gc.Equals, second)
}

func (s *CatacombSuite) TestKillNilDoesNotOverwriteNonNil(c *gc.C) {
	first := errors.New("blib")

	err := s.fix.run(c, func() {
		s.fix.catacomb.Kill(first)
		s.fix.catacomb.Kill(nil)
	})
	c.Check(err, gc.Equals, first)
}

func (s *CatacombSuite) TestKillNonNilDoesNotOverwriteNonNil(c *gc.C) {
	first := errors.New("blib")
	second := errors.New("blob")

	err := s.fix.run(c, func() {
		s.fix.catacomb.Kill(first)
		s.fix.catacomb.Kill(second)
	})
	c.Check(err, gc.Equals, first)
}

func (s *CatacombSuite) TestAliveErrDyingDifferent(c *gc.C) {
	s.fix.run(c, func() {
		notDying := s.fix.catacomb.ErrDying()
		c.Check(notDying, gc.ErrorMatches, "bad catacomb ErrDying: still alive")

		s.fix.catacomb.Kill(nil)
		dying := s.fix.catacomb.ErrDying()
		c.Check(dying, gc.ErrorMatches, "catacomb 0x[0-9a-f]+ is dying")
	})
}

func (s *CatacombSuite) TestKillAliveErrDying(c *gc.C) {
	var notDying error

	err := s.fix.run(c, func() {
		notDying = s.fix.catacomb.ErrDying()
		s.fix.catacomb.Kill(notDying)
	})
	c.Check(err, gc.Equals, notDying)
}

func (s *CatacombSuite) TestKillErrDyingDoesNotOverwriteNil(c *gc.C) {
	err := s.fix.run(c, func() {
		s.fix.catacomb.Kill(nil)
		errDying := s.fix.catacomb.ErrDying()
		s.fix.catacomb.Kill(errDying)
	})
	c.Check(err, jc.ErrorIsNil)
}

func (s *CatacombSuite) TestKillErrDyingDoesNotOverwriteNonNil(c *gc.C) {
	first := errors.New("FRIST!")

	err := s.fix.run(c, func() {
		s.fix.catacomb.Kill(first)
		errDying := s.fix.catacomb.ErrDying()
		s.fix.catacomb.Kill(errDying)
	})
	c.Check(err, gc.Equals, first)
}

func (s *CatacombSuite) TestKillCauseErrDyingDoesNotOverwriteNil(c *gc.C) {
	err := s.fix.run(c, func() {
		s.fix.catacomb.Kill(nil)
		errDying := s.fix.catacomb.ErrDying()
		disguised := errors.Annotatef(errDying, "disguised")
		s.fix.catacomb.Kill(disguised)
	})
	c.Check(err, jc.ErrorIsNil)
}

func (s *CatacombSuite) TestKillCauseErrDyingDoesNotOverwriteNonNil(c *gc.C) {
	first := errors.New("FRIST!")

	err := s.fix.run(c, func() {
		s.fix.catacomb.Kill(first)
		errDying := s.fix.catacomb.ErrDying()
		disguised := errors.Annotatef(errDying, "disguised")
		s.fix.catacomb.Kill(disguised)
	})
	c.Check(err, gc.Equals, first)
}

func (s *CatacombSuite) TestKillTombErrDying(c *gc.C) {
	err := s.fix.run(c, func() {
		s.fix.catacomb.Kill(tomb.ErrDying)
	})
	c.Check(err, gc.ErrorMatches, "bad catacomb Kill: tomb.ErrDying")
}

func (s *CatacombSuite) TestKillErrDyingFromOtherCatacomb(c *gc.C) {
	fix2 := &fixture{}
	fix2.run(c, func() {})
	errDying := fix2.catacomb.ErrDying()

	err := s.fix.run(c, func() {
		s.fix.catacomb.Kill(errDying)
	})
	c.Check(err, gc.ErrorMatches, "bad catacomb Kill: other catacomb's ErrDying")
}

func (s *CatacombSuite) TestStopsAddedWorker(c *gc.C) {
	w := s.fix.startErrorWorker(c, nil)

	err := s.fix.run(c, func() {
		s.fix.assertAddAlive(c, w)
	})
	c.Check(err, jc.ErrorIsNil)
	w.assertDead(c)
}

func (s *CatacombSuite) TestStopsInitWorker(c *gc.C) {
	w := s.fix.startErrorWorker(c, nil)

	err := s.fix.run(c, func() {
		w.waitStillAlive(c)
	}, w)
	c.Check(err, jc.ErrorIsNil)
	w.assertDead(c)
}

func (s *CatacombSuite) TestStoppedWorkerErrorOverwritesNil(c *gc.C) {
	expect := errors.New("splot")
	w := s.fix.startErrorWorker(c, expect)

	err := s.fix.run(c, func() {
		s.fix.assertAddAlive(c, w)
	})
	c.Check(err, gc.Equals, expect)
	w.assertDead(c)
}

func (s *CatacombSuite) TestStoppedWorkerErrorDoesNotOverwriteNonNil(c *gc.C) {
	expect := errors.New("splot")
	w := s.fix.startErrorWorker(c, errors.New("not interesting"))

	err := s.fix.run(c, func() {
		s.fix.assertAddAlive(c, w)
		s.fix.catacomb.Kill(expect)
	})
	c.Check(err, gc.Equals, expect)
	w.assertDead(c)
}

func (s *CatacombSuite) TestAddWhenDyingStopsWorker(c *gc.C) {
	err := s.fix.run(c, func() {
		w := s.fix.startErrorWorker(c, nil)
		s.fix.catacomb.Kill(nil)
		expect := s.fix.catacomb.ErrDying()

		err := s.fix.catacomb.Add(w)
		c.Assert(err, gc.Equals, expect)
		w.assertDead(c)
	})
	c.Check(err, jc.ErrorIsNil)
}

func (s *CatacombSuite) TestAddWhenDyingReturnsWorkerError(c *gc.C) {
	err := s.fix.run(c, func() {
		expect := errors.New("squelch")
		w := s.fix.startErrorWorker(c, expect)
		s.fix.catacomb.Kill(nil)

		actual := s.fix.catacomb.Add(w)
		c.Assert(errors.Cause(actual), gc.Equals, expect)
		w.assertDead(c)
	})
	c.Check(err, jc.ErrorIsNil)
}

func (s *CatacombSuite) TestAddWhenDeadStopsWorker(c *gc.C) {
	s.fix.run(c, func() {})
	expect := s.fix.catacomb.ErrDying()

	w := s.fix.startErrorWorker(c, nil)
	err := s.fix.catacomb.Add(w)
	c.Assert(err, gc.Equals, expect)
	w.assertDead(c)
}

func (s *CatacombSuite) TestAddWhenDeadReturnsWorkerError(c *gc.C) {
	s.fix.run(c, func() {})

	expect := errors.New("squelch")
	w := s.fix.startErrorWorker(c, expect)
	actual := s.fix.catacomb.Add(w)
	c.Assert(errors.Cause(actual), gc.Equals, expect)
	w.assertDead(c)
}

func (s *CatacombSuite) TestFailAddedWorkerKills(c *gc.C) {
	expect := errors.New("blarft")
	w := s.fix.startErrorWorker(c, expect)

	err := s.fix.run(c, func() {
		s.fix.assertAddAlive(c, w)
		w.Kill()
		s.fix.waitDying(c)
	})
	c.Check(err, gc.Equals, expect)
	w.assertDead(c)
}

func (s *CatacombSuite) TestAddFailedWorkerKills(c *gc.C) {
	expect := errors.New("blarft")
	w := s.fix.startErrorWorker(c, expect)
	w.stop()

	err := s.fix.run(c, func() {
		err := s.fix.catacomb.Add(w)
		c.Assert(err, jc.ErrorIsNil)
		s.fix.waitDying(c)
	})
	c.Check(err, gc.Equals, expect)
}

func (s *CatacombSuite) TestInitFailedWorkerKills(c *gc.C) {
	expect := errors.New("blarft")
	w := s.fix.startErrorWorker(c, expect)
	w.stop()

	err := s.fix.run(c, func() {
		s.fix.waitDying(c)
	}, w)
	c.Check(err, gc.Equals, expect)
}

func (s *CatacombSuite) TestFinishAddedWorkerDoesNotKill(c *gc.C) {
	w := s.fix.startErrorWorker(c, nil)

	err := s.fix.run(c, func() {
		s.fix.assertAddAlive(c, w)
		w.Kill()

		w2 := s.fix.startErrorWorker(c, nil)
		s.fix.assertAddAlive(c, w2)
	})
	c.Check(err, jc.ErrorIsNil)
	w.assertDead(c)
}

func (s *CatacombSuite) TestAddFinishedWorkerDoesNotKill(c *gc.C) {
	w := s.fix.startErrorWorker(c, nil)
	w.stop()

	err := s.fix.run(c, func() {
		err := s.fix.catacomb.Add(w)
		c.Assert(err, jc.ErrorIsNil)

		w2 := s.fix.startErrorWorker(c, nil)
		s.fix.assertAddAlive(c, w2)
	})
	c.Check(err, jc.ErrorIsNil)
}

func (s *CatacombSuite) TestInitFinishedWorkerDoesNotKill(c *gc.C) {
	w := s.fix.startErrorWorker(c, nil)
	w.stop()

	err := s.fix.run(c, func() {
		w2 := s.fix.startErrorWorker(c, nil)
		s.fix.assertAddAlive(c, w2)
	}, w)
	c.Check(err, jc.ErrorIsNil)
}

func (s *CatacombSuite) TestStress(c *gc.C) {
	const workerCount = 1000
	workers := make([]*errorWorker, 0, workerCount)

	// Just add a whole bunch of workers...
	err := s.fix.run(c, func() {
		for i := 0; i < workerCount; i++ {
			w := s.fix.startErrorWorker(c, errors.Errorf("error %d", i))
			err := s.fix.catacomb.Add(w)
			c.Check(err, jc.ErrorIsNil)
			workers = append(workers, w)
		}
	})

	// ...and check that one of them killed the catacomb when it shut down;
	// and that all of them have been stopped.
	c.Check(err, gc.ErrorMatches, "error [0-9]+")
	for _, w := range workers {
		defer w.assertDead(c)
	}
}

func (s *CatacombSuite) TestStressAddKillRaces(c *gc.C) {
	const workerCount = 500

	// This construct lets us run a bunch of funcs "simultaneously"...
	var wg sync.WaitGroup
	block := make(chan struct{})
	together := func(f func()) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-block
			f()
		}()
	}

	// ...so we can queue up a whole bunch of adds/kills...
	errFailed := errors.New("pow")
	w := s.fix.startErrorWorker(c, errFailed)
	err := s.fix.run(c, func() {
		for i := 0; i < workerCount; i++ {
			together(func() {
				// NOTE: we reuse the same worker, largely for brevity's sake;
				// the important thing is that it already exists so we can hit
				// Add() as soon as possible, just like the Kill() below.
				if err := s.fix.catacomb.Add(w); err != nil {
					cause := errors.Cause(err)
					c.Check(cause, gc.Equals, errFailed)
				}
			})
			together(func() {
				s.fix.catacomb.Kill(errFailed)
			})
		}

		// ...then activate them all and see what happens.
		close(block)
		wg.Wait()
	})
	cause := errors.Cause(err)
	c.Check(cause, gc.Equals, errFailed)
}

func (s *CatacombSuite) TestReusedCatacomb(c *gc.C) {
	var site catacomb.Catacomb
	err := catacomb.Invoke(catacomb.Plan{
		Site: &site,
		Work: func() error { return nil },
	})
	c.Check(err, jc.ErrorIsNil)
	err = site.Wait()
	c.Check(err, jc.ErrorIsNil)

	w := s.fix.startErrorWorker(c, nil)
	err = catacomb.Invoke(catacomb.Plan{
		Site: &site,
		Work: func() error { return nil },
		Init: []worker.Worker{w},
	})
	c.Check(err, gc.ErrorMatches, "catacomb 0x[0-9a-f]+ has already been used")
	w.assertDead(c)
}

func (s *CatacombSuite) TestPlanBadSite(c *gc.C) {
	w := s.fix.startErrorWorker(c, nil)
	plan := catacomb.Plan{
		Work: func() error { panic("no") },
		Init: []worker.Worker{w},
	}
	checkInvalid(c, plan, "nil Site not valid")
	w.assertDead(c)
}

func (s *CatacombSuite) TestPlanBadWork(c *gc.C) {
	w := s.fix.startErrorWorker(c, nil)
	plan := catacomb.Plan{
		Site: &catacomb.Catacomb{},
		Init: []worker.Worker{w},
	}
	checkInvalid(c, plan, "nil Work not valid")
	w.assertDead(c)
}

func (s *CatacombSuite) TestPlanBadInit(c *gc.C) {
	w := s.fix.startErrorWorker(c, nil)
	plan := catacomb.Plan{
		Site: &catacomb.Catacomb{},
		Work: func() error { panic("no") },
		Init: []worker.Worker{w, nil},
	}
	checkInvalid(c, plan, "nil Init item 1 not valid")
	w.assertDead(c)
}

func (s *CatacombSuite) TestPlanDataRace(c *gc.C) {
	w := s.fix.startErrorWorker(c, nil)
	plan := catacomb.Plan{
		Site: &catacomb.Catacomb{},
		Work: func() error { return nil },
		Init: []worker.Worker{w},
	}
	err := catacomb.Invoke(plan)
	c.Assert(err, jc.ErrorIsNil)

	plan.Init[0] = nil
}

func checkInvalid(c *gc.C, plan catacomb.Plan, match string) {
	check := func(err error) {
		c.Check(err, gc.ErrorMatches, match)
		c.Check(err, jc.Satisfies, errors.IsNotValid)
	}
	check(plan.Validate())
	check(catacomb.Invoke(plan))
}

// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package catacomb_test

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"launchpad.net/tomb"

	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/catacomb"
)

type CatacombSuite struct {
	testing.IsolationSuite
	catacomb *catacomb.Catacomb
}

var _ = gc.Suite(&CatacombSuite{})

func (s *CatacombSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.catacomb = catacomb.New()
}

func (s *CatacombSuite) TearDownTest(c *gc.C) {
	if s.catacomb != nil {
		select {
		case <-s.catacomb.Dead():
		default:
			s.catacomb.Done()
		}
	}
	s.IsolationSuite.TearDownTest(c)
}

func (s *CatacombSuite) TestStartsAlive(c *gc.C) {
	s.assertNotDying(c)
	s.assertNotDead(c)
}

func (s *CatacombSuite) TestKillClosesDying(c *gc.C) {
	s.catacomb.Kill(nil)
	s.assertDying(c)
}

func (s *CatacombSuite) TestKillDoesNotCloseDead(c *gc.C) {
	s.catacomb.Kill(nil)
	s.assertNotDead(c)
}

func (s *CatacombSuite) TestDoneStopsCompletely(c *gc.C) {
	s.assertDoneError(c, nil)
	s.assertDying(c)
}

func (s *CatacombSuite) TestKillNonNilOverwritesNil(c *gc.C) {
	s.catacomb.Kill(nil)
	second := errors.New("blah")
	s.catacomb.Kill(second)
	s.assertDoneError(c, second)
}

func (s *CatacombSuite) TestKillNilDoesNotOverwriteNonNil(c *gc.C) {
	first := errors.New("blib")
	s.catacomb.Kill(first)
	s.catacomb.Kill(nil)
	s.assertDoneError(c, first)
}

func (s *CatacombSuite) TestKillNonNilDoesNotOverwriteNonNil(c *gc.C) {
	first := errors.New("blib")
	s.catacomb.Kill(first)
	second := errors.New("blob")
	s.catacomb.Kill(second)
	s.assertDoneError(c, first)
}

func (s *CatacombSuite) TestAliveErrDyingDifferent(c *gc.C) {
	notDying := s.catacomb.ErrDying()
	c.Check(notDying, gc.ErrorMatches, "bad catacomb ErrDying: still alive")

	s.catacomb.Kill(nil)
	dying := s.catacomb.ErrDying()
	c.Check(dying, gc.ErrorMatches, "catacomb 0x[0-9a-f]+ is dying")
}

func (s *CatacombSuite) TestKillAliveErrDying(c *gc.C) {
	notDying := s.catacomb.ErrDying()
	s.catacomb.Kill(notDying)
	s.assertDoneError(c, notDying)
}

func (s *CatacombSuite) TestKillErrDyingDoesNotOverwriteNil(c *gc.C) {
	s.catacomb.Kill(nil)
	s.catacomb.Kill(s.catacomb.ErrDying())
	s.assertDoneError(c, nil)
}

func (s *CatacombSuite) TestKillErrDyingDoesNotOverwriteNonNil(c *gc.C) {
	first := errors.New("FRIST!")
	s.catacomb.Kill(first)
	s.catacomb.Kill(s.catacomb.ErrDying())
	s.assertDoneError(c, first)
}

func (s *CatacombSuite) TestKillCauseErrDyingDoesNotOverwriteNil(c *gc.C) {
	s.catacomb.Kill(nil)
	disguised := errors.Annotatef(s.catacomb.ErrDying(), "disguised")
	s.catacomb.Kill(disguised)
	s.assertDoneError(c, nil)
}

func (s *CatacombSuite) TestKillCauseErrDyingDoesNotOverwriteNonNil(c *gc.C) {
	first := errors.New("FRIST!")
	s.catacomb.Kill(first)
	disguised := errors.Annotatef(s.catacomb.ErrDying(), "disguised")
	s.catacomb.Kill(disguised)
	s.assertDoneError(c, first)
}

func (s *CatacombSuite) TestKillTombErrDying(c *gc.C) {
	s.catacomb.Kill(tomb.ErrDying)
	s.catacomb.Done()
	err := s.catacomb.Wait()
	c.Check(err, gc.ErrorMatches, "bad catacomb Kill: tomb.ErrDying")
}

func (s *CatacombSuite) TestKillErrDyingFromOtherCatacomb(c *gc.C) {
	other := catacomb.New()
	other.Kill(nil)
	defer func() {
		other.Done()
		other.Wait()
	}()

	s.catacomb.Kill(other.ErrDying())
	s.catacomb.Done()
	err := s.catacomb.Wait()
	c.Check(err, gc.ErrorMatches, "bad catacomb Kill: other catacomb's ErrDying")
}

func (s *CatacombSuite) TestKillStopsAddedWorker(c *gc.C) {
	w := s.startErrorWorker(c, nil)
	s.assertAddAlive(c, w)

	s.catacomb.Kill(nil)
	s.assertDoneError(c, nil)
	w.assertDead(c)
}

func (s *CatacombSuite) TestStoppedWorkerErrorOverwritesNil(c *gc.C) {
	expect := errors.New("splot")
	w := s.startErrorWorker(c, expect)
	s.assertAddAlive(c, w)

	s.catacomb.Kill(nil)
	s.assertDoneError(c, expect)
	w.assertDead(c)
}

func (s *CatacombSuite) TestStoppedWorkerErrorDoesNotOverwriteNonNil(c *gc.C) {
	w := s.startErrorWorker(c, errors.New("not interesting"))
	s.assertAddAlive(c, w)

	expect := errors.New("splot")
	s.catacomb.Kill(expect)
	s.assertDoneError(c, expect)
	w.assertDead(c)
}

func (s *CatacombSuite) TestAddWhenDyingStopsWorker(c *gc.C) {
	w := s.startErrorWorker(c, nil)

	s.catacomb.Kill(nil)
	err := s.catacomb.Add(w)
	c.Assert(err, gc.Equals, s.catacomb.ErrDying())
	w.assertDead(c)
}

func (s *CatacombSuite) TestAddWhenDyingReturnsWorkerError(c *gc.C) {
	expect := errors.New("squelch")
	w := s.startErrorWorker(c, expect)

	s.catacomb.Kill(nil)
	actual := s.catacomb.Add(w)
	c.Assert(errors.Cause(actual), gc.Equals, expect)
	w.assertDead(c)
}

func (s *CatacombSuite) TestFailedAddedWorkerKills(c *gc.C) {
	expect := errors.New("blarft")
	w := s.startErrorWorker(c, expect)
	s.assertAddAlive(c, w)

	w.Kill()
	s.waitDying(c)
	s.assertDoneError(c, expect)
	w.assertDead(c)
}

func (s *CatacombSuite) TestAddFailedWorkerKills(c *gc.C) {
	expect := errors.New("blarft")
	w := s.startErrorWorker(c, expect)
	w.Kill()
	err := s.catacomb.Add(w)
	c.Assert(err, jc.ErrorIsNil)

	s.waitDying(c)
	s.assertDoneError(c, expect)
	w.assertDead(c)
}

func (s *CatacombSuite) TestFinishAddedWorkerDoesNotKill(c *gc.C) {
	w := s.startErrorWorker(c, nil)
	s.assertAddAlive(c, w)
	w.Kill()
	s.assertAddAlive(c, s.startErrorWorker(c, nil))
	w.assertDead(c)
	s.assertDoneError(c, nil)
}

func (s *CatacombSuite) TestAddFinishedWorkerDoesNotKill(c *gc.C) {
	w := s.startErrorWorker(c, nil)
	w.Kill()
	err := s.catacomb.Add(w)
	c.Assert(err, jc.ErrorIsNil)
	s.assertAddAlive(c, s.startErrorWorker(c, nil))
	s.assertDoneError(c, nil)
}

func (s *CatacombSuite) TestStress(c *gc.C) {
	const workerCount = 100
	for i := 0; i < workerCount; i++ {
		w := s.startErrorWorker(c, errors.Errorf("error %d", i))
		defer w.assertDead(c)
		err := s.catacomb.Add(w)
		c.Check(err, jc.ErrorIsNil)
	}
	s.catacomb.Done()
	err := s.catacomb.Wait()
	c.Check(err, gc.ErrorMatches, "error [0-9]+")
}

func (s *CatacombSuite) waitDying(c *gc.C) {
	select {
	case <-s.catacomb.Dying():
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out; still alive")
	}
}

func (s *CatacombSuite) assertDying(c *gc.C) {
	select {
	case <-s.catacomb.Dying():
	default:
		c.Fatalf("still alive")
	}
}

func (s *CatacombSuite) assertNotDying(c *gc.C) {
	select {
	case <-s.catacomb.Dying():
		c.Fatalf("already dying")
	default:
	}
}

func (s *CatacombSuite) assertDead(c *gc.C) {
	select {
	case <-s.catacomb.Dead():
	default:
		c.Fatalf("not dead")
	}
}

func (s *CatacombSuite) assertNotDead(c *gc.C) {
	select {
	case <-s.catacomb.Dead():
		c.Fatalf("already dead")
	default:
	}
}

func (s *CatacombSuite) assertDoneError(c *gc.C, expect error) {
	s.catacomb.Done()
	s.assertDead(c)
	actual := s.catacomb.Wait()
	if expect == nil {
		c.Assert(actual, jc.ErrorIsNil)
	} else {
		c.Assert(actual, gc.Equals, expect)
	}
}

func (s *CatacombSuite) assertAddAlive(c *gc.C, w *errorWorker) {
	err := s.catacomb.Add(w)
	c.Assert(err, jc.ErrorIsNil)
	w.waitStillAlive(c)
}

func (s *CatacombSuite) startErrorWorker(c *gc.C, err error) *errorWorker {
	ew := &errorWorker{}
	go func() {
		defer ew.tomb.Done()
		defer ew.tomb.Kill(err)
		<-ew.tomb.Dying()
	}()
	s.AddCleanup(func(c *gc.C) {
		ew.Kill()
		ew.Wait()
	})
	return ew
}

type errorWorker struct {
	tomb tomb.Tomb
}

func (ew *errorWorker) Kill() {
	ew.tomb.Kill(nil)
}

func (ew *errorWorker) Wait() error {
	return ew.tomb.Wait()
}

func (ew *errorWorker) waitStillAlive(c *gc.C) {
	select {
	case <-ew.tomb.Dying():
		c.Fatalf("already dying")
	case <-time.After(coretesting.ShortWait):
	}
}

func (ew *errorWorker) assertDead(c *gc.C) {
	select {
	case <-ew.tomb.Dead():
	default:
		c.Fatalf("not yet dead")
	}
}

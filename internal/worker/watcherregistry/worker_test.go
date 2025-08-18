// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package watcherregistry

import (
	"fmt"
	"strings"
	"sync"
	stdtesting "testing"

	"github.com/juju/clock"
	"github.com/juju/tc"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/goleak"

	coreerrors "github.com/juju/juju/core/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
)

type registrySuite struct {
	testhelpers.IsolationSuite
}

func TestRegistrySuite(t *stdtesting.T) {
	defer goleak.VerifyNone(t)
	tc.Run(t, &registrySuite{})
}

func (s *registrySuite) TestRegisterCount(c *tc.C) {
	reg, regGetter := s.newRegistry(c)
	defer workertest.CleanKill(c, regGetter)

	c.Check(reg.Count(), tc.Equals, 0)
}

func (s *registrySuite) TestRegisterGetCount(c *tc.C) {
	reg, regGetter := s.newRegistry(c)
	defer workertest.CleanKill(c, regGetter)

	for i := range 10 {
		w := workertest.NewErrorWorker(nil)

		id, err := reg.Register(c.Context(), w)
		c.Assert(err, tc.ErrorIsNil)

		w1, err := reg.Get(id)
		c.Assert(err, tc.ErrorIsNil)
		c.Check(w1, tc.Equals, w)
		c.Check(reg.Count(), tc.Equals, i+1)
	}

	err := reg.StopAll()
	c.Assert(err, tc.ErrorIsNil)

	c.Check(reg.Count(), tc.Equals, 0)
}

func (s *registrySuite) TestRegisterNamedGetCount(c *tc.C) {
	reg, regGetter := s.newRegistry(c)
	defer workertest.CleanKill(c, regGetter)

	for i := range 10 {
		w := workertest.NewErrorWorker(nil)

		id := fmt.Sprintf("id-%d", i)
		err := reg.RegisterNamed(c.Context(), id, w)
		c.Assert(err, tc.ErrorIsNil)

		w1, err := reg.Get(id)
		c.Assert(err, tc.ErrorIsNil)
		c.Check(w1, tc.Equals, w)
		c.Check(reg.Count(), tc.Equals, i+1)
	}

	err := reg.StopAll()
	c.Assert(err, tc.ErrorIsNil)

	c.Check(reg.Count(), tc.Equals, 0)
}

func (s *registrySuite) TestRegisterNamedRepeatedError(c *tc.C) {
	reg, regGetter := s.newRegistry(c)
	defer workertest.CleanKill(c, regGetter)

	w := workertest.NewErrorWorker(nil)

	err := reg.RegisterNamed(c.Context(), "foo", w)
	c.Assert(err, tc.ErrorIsNil)

	err = reg.RegisterNamed(c.Context(), "foo", w)
	c.Assert(err, tc.ErrorMatches, `worker "foo" already exists`)
	c.Assert(err, tc.ErrorIs, coreerrors.AlreadyExists)
}

func (s *registrySuite) TestRegisterNamedIntegerName(c *tc.C) {
	reg, regGetter := s.newRegistry(c)
	defer workertest.CleanKill(c, regGetter)

	w := workertest.NewErrorWorker(nil)
	defer workertest.CleanKill(c, w)

	err := reg.RegisterNamed(c.Context(), "0", w)
	c.Assert(err, tc.ErrorMatches, `namespace "0" not valid`)
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

func (s *registrySuite) TestRegisterStop(c *tc.C) {
	reg, regGetter := s.newRegistry(c)
	defer workertest.CleanKill(c, regGetter)

	err := reg.RegisterNamed(c.Context(), "foo", workertest.NewErrorWorker(nil))
	c.Assert(err, tc.ErrorIsNil)

	err = reg.Stop("foo")
	c.Assert(err, tc.ErrorIsNil)

	c.Check(reg.Count(), tc.Equals, 0)
}

func (s *registrySuite) TestConcurrency(c *tc.C) {
	// This test is designed to cause the race detector
	// to fail if the locking is not done correctly.
	reg, regGetter := s.newRegistry(c)
	defer workertest.CleanKill(c, regGetter)

	var wg sync.WaitGroup
	start := func(f func()) {
		wg.Add(1)
		go func() {
			f()
			wg.Done()
		}()
	}
	_, err := reg.Register(c.Context(), workertest.NewErrorWorker(nil))
	c.Assert(err, tc.ErrorIsNil)

	start(func() {
		_, _ = reg.Register(c.Context(), workertest.NewErrorWorker(nil))
	})
	start(func() {
		_ = reg.RegisterNamed(c.Context(), "named", workertest.NewErrorWorker(nil))
	})
	start(func() {
		_ = reg.Stop("1")
	})
	start(func() {
		_ = reg.Count()
	})
	start(func() {
		_, _ = reg.Get("2")
	})
	start(func() {
		_, _ = reg.Get("named")
	})
	wg.Wait()
}

func (s *registrySuite) TestMultipleRegistries(c *tc.C) {
	w := s.newWorker(c)
	defer workertest.CleanKill(c, w)

	reg1, err := w.GetWatcherRegistry(c.Context(), 0)
	c.Assert(err, tc.ErrorIsNil)

	reg2, err := w.GetWatcherRegistry(c.Context(), 1)
	c.Assert(err, tc.ErrorIsNil)

	type result struct {
		id  string
		err error
	}

	res1 := make(chan result, 1)
	go func() {
		id, err := reg1.Register(c.Context(), workertest.NewErrorWorker(nil))
		res1 <- result{
			id:  id,
			err: err,
		}
	}()

	res2 := make(chan result, 1)
	go func() {
		id, err := reg2.Register(c.Context(), workertest.NewErrorWorker(nil))
		res2 <- result{
			id:  id,
			err: err,
		}
	}()

	var id1 string
	select {
	case res := <-res1:
		c.Assert(res.err, tc.ErrorIsNil)
		id1 = res.id
	case <-c.Context().Done():
		c.Fatalf("timed out waiting for registry 1 to register")
	}

	var id2 string
	select {
	case res := <-res2:
		c.Assert(res.err, tc.ErrorIsNil)
		id2 = res.id
	case <-c.Context().Done():
		c.Fatalf("timed out waiting for registry 2 to register")
	}

	c.Check(reg1.Count(), tc.Equals, 1)
	c.Check(reg2.Count(), tc.Equals, 1)

	// Each registry should be namespaced separately.

	c.Assert(strings.HasPrefix(id1, w.namespacePrefix), tc.IsTrue)
	c.Assert(strings.HasPrefix(id2, w.namespacePrefix), tc.IsTrue)

	// As we're only creating one worker in each registry, they should have
	// the same id.
	c.Check(id1 == id2, tc.IsTrue)

	w1, err := reg1.Get(id1)
	c.Assert(err, tc.ErrorIsNil)

	w2, err := reg2.Get(id2)
	c.Assert(err, tc.ErrorIsNil)

	// They should not be the same worker.
	c.Check(w1 == w2, tc.IsFalse)
}

func (s *registrySuite) TestMultipleNamedRegistries(c *tc.C) {
	w := s.newWorker(c)
	defer workertest.CleanKill(c, w)

	reg1, err := w.GetWatcherRegistry(c.Context(), 0)
	c.Assert(err, tc.ErrorIsNil)

	reg2, err := w.GetWatcherRegistry(c.Context(), 1)
	c.Assert(err, tc.ErrorIsNil)

	err1 := make(chan error, 1)
	go func() {
		err := reg1.RegisterNamed(c.Context(), "foo", workertest.NewErrorWorker(nil))
		err1 <- err
	}()

	err2 := make(chan error, 1)
	go func() {
		err := reg2.RegisterNamed(c.Context(), "foo", workertest.NewErrorWorker(nil))
		err2 <- err
	}()

	select {
	case err := <-err1:
		c.Assert(err, tc.ErrorIsNil)
	case <-c.Context().Done():
		c.Fatalf("timed out waiting for registry 1 to register")
	}

	select {
	case err := <-err2:
		c.Assert(err, tc.ErrorIsNil)
	case <-c.Context().Done():
		c.Fatalf("timed out waiting for registry 2 to register")
	}

	c.Check(reg1.Count(), tc.Equals, 1)
	c.Check(reg2.Count(), tc.Equals, 1)

	// Each registry should be namespaced separately.

	w1, err := reg1.Get("foo")
	c.Assert(err, tc.ErrorIsNil)

	w2, err := reg2.Get("foo")
	c.Assert(err, tc.ErrorIsNil)

	// They should not be the same worker.
	c.Check(w1 == w2, tc.IsFalse)
}

func (s *registrySuite) TestMultipleRegistriesStopOneShouldNotStopOthers(c *tc.C) {
	w := s.newWorker(c)
	defer workertest.CleanKill(c, w)

	reg1, err := w.GetWatcherRegistry(c.Context(), 0)
	c.Assert(err, tc.ErrorIsNil)

	reg2, err := w.GetWatcherRegistry(c.Context(), 1)
	c.Assert(err, tc.ErrorIsNil)

	_, err = reg1.Register(c.Context(), workertest.NewErrorWorker(nil))
	c.Assert(err, tc.ErrorIsNil)

	_, err = reg2.Register(c.Context(), workertest.NewErrorWorker(nil))
	c.Assert(err, tc.ErrorIsNil)

	c.Check(reg1.Count(), tc.Equals, 1)
	c.Check(reg2.Count(), tc.Equals, 1)

	err = reg1.StopAll()
	c.Assert(err, tc.ErrorIsNil)

	// Stopping one registry should not stop the other.
	c.Check(reg1.Count(), tc.Equals, 0)
	c.Check(reg2.Count(), tc.Equals, 1)

	w3 := workertest.NewErrorWorker(nil)
	defer workertest.CleanKill(c, w3)

	_, err = reg1.Register(c.Context(), w3)
	c.Assert(err, tc.ErrorIs, ErrWatcherRegistryClosed)

	w4 := workertest.NewErrorWorker(nil)
	defer workertest.CleanKill(c, w4)

	_, err = reg2.Register(c.Context(), w4)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(reg1.Count(), tc.Equals, 0)
	c.Check(reg2.Count(), tc.Equals, 2)
}

func (s *registrySuite) TestMultipleRegistriesReturnsTheSameRegistry(c *tc.C) {
	w := s.newWorker(c)
	defer workertest.CleanKill(c, w)

	reg1, err := w.GetWatcherRegistry(c.Context(), 0)
	c.Assert(err, tc.ErrorIsNil)

	reg2, err := w.GetWatcherRegistry(c.Context(), 0)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(reg1 == reg2, tc.IsTrue)
}

func (s *registrySuite) newRegistry(c *tc.C) (WatcherRegistry, worker.Worker) {
	w := s.newWorker(c)

	reg, err := w.GetWatcherRegistry(c.Context(), 0)
	c.Assert(err, tc.ErrorIsNil)

	return reg, w
}

func (s *registrySuite) newWorker(c *tc.C) *Worker {
	w, err := NewWorker(Config{
		Clock:  clock.WallClock,
		Logger: loggertesting.WrapCheckLog(c),
	})
	c.Assert(err, tc.ErrorIsNil)
	return w.(*Worker)
}

// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package providertracker

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/catacomb"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/database"
	"github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/testing"
)

type providerWorkerSuite struct {
	baseSuite

	called int64
}

var _ = gc.Suite(&providerWorkerSuite{})

func (s *providerWorkerSuite) TestKilledSingularWorkerProviderErrDying(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Ensure that a killed worker returns the correct error when
	// Provider is called.

	s.expectServiceFactory("hunter2")

	w := s.newSingularWorker(c)
	defer workertest.DirtyKill(c, w)

	s.ensureStartup(c)

	// Ensure that the worker does die correctly. If not the test will just
	// continue forever.
	workertest.DirtyKill(c, w)

	worker := w.(*providerWorker)
	_, err := worker.Provider()
	c.Assert(err, jc.ErrorIs, ErrProviderWorkerDying)
}

func (s *providerWorkerSuite) TestKilledMultiWorkerProviderErrDying(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Ensure that a killed worker returns the correct error when
	// Provider is called.

	w := s.newMultiWorker(c)
	defer workertest.DirtyKill(c, w)

	s.ensureStartup(c)

	// Ensure that the worker does die correctly. If not the test will just
	// continue forever.
	workertest.DirtyKill(c, w)

	worker := w.(*providerWorker)
	_, err := worker.ProviderForModel(context.Background(), "hunter2")
	c.Assert(err, jc.ErrorIs, ErrProviderWorkerDying)
}

func (s *providerWorkerSuite) TestMultiFailsForSingularModels(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// If we're running in multi mode, ensure that we get an error if
	// we're in a singular-model environment.

	w := s.newMultiWorker(c)
	defer workertest.DirtyKill(c, w)

	s.ensureStartup(c)

	worker := w.(*providerWorker)
	_, err := worker.Provider()
	c.Assert(err, jc.ErrorIs, errors.NotValid)
}

func (s *providerWorkerSuite) TestSingularFailsForMultiModels(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// If we're running in singular mode, ensure that we get an error if
	// we're in a multi-model environment.

	s.expectServiceFactory("hunter2")

	w := s.newSingularWorker(c)
	defer workertest.DirtyKill(c, w)

	s.ensureStartup(c)

	worker := w.(*providerWorker)
	_, err := worker.ProviderForModel(context.Background(), "hunter2")
	c.Assert(err, jc.ErrorIs, errors.NotValid)
}

func (s *providerWorkerSuite) TestControllerNamespaceFails(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Prevent requests to the controller namespace.

	s.expectServiceFactory("hunter2")

	w := s.newSingularWorker(c)
	defer workertest.DirtyKill(c, w)

	s.ensureStartup(c)

	worker := w.(*providerWorker)
	_, err := worker.ProviderForModel(context.Background(), database.ControllerNS)
	c.Assert(err, jc.ErrorIs, errors.NotValid)
}

func (s *providerWorkerSuite) TestProvider(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Ensure that the provider is returned correctly.

	s.expectServiceFactory("hunter2")

	w := s.newSingularWorker(c)
	defer workertest.CleanKill(c, w)

	s.ensureStartup(c)

	worker := w.(*providerWorker)
	provider, err := worker.Provider()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(provider, gc.NotNil)
}

func (s *providerWorkerSuite) TestProviderIsCached(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Ensure that calling the provider multiple times returns the same
	// provider.

	s.expectServiceFactory("hunter2")

	w := s.newSingularWorker(c)
	defer workertest.CleanKill(c, w)

	s.ensureStartup(c)

	worker := w.(*providerWorker)
	for i := 0; i < 10; i++ {
		_, err := worker.Provider()
		c.Assert(err, jc.ErrorIsNil)
	}

	workertest.CleanKill(c, w)

	c.Assert(atomic.LoadInt64(&s.called), gc.Equals, int64(1))
}

func (s *providerWorkerSuite) TestProviderForModel(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Ensure that the provider for a model is returned correctly.

	s.expectServiceFactory("hunter2")

	w := s.newMultiWorker(c)
	defer workertest.CleanKill(c, w)

	s.ensureStartup(c)

	worker := w.(*providerWorker)

	provider, err := worker.ProviderForModel(context.Background(), "hunter2")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(provider, gc.NotNil)
}

func (s *providerWorkerSuite) TestProviderForModelIsCached(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Ensure that calling the provider multiple times returns the same
	// provider.

	s.expectServiceFactory("hunter2")

	w := s.newSingularWorker(c)
	defer workertest.CleanKill(c, w)

	s.ensureStartup(c)

	worker := w.(*providerWorker)
	for i := 0; i < 10; i++ {
		_, err := worker.Provider()
		c.Assert(err, jc.ErrorIsNil)
	}

	workertest.CleanKill(c, w)

	c.Assert(atomic.LoadInt64(&s.called), gc.Equals, int64(1))
}

func (s *providerWorkerSuite) TestProviderForModelIsNotCachedForDifferentNamespaces(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Ensure that calling the provider multiple times returns the same
	// provider.

	for i := 0; i < 10; i++ {
		name := fmt.Sprintf("hunter-%d", i)
		s.expectServiceFactory(name)
	}

	w := s.newMultiWorker(c)
	defer workertest.CleanKill(c, w)

	s.ensureStartup(c)

	worker := w.(*providerWorker)
	for i := 0; i < 10; i++ {
		name := fmt.Sprintf("hunter-%d", i)

		_, err := worker.ProviderForModel(context.Background(), name)
		c.Assert(err, jc.ErrorIsNil)
	}

	workertest.CleanKill(c, w)

	c.Assert(atomic.LoadInt64(&s.called), gc.Equals, int64(10))
}

func (s *providerWorkerSuite) TestProviderForModelConcurrently(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Ensure that calling the provider multiple times returns the same
	// provider.

	for i := 0; i < 10; i++ {
		name := fmt.Sprintf("hunter-%d", i)
		s.expectServiceFactory(name)
	}

	w := s.newMultiWorker(c)
	defer workertest.CleanKill(c, w)

	s.ensureStartup(c)

	var wg sync.WaitGroup
	wg.Add(10)

	worker := w.(*providerWorker)
	for i := 0; i < 10; i++ {
		go func(i int) {
			defer wg.Done()
			name := fmt.Sprintf("hunter-%d", i)

			_, err := worker.ProviderForModel(context.Background(), name)
			c.Assert(err, jc.ErrorIsNil)
		}(i)
	}

	assertWait(c, wg.Wait)
	c.Assert(atomic.LoadInt64(&s.called), gc.Equals, int64(10))
}

func (s *providerWorkerSuite) setupMocks(c *gc.C) *gomock.Controller {
	atomic.StoreInt64(&s.called, 0)

	return s.baseSuite.setupMocks(c)
}

func (s *providerWorkerSuite) newSingularWorker(c *gc.C) worker.Worker {
	return s.newWorker(c, SingularType("hunter2"))
}

func (s *providerWorkerSuite) newMultiWorker(c *gc.C) worker.Worker {
	return s.newWorker(c, MultiType())
}

func (s *providerWorkerSuite) newWorker(c *gc.C, trackerType TrackerType) worker.Worker {
	w, err := newWorker(Config{
		TrackerType:          trackerType,
		ServiceFactoryGetter: s.serviceFactoryGetter,
		GetIAASProvider: func(ctx context.Context, pcg ProviderConfigGetter) (Provider, cloudspec.CloudSpec, error) {
			return s.environ, cloudspec.CloudSpec{}, nil
		},
		GetCAASProvider: func(ctx context.Context, pcg ProviderConfigGetter) (Provider, cloudspec.CloudSpec, error) {
			c.Fatalf("unexpected call to GetCAASProvider")
			return nil, cloudspec.CloudSpec{}, nil
		},
		NewTrackerWorker: func(ctx context.Context, cfg TrackerConfig) (worker.Worker, error) {
			atomic.AddInt64(&s.called, 1)

			w := &trackerWorker{
				provider: s.environ,
			}
			err := catacomb.Invoke(catacomb.Plan{
				Site: &w.catacomb,
				Work: func() error {
					<-w.catacomb.Dying()
					return w.catacomb.ErrDying()
				},
			})
			return w, err
		},
		Logger: s.logger,
		Clock:  clock.WallClock,
	}, s.states)
	c.Assert(err, jc.ErrorIsNil)

	return w
}

func assertWait(c *gc.C, wait func()) {
	done := make(chan struct{})

	go func() {
		defer close(done)
		wait()
	}()

	select {
	case <-done:
	case <-time.After(testing.LongWait):
		c.Fatalf("timed out waiting")
	}
}

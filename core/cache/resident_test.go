// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cache

import (
	"sync"

	"github.com/golang/mock/gomock"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/worker.v1"

	"github.com/juju/juju/testing"
	"github.com/juju/juju/worker/mocks"
)

type residentSuite struct {
	testing.BaseSuite

	manager *residentManager
}

var _ = gc.Suite(&residentSuite{})

func (s *residentSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.manager = newResidentManager()
}

func (s *residentSuite) TestManagerNewIdentifiedResources(c *gc.C) {
	r1 := s.manager.new()
	r2 := s.manager.new()

	// Check that the count is what we expect.
	c.Check(s.manager.residentCount.last(), gc.Equals, uint64(2))

	// Check that the residents have IDs,
	// and that they are registered with the manager.
	c.Check(r1.id, gc.Equals, uint64(1))
	c.Check(r2.id, gc.Equals, uint64(2))
	c.Check(s.manager.residents, gc.DeepEquals, map[uint64]*resident{1: r1, 2: r2})
}

func (s *residentSuite) TestManagerDeregister(c *gc.C) {
	r1 := s.manager.new()
	c.Assert(r1.evict(), jc.ErrorIsNil)
	c.Check(s.manager.residents, gc.HasLen, 0)
}

func (s *residentSuite) TestResidentWorkerConcurrentRegisterCleanup(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	w1 := mocks.NewMockWorker(ctrl)
	w1.EXPECT().Kill()
	w1.EXPECT().Wait().Return(nil)

	w2 := mocks.NewMockWorker(ctrl)
	w2.EXPECT().Kill()
	w2.EXPECT().Wait().Return(nil)

	r := s.manager.new()

	var id1, id2 uint64

	// Register some workers concurrently.
	wg := sync.WaitGroup{}
	wg.Add(2)
	go func() {
		id1 = r.registerWorker(w1)
		wg.Done()
	}()
	go func() {
		id2 = r.registerWorker(w2)
		wg.Done()
	}()
	wg.Wait()

	// Check that the count is what we expect.
	c.Check(s.manager.resourceCount.last(), gc.Equals, uint64(2))

	// Check that the workers have IDs,
	// and that they are registered with the resident.
	switch id1 {
	case 1:
		c.Check(id2, gc.Equals, uint64(2))
		c.Check(r.workers, gc.DeepEquals, map[uint64]worker.Worker{1: w1, 2: w2})
	case 2:
		c.Check(id2, gc.Equals, uint64(1))
		c.Check(r.workers, gc.DeepEquals, map[uint64]worker.Worker{1: w2, 2: w1})
	default:
		c.Errorf("expected id1 to be either 1 or 2, got %d", id1)
	}

	// Call cleanup, which should stop and deregister the workers.
	c.Assert(r.cleanup(), jc.ErrorIsNil)
	c.Check(r.workers, gc.HasLen, 0)
}

func (s *residentSuite) TestResidentWorkerConcurrentDeregister(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	r := s.manager.new()

	// Note that we do not expect deregister to stop the worker.
	id1 := r.registerWorker(mocks.NewMockWorker(ctrl))
	id2 := r.registerWorker(mocks.NewMockWorker(ctrl))

	// Unregister concurrently.
	wg := sync.WaitGroup{}
	wg.Add(2)
	go func() {
		r.deregisterWorker(id1)
		wg.Done()
	}()
	go func() {
		r.deregisterWorker(id2)
		wg.Done()
	}()
	wg.Wait()

	c.Check(r.workers, gc.HasLen, 0)
}

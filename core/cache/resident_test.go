// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cache

import (
	"reflect"
	"sync"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v2"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/testing"
	"github.com/juju/juju/worker/mocks"
)

type residentSuite struct {
	BaseSuite
}

var _ = gc.Suite(&residentSuite{})

func (s *residentSuite) TestManagerNewIdentifiedResources(c *gc.C) {
	r1 := s.Manager.new()
	r2 := s.Manager.new()

	// Check that the count is what we expect.
	c.Check(s.Manager.residentCount.last(), gc.Equals, uint64(2))

	// Check that the residents have IDs,
	// and that they are registered with the manager.
	c.Check(r1.id, gc.Equals, uint64(1))
	c.Check(r2.id, gc.Equals, uint64(2))
	c.Check(s.Manager.residents, gc.DeepEquals, map[uint64]*Resident{1: r1, 2: r2})
}

func (s *residentSuite) TestManagerDeregister(c *gc.C) {
	r1 := s.Manager.new()
	c.Assert(r1.evict(), jc.ErrorIsNil)
	c.Check(s.Manager.residents, gc.HasLen, 0)
}

func (s *residentSuite) TestManagerMarkAndSweepSendsRemovalMessagesForStaleResidents(c *gc.C) {
	r1 := s.Manager.new()
	r2 := s.Manager.new()
	r3 := s.Manager.new()
	r4 := s.Manager.new()

	r1.removalMessage = 1
	r2.removalMessage = 2
	r3.removalMessage = 3
	r4.removalMessage = 4

	// Sets all 4 to be stale, but we freshen up one.
	c.Assert(s.Manager.isMarked(), jc.IsFalse)
	s.Manager.mark()
	c.Assert(s.Manager.isMarked(), jc.IsTrue)
	r1.setStale(false)

	// Consume all the messages from the manager's removals channel.
	var removals []interface{}
	done := make(chan struct{})
	go func() {
		timeout := time.After(testing.LongWait)
		for {
			select {
			case msg, ok := <-s.Changes:
				if !ok {
					close(done)
					return
				}
				removals = append(removals, msg)
			case <-timeout:
				c.Fatal("did not finish receiving removal messages")
			}
		}
	}()

	select {
	case <-s.Manager.sweep():
	case <-time.After(testing.LongWait):
		c.Fatal("timeout waiting for sweep to complete")
	}
	close(s.Changes)
	select {
	case <-done:
	case <-time.After(testing.LongWait):
		c.Fatal("timeout waiting for sweep removal messages")
	}

	// Stale resident messages were received in descending order.
	c.Assert(removals, gc.DeepEquals, []interface{}{4, 3, 2})
	c.Assert(s.Manager.isMarked(), jc.IsFalse)
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

	r := s.Manager.new()

	// Register some workers concurrently.
	wg := sync.WaitGroup{}
	wg.Add(2)
	go func() {
		_ = r.registerWorker(w1)
		wg.Done()
	}()
	go func() {
		_ = r.registerWorker(w2)
		wg.Done()
	}()
	wg.Wait()

	// Check that the count is what we expect.
	c.Check(s.Manager.resourceCount.last(), gc.Equals, uint64(2))

	// Check that the workers have IDs,
	// and that they are registered with the resident.
	switch {
	case reflect.DeepEqual(r.workers, map[uint64]worker.Worker{1: w1, 2: w2}):
	case reflect.DeepEqual(r.workers, map[uint64]worker.Worker{2: w1, 1: w2}):
	default:
		c.Errorf("expected correctly registered workers, got %v", r.workers)
	}

	// Call cleanup, which should stop the workers.
	c.Assert(r.cleanup(), jc.ErrorIsNil)

	r.deregisterWorker(1)
	r.deregisterWorker(2)
	c.Check(r.workers, gc.HasLen, 0)
}

func (s *residentSuite) TestResidentWorkerCleanupErrors(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	// Stop attempted for all workers, 2 fail, 1 succeeds.
	w1 := mocks.NewMockWorker(ctrl)
	w1.EXPECT().Kill()
	w1.EXPECT().Wait().Return(errors.New("biff"))

	w2 := mocks.NewMockWorker(ctrl)
	w2.EXPECT().Kill()
	w2.EXPECT().Wait().Return(errors.New("thwack"))

	w3 := mocks.NewMockWorker(ctrl)
	w3.EXPECT().Kill()
	w3.EXPECT().Wait().Return(nil)

	r := s.Manager.new()
	_ = r.registerWorker(w1)
	_ = r.registerWorker(w2)
	_ = r.registerWorker(w3)

	err := r.cleanup()
	c.Assert(err, gc.ErrorMatches, "(.|\n|\t)*(biff|thwack)(.|\n|\t)*(biff|thwack)")
	c.Assert(err, gc.Not(gc.ErrorMatches), "worker 3")
}

func (s *residentSuite) TestResidentWorkerConcurrentDeregister(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	r := s.Manager.new()

	// Note that we do not expect deregister to stop the worker.
	deregister1 := r.registerWorker(mocks.NewMockWorker(ctrl))
	deregister2 := r.registerWorker(mocks.NewMockWorker(ctrl))

	// Unregister concurrently.
	wg := sync.WaitGroup{}
	wg.Add(2)
	go func() {
		deregister1()
		wg.Done()
	}()
	go func() {
		deregister2()
		wg.Done()
	}()
	wg.Wait()

	c.Check(r.workers, gc.HasLen, 0)
}

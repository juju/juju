// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package util

import (
	"sync"

	"github.com/juju/errors"
	"launchpad.net/tomb"

	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/dependency"
)

type Unblocker interface {
	Unblock()
}

type UnblockWaiter interface {
	Unblocked() <-chan struct{}
}

type UnblockedWaiter struct{}

func (UnblockedWaiter) Unblocked() <-chan struct{} {
	ch := make(chan struct{})
	close(ch)
	return ch
}

func BlockerManifold() dependency.Manifold {
	mu := new(sync.Mutex)
	ch := make(chan struct{})
	return dependency.Manifold{
		Start: func(_ dependency.GetResourceFunc) (worker.Worker, error) {
			return newBlockerWorker(mu, ch), nil
		},
		Output: func(in worker.Worker, out interface{}) error {
			inWorker, _ := in.(*blockerWorker)
			if inWorker == nil {
				return errors.Errorf("in should be a *blockerWorker; is %#v", in)
			}
			switch outPointer := out.(type) {
			case *Unblocker:
				*outPointer = inWorker
			case *UnblockWaiter:
				*outPointer = inWorker
			default:
				return errors.Errorf("out shoudl be a pointer to an Unblocker or an UnblockWaiter; is %#v", out)
			}
			return nil
		},
	}
}

type blockerWorker struct {
	tomb tomb.Tomb
	mu   *sync.Mutex
	ch   chan struct{}
}

func newBlockerWorker(mu *sync.Mutex, ch chan struct{}) worker.Worker {
	w := &blockerWorker{
		mu: mu,
		ch: ch,
	}
	go func() {
		defer w.tomb.Done()
		<-w.tomb.Dying()
	}()
	return w
}

func (w *blockerWorker) Kill() {
	w.tomb.Kill(nil)
}

func (w *blockerWorker) Wait() error {
	return w.tomb.Wait()
}

func (w *blockerWorker) Unblock() {
	w.mu.Lock()
	defer w.mu.Unlock()
	select {
	case <-w.ch:
	default:
		close(w.ch)
	}
}

func (w *blockerWorker) Unblocked() <-chan struct{} {
	return w.ch
}

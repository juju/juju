// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package worker

// simpleWorker implements the worker returned by NewSimpleWorker.
// The stopc and done channels are used for closing notifications
// only. No values are sent over them. The err value is set once only,
// just before the done channel is closed.
type simpleWorker struct {
	stopc chan struct{}
	done  chan struct{}
	err   error
}

// NewSimpleWorker returns a worker that runs the given function.  The
// stopCh argument will be closed when the worker is killed. The error returned
// by the doWork function will be returned by the worker's Wait function.
func NewSimpleWorker(doWork func(stopCh <-chan struct{}) error) Worker {
	w := &simpleWorker{
		stopc: make(chan struct{}, 1),
		done:  make(chan struct{}),
	}
	go func() {
		defer close(w.done)
		w.err = doWork(w.stopc)
	}()
	return w
}

// Kill implements Worker.Kill() and will close the channel given to the doWork
// function.
func (w *simpleWorker) Kill() {
	select {
	case w.stopc <- struct{}{}:
	default: // stopc is already closed
	}
}

// Wait implements Worker.Wait(), and will return the error returned by
// the doWork function.
func (w *simpleWorker) Wait() error {
	<-w.done
	return w.err
}

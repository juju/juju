// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package worker

// simpleWorker is a type that just provides very simple Kill and Wait
// mechanisms through the use of a couple channels.  The channels are only used
// for their "block until closed" feature.  Nothing is ever sent over them.
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
		stopc: make(chan struct{}),
		done:  make(chan struct{}),
	}
	go func() {
		w.err = doWork(w.stopc)
		close(w.done)
	}()
	return w
}

// Kill implements Worker.Kill() and will close the channel given to the doWork
// function.
func (w *simpleWorker) Kill() {
	defer func() {
		// Allow any number of calls to Kill - the second and subsequent calls
		// will panic, but we don't care.
		recover()
	}()
	close(w.stopc)
}

// Wait implements Worker.Wait(), and will return the error returned by
// the doWork function.
func (w *simpleWorker) Wait() error {
	<-w.done
	return w.err
}

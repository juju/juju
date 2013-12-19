// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package parallel

import (
	"errors"
	"io"
	"sync"

	"launchpad.net/tomb"
)

var (
	ErrStopped = errors.New("try was stopped")
	ErrClosed  = errors.New("try was closed")
)

// Try represents an attempt made concurrently
// by a number of goroutines.
type Try struct {
	tomb          tomb.Tomb
	closeMutex    sync.Mutex
	close         chan struct{}
	limiter       chan struct{}
	start         chan func()
	result        chan result
	combineErrors func(err0, err1 error) error
	maxParallel   int
	endResult     io.Closer
}

// NewTry returns an object that runs functions concurrently until one
// succeeds. The result of the first function that returns without an
// error is available from the Result method. If maxParallel is
// positive, it limits the number of concurrently running functions.
//
// The function combineErrors(oldErr, newErr) is called to determine
// the error return (see the Result method). The first time it is called,
// oldErr will be nil; subsequently oldErr will be the error previously
// returned by combineErrors. If combineErrors is nil, the last
// encountered error is chosen.
func NewTry(maxParallel int, combineErrors func(err0, err1 error) error) *Try {
	if combineErrors == nil {
		combineErrors = chooseLastError
	}
	t := &Try{
		combineErrors: combineErrors,
		maxParallel:   maxParallel,
		close:         make(chan struct{}, 1),
		result:        make(chan result),
		start:         make(chan func()),
	}
	if t.maxParallel > 0 {
		t.limiter = make(chan struct{}, t.maxParallel)
		for i := 0; i < t.maxParallel; i++ {
			t.limiter <- struct{}{}
		}
	}
	go func() {
		defer t.tomb.Done()
		val, err := t.loop()
		t.endResult = val
		t.tomb.Kill(err)
	}()
	return t
}

func chooseLastError(err0, err1 error) error {
	return err1
}

type result struct {
	val io.Closer
	err error
}

func (t *Try) loop() (io.Closer, error) {
	var err error
	close := t.close
	nrunning := 0
	for {
		select {
		case f := <-t.start:
			nrunning++
			go f()
		case r := <-t.result:
			if r.err == nil {
				return r.val, r.err
			}
			err = t.combineErrors(err, r.err)
			nrunning--
			if close == nil && nrunning == 0 {
				return nil, err
			}
		case <-t.tomb.Dying():
			if err == nil {
				return nil, ErrStopped
			}
			return nil, err
		case <-close:
			close = nil
			if nrunning == 0 {
				return nil, err
			}
		}
	}
}

// Start requests the given function to be started, waiting until there
// are less than maxParallel functions running if necessary. It returns
// an error if the function has not been started (ErrClosed if the Try
// has been closed, and ErrStopped if the try is finishing).
//
// The function should listen on the stop channel and return if it
// receives a value, though this is advisory only - the Try does not
// wait for all started functions to return before completing.
//
// If the function returns a nil error but some earlier try was
// successful (that is, the returned value is being discarded),
// its returned value will be closed by calling its Close method.
func (t *Try) Start(try func(stop <-chan struct{}) (io.Closer, error)) error {
	if t.limiter != nil {
		// Wait for availability slot.
		select {
		case <-t.limiter:
		case <-t.tomb.Dying():
			return ErrStopped
		case <-t.close:
			return ErrClosed
		}
	}
	dying := t.tomb.Dying()
	f := func() {
		val, err := try(dying)
		if t.limiter != nil {
			// Signal availability slot is now free.
			t.limiter <- struct{}{}
		}
		// Deliver result.
		select {
		case t.result <- result{val, err}:
		case <-dying:
			if err == nil {
				val.Close()
			}
		}
	}
	select {
	case t.start <- f:
		return nil
	case <-dying:
		return ErrStopped
	case <-t.close:
		return ErrClosed
	}
}

// Close closes the Try. No more functions will be started
// if Start is called, and the Try will terminate when all
// outstanding functions have completed (or earlier
// if one succeeds)
func (t *Try) Close() {
	t.closeMutex.Lock()
	defer t.closeMutex.Unlock()
	select {
	case <-t.close:
	default:
		close(t.close)
	}
}

// Dead returns a channel that is closed when the
// Try completes.
func (t *Try) Dead() <-chan struct{} {
	return t.tomb.Dead()
}

// Wait waits for the Try to complete and returns the same
// error returned by Result.
func (t *Try) Wait() error {
	return t.tomb.Wait()
}

// Result waits for the Try to complete and returns the result of the
// first successful function started by Start.
//
// If no function succeeded, the last error returned by
// combineErrors is returned. If there were no errors or
// combineErrors returned nil, ErrStopped is returned.
func (t *Try) Result() (io.Closer, error) {
	err := t.tomb.Wait()
	return t.endResult, err
}

// Kill stops the try and all its currently executing functions.
func (t *Try) Kill() {
	t.tomb.Kill(nil)
}

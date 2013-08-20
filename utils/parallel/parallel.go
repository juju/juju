// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// The parallel package provides a way of running functions concurrently while
// limiting the maximum number running at once.
package parallel

import (
	"fmt"
	"sync"
)

// Run represents a number of functions running concurrently.
type Run struct {
	limiter chan struct{}
	done    chan error
	err     chan error
	wg      sync.WaitGroup
}

// Errors holds any errors encountered during the parallel run.
type Errors []error

func (errs Errors) Error() string {
	switch len(errs) {
	case 0:
		return "no error"
	case 1:
		return errs[0].Error()
	}
	return fmt.Sprintf("%s (and %d more)", errs[0].Error(), len(errs)-1)
}

// NewRun returns a new parallel instance.  It will run up to maxParallel
// functions concurrently.
func NewRun(maxParallel int) *Run {
	if maxParallel < 1 {
		panic("parameter maxParallel must be >= 1")
	}
	parallelRun := &Run{
		limiter: make(chan struct{}, maxParallel),
		done:    make(chan error),
		err:     make(chan error),
	}
	go func() {
		var errs Errors
		for e := range parallelRun.done {
			errs = append(errs, e)
		}
		// TODO(rog) sort errors by original order of Do request?
		if len(errs) > 0 {
			parallelRun.err <- errs
		} else {
			parallelRun.err <- nil
		}
	}()
	return parallelRun
}

// Do requests that r run f concurrently.  If there are already the maximum
// number of functions running concurrently, it will block until one of them
// has completed. Do may itself be called concurrently.
func (parallelRun *Run) Do(f func() error) {
	parallelRun.limiter <- struct{}{}
	parallelRun.wg.Add(1)
	go func() {
		defer func() {
			parallelRun.wg.Done()
			<-parallelRun.limiter
		}()
		if err := f(); err != nil {
			parallelRun.done <- err
		}
	}()
}

// Wait marks the parallel instance as complete and waits for all the functions
// to complete.  If any errors were encountered, it returns an Errors value
// describing all the errors in arbitrary order.
func (parallelRun *Run) Wait() error {
	parallelRun.wg.Wait()
	close(parallelRun.done)
	return <-parallelRun.err
}
